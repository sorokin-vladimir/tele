package tg

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/gotd/log/logzap"
	"go.uber.org/zap"
	"golang.org/x/net/proxy"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// GotdClient wraps the gotd telegram client and implements the Client interface
type GotdClient struct {
	mu           sync.RWMutex
	api          *tg.Client
	mustDeliver  chan store.Event
	droppable    chan store.Event
	updates      chan store.Event
	peers        map[int64]store.Peer
	log          *zap.Logger
	traceLog     *zap.Logger
	suppressMu   sync.Mutex
	suppressIDs  map[int]struct{}
	stateStorage updates.StateStorage
	// senderNames remembers userID -> display name across updates and history
	// fetches so a live update that omits the sender's entity still resolves the
	// author instead of rendering "?" (#161).
	senderNames *nameCache
}

func NewGotdClient(log *zap.Logger, stateStorage updates.StateStorage, trace bool) *GotdClient {
	traceLog := zap.NewNop()
	if trace {
		traceLog = log
	}
	return &GotdClient{
		mustDeliver:  make(chan store.Event, 256),
		droppable:    make(chan store.Event, 64),
		updates:      make(chan store.Event, 32),
		peers:        make(map[int64]store.Peer),
		log:          log,
		traceLog:     traceLog,
		suppressIDs:  make(map[int]struct{}),
		stateStorage: stateStorage,
		senderNames:  newNameCache(),
	}
}

// acquireAPI returns the live API client under the read lock, or an error when
// the client is not connected yet.
func (c *GotdClient) acquireAPI() (*tg.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.api == nil {
		return nil, fmt.Errorf("not connected")
	}
	return c.api, nil
}

// Connect starts the gotd client. Call in a goroutine — blocks until ctx is cancelled.
// Closes readyCh once auth is complete and the updates loop has started.
// onAuth is called with the authenticated user ID and username before readyCh
// is closed; may be nil.
func (c *GotdClient) Connect(ctx context.Context, cfg *config.Config, af *AuthFlow, readyCh chan<- struct{}, onAuth func(int64, string)) error {
	sess := NewFileSession(cfg.Telegram.SessionFile)

	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, c.mustDeliver, c.droppable, c.log, func(id int) bool {
		c.suppressMu.Lock()
		defer c.suppressMu.Unlock()
		if _, ok := c.suppressIDs[id]; ok {
			delete(c.suppressIDs, id)
			return true
		}
		return false
	}, c.senderNames)

	// updates.New does not return an error — confirmed via go doc.
	updCfg := updates.Config{
		Handler: dispatcher,
		Storage: c.stateStorage,
		// Without an explicit logger updates.Manager defaults to zap.NewNop(),
		// discarding all of its gap/idle-timeout/getDifference diagnostics. Wire
		// our logger so the manager's recovery behavior after a long idle is
		// observable (#119): "Idle timeout", "Getting difference", "Pts gap
		// timeout" and channel-difference results all surface at debug level.
		// gotd v0.154.0 changed Logger from *zap.Logger to gotd/log.Logger;
		// wrap our zap logger with the logzap adapter to keep zap out of gotd's core graph.
		Logger: logzap.New(c.log.Named("updates")),
	}
	// Persist channel access hashes so channels are re-registered at startup and
	// UpdateChannelTooLong after a long idle is acted upon instead of dropped
	// (#119). Falls back to gotd's in-memory hasher if the storage does not
	// implement it.
	if h, ok := c.stateStorage.(updates.ChannelAccessHasher); ok {
		updCfg.AccessHasher = h
	}
	manager := updates.New(updCfg)

	// outboxHook intercepts UpdateReadHistoryOutbox / UpdateReadChannelOutbox before
	// the pts-tracking layer. updates.Manager silently drops these when a pts gap is
	// present (the pending buffer never flushes), so we extract them from the raw
	// wire message and emit the event immediately, then hand the update on to the
	// manager as usual.
	hook := newOutboxHook(manager, c.mustDeliver, c.log)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-c.mustDeliver:
				select {
				case c.updates <- evt:
				case <-ctx.Done():
					return
				}
			case evt := <-c.droppable:
				select {
				case c.updates <- evt:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	dialer := proxy.FromEnvironment()
	if dialer != proxy.Direct {
		c.log.Info("using system proxy from ALL_PROXY")
	}
	resolver := dcs.Plain(dcs.PlainOptions{
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				return cd.DialContext(ctx, network, addr)
			}
			return dialer.Dial(network, addr)
		},
	})

	tc := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		UpdateHandler:  hook,
		SessionStorage: sess,
		Resolver:       resolver,
		Logger:         logzap.New(c.log),
		// OnDead marks MTProto connection death (and the reconnect that follows)
		// so a long-idle update stall (#119) can be correlated with connection
		// drops in the logs. Logged at warn so it shows without -e.
		OnDead: func(err error) {
			c.log.Warn("mtproto connection dead", zap.Error(err))
		},
	})

	c.log.Debug("connecting to telegram")
	return tc.Run(ctx, func(ctx context.Context) error {
		c.log.Debug("running auth flow")
		flow := auth.NewFlow(af, auth.SendCodeOptions{})
		if err := tc.Auth().IfNecessary(ctx, flow); err != nil {
			c.log.Error("auth failed", zap.Error(err))
			return err
		}

		self, err := tc.Self(ctx)
		if err != nil {
			c.log.Error("Self() failed", zap.Error(err))
			return err
		}
		c.log.Info("authenticated", zap.Int64("user_id", self.ID))

		if onAuth != nil {
			onAuth(self.ID, self.Username)
		}

		c.mu.Lock()
		c.api = tc.API()
		c.mu.Unlock()

		return manager.Run(ctx, tc.API(), self.ID, updates.AuthOptions{
			OnStart: func(ctx context.Context) {
				c.log.Debug("updates manager started, signalling ready")
				close(readyCh)
			},
		})
	})
}

func (c *GotdClient) Updates() <-chan store.Event {
	return c.updates
}
