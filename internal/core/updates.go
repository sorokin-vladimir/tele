package core

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// Start connects to Telegram and runs until ctx is cancelled. The caller runs it
// in its own goroutine; the update loop is started separately by RunUpdates.
// The login, and loading the account once it is logged in, are the owner's own
// work alongside it; what the connection ended with is part of the account
// (#309).
func (o *Owner) Start(ctx context.Context) error {
	o.spawn(func() { o.runLogin(ctx) })
	err := o.client.Connect(ctx, o.authFlow, o.readyCh, func(self domain.User) {
		o.state.Store().SetOwnerID(self.ID)
		o.onAuth(self)
	})
	o.connectionEnded(ctx, err)
	return err
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
			o.heard()
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
		o.spawn(func() { o.scanForGaps(o.ctx) })
		return
	case store.EventTopicsChanged:
		// Like a gap, this is work rather than news: what changed is learned by
		// reading the topics again, and the read is what commits.
		o.topicsChanged(evt)
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
	c, changed := state.Apply(o.state, evt)
	if !changed {
		return
	}
	if c.UnknownTopic != 0 {
		o.refreshTopics(c.ChatID, []int{c.UnknownTopic})
	}

	focused := o.focus.showing
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
	// Topics are read after the dialog list rather than with it: the list is
	// what the main screen waits for, and a forum's badge can follow it. So
	// nobody waits on them, and they yield to what somebody does (#212).
	o.spawn(func() { o.loadForumTopics(priority.InBackground(ctx), append(chats, archived...)) })
	return nil
}

// forumTopicsPage is how many topics the first page of a forum asks for, which
// is what a forum's badge is counted from until a topic list is scrolled.
const forumTopicsPage = 100

// loadForumTopics reads the first page of every forum's topics. It runs at
// every connection, since topics changed while the account was away are known
// only by asking.
func (o *Owner) loadForumTopics(ctx context.Context, chats []domain.Chat) {
	for _, chat := range chats {
		if !chat.IsForum {
			continue
		}
		page, err := o.client.GetForumTopics(ctx, chat.Peer, internaltg.TopicsOffset{}, forumTopicsPage)
		if err != nil {
			o.log.Warn("forum topics load failed", zap.Int64("chat", chat.ID), zap.Error(err))
			continue
		}
		o.applyFirstTopicsPage(chat.ID, page)
	}
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
