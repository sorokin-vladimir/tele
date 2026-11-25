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

// syncShown tells the store what every attached client's windows show, and
// what commands are reading, so the cap never drops it: a scrollback somebody
// loaded is held while somebody looks at it and given up once nobody does. It
// runs whenever a window opens, moves, closes or is pinned, and before any
// fetch made for that window.
func (o *Owner) syncShown() {
	shown := make(map[domain.HistoryKey][]store.Shown)
	for _, a := range o.attached() {
		for _, w := range a.registry.HistoryWindows() {
			shown[w.History()] = append(shown[w.History()], shownOf(w))
		}
	}
	o.readingMu.Lock()
	for _, r := range o.reading {
		shown[r.h] = append(shown[r.h], r.shown)
	}
	o.readingMu.Unlock()
	o.state.Store().SetShown(shown)
}

// readHold is what one command is reading of a history.
type readHold struct {
	h     domain.HistoryKey
	shown store.Shown
}

// holdReading shows the store what a command is reading, for as long as it
// reads: a page it fetched must still be there when it reads it back, which
// the cap would not otherwise promise with the store full and no window open.
// The returned func lets go of it; the next write may then trim it, as the cap
// trims any run nobody looks at.
func (o *Owner) holdReading(h domain.HistoryKey, shown store.Shown) func() {
	o.readingMu.Lock()
	o.readingNext++
	token := o.readingNext
	o.reading[token] = readHold{h: h, shown: shown}
	o.readingMu.Unlock()
	o.syncShown()
	return func() {
		o.readingMu.Lock()
		delete(o.reading, token)
		o.readingMu.Unlock()
		o.syncShown()
	}
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
	if !hasGap && !shortfallOf(project.BuildHistory(o.reader(), cw), cw).any() {
		return
	}
	o.spawn(func() { o.fill(o.ctx, a, id, cw) })
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
