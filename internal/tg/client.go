package tg

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/gotd/log/logzap"
	"go.uber.org/zap"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/updates"
	updhook "github.com/gotd/td/telegram/updates/hook"
	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// GotdClient wraps the gotd telegram client and implements the Client interface
type GotdClient struct {
	mu           sync.RWMutex
	api          *tg.Client
	mustDeliver  chan store.Event
	droppable    chan store.Event
	updates      chan store.Event
	peers        map[int64]domain.Peer
	log          *zap.Logger
	traceLog     *zap.Logger
	suppressMu   sync.Mutex
	suppressIDs  map[int]struct{}
	stateStorage updates.StateStorage
	// resolver is how this client reaches Telegram: directly, or through the
	// proxy the config named. Handed in rather than built here, because a proxy
	// that cannot be reached has to stop the start before anything is drawn.
	resolver dcs.Resolver
	// senderNames remembers userID -> display name across updates and history
	// fetches so a live update that omits the sender's entity still resolves the
	// author instead of rendering "?" (#161).
	senderNames *nameCache
	// skew watches for a local clock too far off for gotd to accept what
	// Telegram sends (#277).
	skew *clockSkew
}

// SetOnClockSkew installs the function told about clock skew: the skew when it
// is found or moves, positive when the local clock is ahead, and zero when
// messages get through again. It must not block. Set before Connect.
func (c *GotdClient) SetOnClockSkew(report func(time.Duration)) { c.skew.SetReport(report) }

func NewGotdClient(log *zap.Logger, stateStorage updates.StateStorage, trace bool, resolver dcs.Resolver) *GotdClient {
	traceLog := zap.NewNop()
	if trace {
		traceLog = log
	}
	return &GotdClient{
		mustDeliver:  make(chan store.Event, 256),
		droppable:    make(chan store.Event, 64),
		updates:      make(chan store.Event, 32),
		peers:        make(map[int64]domain.Peer),
		log:          log,
		traceLog:     traceLog,
		suppressIDs:  make(map[int]struct{}),
		stateStorage: stateStorage,
		resolver:     resolver,
		senderNames:  newNameCache(),
		skew:         newClockSkew(),
	}
}

