package core

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// Start connects to Telegram and runs until ctx is cancelled. The caller runs it
// in its own goroutine; the update loop is started separately by RunUpdates.
func (o *Owner) Start(ctx context.Context) error {
	return o.client.Connect(ctx, o.Config(), o.authFlow, o.readyCh, func(userID int64, username string) {
		o.state.Store().SetOwnerID(userID)
		o.onAuth(userID, username)
	})
}

// RunUpdates applies incoming Telegram events to domain state and makes the
// notification decision. Publishing deltas is the commit listener's job, not
// this loop's.
func (o *Owner) RunUpdates(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-o.events:
			o.log.Debug("incoming update", zap.Int("kind", int(evt.Kind)))
			o.handleEvent(evt)
		}
	}
}

// handleEvent applies one event and decides what the user hears about it. One
// decision, one clock, two sinks: splitting it in two is how the desktop
// notification and the in-app toast used to drift apart (#192).
//
// Each sink can be switched off, and the switches are read here rather than
// where a sink delivers. A switch says where a notification goes, never whether
// it was decided, so the decision above it never learns about them (#249, ADR
// 0013).
func (o *Owner) handleEvent(evt store.Event) {
	// A gap is not an update to apply. Nothing arrived and nothing changed:
	// Telegram said something is missing, which is work to schedule rather than
	// news to break. It notifies nobody for the same reason - the messages it
	// is about are exactly the ones that have not come yet.
	switch evt.Kind {
	case store.EventChannelGap:
		o.recordGap(evt.ChatID)
		return
	case store.EventGapScan:
		go o.scanForGaps(o.ctx)
		return
	}

	// Applying commits, and the owner's commit listener publishes the resulting
	// deltas. Nothing is forwarded from here.
	//
	// What follows an event follows from the change it made, not from its
	// arrival. Delivery is at least once, so the same event can be applied twice
	// and the second time changes nothing: no banner, no toast, no row flash
	// (ADR 0016). The notification is still decided once, from one snapshot, and
	// both sinks still get the same value (#192, ADR 0013).
	if _, changed := state.Apply(o.state, evt); !changed {
		return
	}

	focused := o.focus.focused
	now := time.Now()
	// One snapshot for the decision and both gates: a reload between two reads
	// would deliver a body rendered under one config to a sink chosen under
	// another.
	notify := o.Config().UI.Notifications
	if n, ok := decideNotification(o.state.Store(), evt, focused,
		notify.Preview, now); ok {
		if notify.Desktop {
			if err := o.notifier.Notify(n.Title, n.Body); err != nil {
				o.log.Warn("desktop notification failed", zap.Error(err))
			}
		}
		if notify.Toast {
			o.publishNotification(n)
		}
	}
	o.publishIncoming(evt, focused)
}

// Bootstrap loads the authoritative dialog list and folder filters once the
// connection is up. Errors on the archived list are logged and swallowed,
// matching the behaviour this replaces: it is not fatal to a session.
func (o *Owner) Bootstrap(ctx context.Context) error {
	chats, err := o.client.GetDialogs(ctx)
	if err != nil {
		return err
	}
	o.log.Info("dialogs loaded", zap.Int("count", len(chats)))
	o.state.SetDialogs(chats)

	archived, err := o.client.GetArchivedDialogs(ctx)
	if err != nil {
		o.log.Warn("GetArchivedDialogs failed", zap.Error(err))
	} else {
		o.log.Info("archived dialogs loaded", zap.Int("count", len(archived)))
		o.state.SetDialogs(archived)
	}
	return nil
}

// LoadFolderFilters refreshes folder filters from the network. Returns the
// filters so the caller can push them to a view; an empty result means the
// account has none and the cached list should stand.
func (o *Owner) LoadFolderFilters(ctx context.Context) ([]domain.FolderFilter, error) {
	filters, err := o.client.GetDialogFilters(ctx)
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return nil, nil
	}
	o.log.Info("folder filters loaded", zap.Int("count", len(filters)))
	o.state.SetFolderFilters(filters)
	return filters, nil
}
