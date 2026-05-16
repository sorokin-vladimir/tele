package tg

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// GotdClient wraps the gotd telegram client and implements the Client interface
type GotdClient struct {
	mu          sync.RWMutex
	api         *tg.Client
	events      chan store.Event
	peers       map[int64]store.Peer
	log         *zap.Logger
	suppressMu  sync.Mutex
	suppressIDs map[int]struct{}
}

func NewGotdClient(log *zap.Logger) *GotdClient {
	return &GotdClient{
		events:      make(chan store.Event, 64),
		peers:       make(map[int64]store.Peer),
		log:         log,
		suppressIDs: make(map[int]struct{}),
	}
}

// Connect starts the gotd client. Call in a goroutine — blocks until ctx is cancelled.
// Closes readyCh once auth is complete and the updates loop has started.
func (c *GotdClient) Connect(ctx context.Context, cfg *config.Config, af *AuthFlow, readyCh chan<- struct{}) error {
	sess := NewFileSession(cfg.Telegram.SessionFile)

	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, c.events, func(id int) bool {
		c.suppressMu.Lock()
		defer c.suppressMu.Unlock()
		if _, ok := c.suppressIDs[id]; ok {
			delete(c.suppressIDs, id)
			return true
		}
		return false
	})

	// updates.New does not return an error — confirmed via go doc.
	manager := updates.New(updates.Config{
		Handler: dispatcher,
	})

	// outboxHook intercepts UpdateReadHistoryOutbox / UpdateReadChannelOutbox before
	// the pts-tracking layer. updates.Manager silently drops these when a pts gap is
	// present (the pending buffer never flushes), so we extract them from the raw
	// wire message and emit the event immediately, then hand the update on to the
	// manager as usual.
	hook := newOutboxHook(manager, c.events, c.log)

	tc := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		UpdateHandler:  hook,
		SessionStorage: sess,
		Logger:         c.log,
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
		c.log.Info("authenticated", zap.Int64("user_id", self.ID), zap.String("username", self.Username))

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
	return c.events
}