// acquireAPI returns the live API client under the read lock, or an error when
// the client is not connected yet. The failure never reaches the middleware —
// there is no RPC — so it is classified here.
func (c *GotdClient) acquireAPI() (*tg.Client, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.api == nil {
		return nil, &telerr.Error{Kind: telerr.Network, Op: "acquire api", Detail: "not connected", Transient: true}
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
		// gotd resets the channel's position and carries on, and says so here.
		// The messages between the old position and the new one are not in that
		// difference and never will be: recovering them is the application's
		// job, and the manager documents that it is not doing it. Left to the
		// default this is a log line and a hole in the chat (#262).
		OnChannelTooLong: func(channelID int64) {
			c.log.Warn("channel fell too far behind; recording a gap", zap.Int64("channel_id", channelID))
			// mustDeliver rather than droppable, and for both reasons: a
			// dropped gap is a hole nothing will ever look for again, and the
			// mark has to be taken before the messages that follow it move the
			// tail past the hole. Same channel as those messages is the only
			// way the order is guaranteed.
			select {
			case c.mustDeliver <- store.Event{Kind: store.EventChannelGap, ChatID: channelID}:
			case <-ctx.Done():
			}
		},
		// The same, for the account's own state, which every chat that is not a
		// channel shares. It names nobody, so the receiver has to go and look.
		OnTooLong: func() {
			c.log.Warn("account state fell too far behind; scanning for gaps")
			select {
			case c.mustDeliver <- store.Event{Kind: store.EventGapScan}:
			case <-ctx.Done():
			}
		},
		// Not a gap but a channel nothing is listening to for the rest of the
		// session. Wired only so it is visible: without it the default logger
		// swallows it and the channel goes quiet with no explanation.
		OnLoadChannelStateFailed: func(channelID int64) {
			c.log.Error("channel state could not be loaded; it will receive no updates this session",
				zap.Int64("channel_id", channelID))
		},
		// channelDiffAPI below strips the server-side cooldown that used to space
		// out getChannelDifference, so bound the calls in flight here instead
		// (#266). Every tracked channel asks for its difference at startup and
		// again on every gap; unbounded, that is one burst per channel against a
		// per-account method rate limit.
		MaxChannelDifferenceConcurrency: maxChannelDiffConcurrency,
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
	// Every update that arrives is a message the clock check let through, so it
	// ends a clock skew (#277).
	arrivals := c.skew.arrivals(hook)

	// resync forces a full catch-up (getDifference) on every reconnect after the
	// first, so state missed during OS sleep/suspend is reconciled immediately on
	// wake instead of waiting for the manager's frozen 15-minute idle timer (#173).
	// UpdatesTooLong is the manager's own "you are too far behind, fetch the
	// difference" signal, so it drives the same common-state + channel catch-up
	// the server would ask for. Run off the connection-lifecycle callback (which
	// must not block) in a goroutine bounded by ctx: Handle can block on the
	// manager's internal queue.
	resync := &reconnectResync{
		force: func() {
			go func() {
				c.log.Info("reconnected, forcing updates catch-up")
				if err := manager.Handle(ctx, &tg.UpdatesTooLong{}); err != nil {
					c.log.Warn("forced updates catch-up after reconnect failed", zap.Error(err))
				}
			}()
		},
	}

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

	c.log.Info("gotd client", zap.String("gotd", gotdVersion()))

	tc := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		UpdateHandler:  arrivals,
		SessionStorage: sess,
		// One resolver for every data centre this client ever reaches, so a
		// photo from a media DC takes the same route as the message it came
		// with. It is built before the interface exists, from the proxy section
		// of the config (ADR 0017).
		Resolver: c.resolver,
		// The "v" field carries the application version on every line, so gotd's
		// own stamp of the same name is dropped and reported once above instead.
		// The clock-skew watch reads the entries gotd writes for the messages it
		// drops, the only place a skewed clock shows (#277).
		Logger: logzap.New(watchClockSkew(withoutField(c.log, "v"), c.skew)),
		// Names this app and this machine in Telegram's active-sessions list;
		// without it gotd reports the Go toolchain and its own version (#200).
		Device: deviceConfig(version.Version, os.Hostname),
		// OnDead marks MTProto connection death (and the reconnect that follows)
		// so a long-idle update stall (#119) can be correlated with connection
		// drops in the logs. Logged at warn so it shows without -e.
		OnDead: func(err error) {
			c.log.Warn("mtproto connection dead", zap.Error(err))
		},
		// OnConnectionState drives the post-reconnect catch-up (#173): gotd emits
		// ConnectionStateReady on every (re)established primary connection, which
		// is the reliable wake/reconnect signal the frozen idle timer is not.
		OnConnectionState: resync.onConnectionState,
		// Maps every RPC error onto the domain taxonomy exactly once, so tgerr
		// never leaves this package (#191).
		// errorMiddleware maps every RPC error onto the domain taxonomy; the
		// update hook feeds the updates an RPC *reply* carries into the same
		// pipeline the pushed ones go through.
		//
		// Telegram does not push an echo for what this client itself did: the
		// created message comes back in the reply to messages.forwardMessages (or
		// sendMessage, sendMedia) and nowhere else, until an unrelated pts gap
		// forces a getDifference. Without this hook a forward stayed invisible
		// until the other side happened to read it (#198).
		Middlewares: []telegram.Middleware{c.errorMiddleware(), updhook.UpdateHook(arrivals.Handle)},
	})

	c.log.Debug("connecting to telegram")
	return tc.Run(ctx, func(ctx context.Context) error {
		c.log.Debug("running auth flow")
		// Our own login loop rather than gotd's auth.Flow, which gives up on
		// the first mistyped code (#285). Asked only when the session is not
		// already authorized, as auth.Client.IfNecessary does.
		status, err := tc.Auth().Status(ctx)
		if err != nil {
			c.log.Error("auth status failed", zap.Error(err))
			return err
		}
		if !status.Authorized {
			if err := af.login(ctx, tc.Auth()); err != nil {
				c.log.Error("auth failed", zap.Error(err))
				return err
			}
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

		// Two wrappers over the same API, one per defect in the manager's gap
		// handling: the channel difference carries a cooldown that must not be
		// obeyed (#266), and a common difference carries updates the manager
		// then drops (#267). Each dies on its own when its upstream fix ships.
		diffAPI := newCommonDiffAPI(newChannelDiffAPI(tc.API(), c.log), dispatcher, c.log)
		return manager.Run(ctx, diffAPI, self.ID, updates.AuthOptions{
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
