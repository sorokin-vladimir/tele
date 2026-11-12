package core

import (
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// projectionReader is what every projection is built from: the store plus the
// send queue. Asserted here so a change to either interface fails at compile
// time rather than at wiring time.
var _ project.Reader = projectionReader{}

// subKey names one subscription across clients: each client chooses its own
// ids, so an id alone is ambiguous once two are attached (#309).
type subKey struct {
	client clientID
	sub    project.SubID
}

// attached is a snapshot of the clients attached now. Work done for each of
// them runs outside the lock, so a client attaching or detaching meanwhile
// waits for nothing it does not need.
func (o *Owner) attached() []*seat {
	o.clientsMu.Lock()
	defer o.clientsMu.Unlock()
	out := make([]*seat, 0, len(o.clients))
	for _, a := range o.clients {
		out = append(out, a)
	}
	return out
}

// Refresh rebuilds every client's subscriptions against current state.
//
// TRANSITIONAL (#193, #195, #196): media still writes to the store directly and
// asks for a rebuild. Commands no longer do — they mutate through state, whose
// commit publishes. The forward preview bump is the one caller inside the owner.
func (o *Owner) Refresh() {
	for _, a := range o.attached() {
		a.registry.Refresh()
	}
	// A rebuild can pin a first-unread window to the message it opened on,
	// which moves what that window shows.
	o.syncShown()
}

// syncShown tells the store what every attached client's windows show, so the
// cap never drops it (ADR 0037). It runs whenever a window opens, moves,
// closes or is pinned, and before any fetch made for that window.
func (o *Owner) syncShown() {
	shown := make(map[domain.HistoryKey][]store.Shown)
	for _, a := range o.attached() {
		for _, w := range a.registry.HistoryWindows() {
			shown[w.History()] = append(shown[w.History()], shownOf(w))
		}
	}
	o.state.Store().SetShown(shown)
}

// shownOf is what a window shows, in the store's terms: its anchor, the newest
// message for a window anchored there or a first-unread window not pinned yet,
// and how far back it reaches.
func shownOf(w project.HistoryWindow) store.Shown {
	if w.Anchor.Kind == project.AnchorNewest {
		return store.Shown{Before: w.Before}
	}
	return store.Shown{Anchor: w.Anchor.MsgID, Before: w.Before}
}

// maybeFetch goes to Telegram when the store cannot answer a chat window on its
// own, so a client never has to know where data comes from. There are two
// reasons: the chat has a recorded gap, which is a hole somebody has to close,
// and the window came back short, which is history nobody has fetched yet.
func (o *Owner) maybeFetch(a *seat, id project.SubID, w project.Window) {
	if o.client == nil {
		return
	}
	if tw, ok := w.(project.TopicListWindow); ok {
		o.maybeFetchTopics(tw)
		return
	}
	cw, ok := w.(project.HistoryWindow)
	if !ok {
		return
	}
	_, hasGap := o.state.Store().Gap(cw.ChatID)
	if !hasGap && !needsBackfill(project.BuildHistory(o.reader(), cw), cw) {
		return
	}
	o.spawn(func() { o.fill(o.ctx, a, id, cw) })
}

// needsBackfill reports that the store could not fill the window: it returned
// fewer messages than were asked for and has nothing older left to give. This is
// deliberately not HasOlder, which reports the opposite — that the store does
// hold more, so no fetch is needed.
func needsBackfill(c project.HistoryContents, w project.HistoryWindow) bool {
	return !c.HasOlder && len(c.Messages) < w.Before+w.After+1
}

// publishChange turns one applied domain change into whatever the current
// subscriptions need to hear. Typing goes out as an event instead: it has no
// persisted state to rebuild a projection from, and nothing would ever clear it
// if it were held as one.
func (o *Owner) publishChange(chg state.Change) {
	if chg.Kind == state.ChangeTyping {
		o.publishTyping(Typing{ChatID: chg.ChatID, TopicID: chg.TopicID, Label: chg.Typing.Label()})
		return
	}
	// A message arriving may be one this owner queued. Dropping the entry here,
	// before the rebuild, is what makes the pending bubble and the real message
	// swap inside a single delta rather than across two, with a frame showing
	// neither in between (#193).
	if chg.Kind == state.ChangeNewMessage {
		o.clearSentOutbox(chg.ChatID)
	}
	o.Refresh()
}
