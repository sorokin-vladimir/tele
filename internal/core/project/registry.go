package project

import "sync"

// Emit hands one delta to the client. It reports whether the delta was taken.
//
// It runs under the registry's lock, which is what keeps a subscription's
// deltas in the order they were computed: each is stated against the one
// before it, so two refreshes on different goroutines must not swap theirs on
// the way out. It must therefore never block and never call back into the
// registry.
type Emit func(Delta) bool

// Registry holds one client's live subscriptions and the contents each was last
// told about. It is the only stateful piece of this package: builders and diffs
// are pure, so everything that can go wrong concentrates here.
//
// It belongs to one client, and the ids in it are the client's: a client names
// a subscription when it makes one, so subscribing never waits for an answer
// (#309).
type Registry struct {
	mu     sync.Mutex
	reader Reader
	emit   Emit
	subs   map[SubID]*sub
}

type sub struct {
	window Window
	list   ChatListContents
	chat   HistoryContents
	topics TopicListContents
	// account is the account as last told, and accountTold whether it has been
	// told at all: unlike a list, an account has no empty state to diff a first
	// build against.
	account     AccountContents
	accountTold bool
}

func NewRegistry(r Reader, emit Emit) *Registry {
	return &Registry{reader: r, emit: emit, subs: make(map[SubID]*sub)}
}

// Subscribe registers a window under the id the client chose and replies with
// its current contents, so a resubscribe is a full resync and no snapshot
// concept is needed. An id already in use is replaced: the client never reuses
// one, so a second use can only be a resubscribe.
func (g *Registry) Subscribe(id SubID, w Window) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.subs[id] = &sub{window: w}
	g.send(g.rebuild(id))
}

// MoveWindow replaces a subscription's window. It carries a whole window rather
// than a delta, so repeating it is equivalent to resubscribing.
func (g *Registry) MoveWindow(id SubID, w Window) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.subs[id]
	if !ok {
		return
	}
	s.window = carryPin(s.window, w)
	g.send(g.rebuild(id))
}

// carryPin keeps a first-unread window's pinned anchor across a move that does
// not name one. Widening for older history repeats the anchor the client opened
// with, and re-pinning it there would re-anchor the window on whatever happens
// to be unread by then. A client asking for a different history, or naming an
// anchor itself, gets exactly what it asked for.
func carryPin(prev, next Window) Window {
	p, okPrev := prev.(HistoryWindow)
	n, okNext := next.(HistoryWindow)
	if !okPrev || !okNext {
		return next
	}
	if p.History() != n.History() || p.Anchor.Kind != n.Anchor.Kind || n.Anchor.MsgID != 0 {
		return next
	}
	n.Anchor.MsgID = p.Anchor.MsgID
	return n
}

func (g *Registry) Unsubscribe(id SubID) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.subs, id)
}

// Window reports a subscription's current window, so the owner can decide
// whether it needs backfilling from Telegram.
func (g *Registry) Window(id SubID) (Window, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	s, ok := g.subs[id]
	if !ok {
		return nil, false
	}
	return s.window, true
}

// HistoryWindows reports every history window the client holds, with the
// anchor a first-unread window was pinned to once it resolved.
func (g *Registry) HistoryWindows() []HistoryWindow {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []HistoryWindow
	for _, s := range g.subs {
		if w, ok := s.window.(HistoryWindow); ok {
			out = append(out, w)
		}
	}
	return out
}

// Refresh rebuilds every subscription and emits whatever actually differs.
// The owner calls it once per applied state change; a change no window contains
// produces no deltas, which is the point.
func (g *Registry) Refresh() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.send(g.rebuildAll())
}

// send emits deltas in the order they were computed. Caller holds the lock.
//
// A delta the client did not take leaves it with a copy no later delta can be
// applied to, so the rest of that subscription's deltas are withheld and its
// contents forgotten: its next rebuild diffs against nothing, which is the
// resync a fresh subscription gets.
func (g *Registry) send(ds []Delta) {
	var lost map[SubID]bool
	for _, d := range ds {
		if lost[d.Sub] {
			continue
		}
		if g.emit(d) {
			continue
		}
		if lost == nil {
			lost = make(map[SubID]bool)
		}
		lost[d.Sub] = true
		if s, ok := g.subs[d.Sub]; ok {
			s.list = ChatListContents{}
			s.chat = HistoryContents{}
			s.topics = TopicListContents{}
			s.accountTold = false
		}
	}
}

// rebuildAll refreshes every subscription. Caller holds the lock. Map iteration
// makes the order across subscriptions unspecified, which is fine: every delta
// is addressed to one subscription.
func (g *Registry) rebuildAll() []Delta {
	var out []Delta
	for id := range g.subs {
		out = append(out, g.rebuild(id)...)
	}
	return out
}

// rebuild recomputes one subscription's contents and diffs it against what the
// client was last told. Caller holds the lock.
func (g *Registry) rebuild(id SubID) []Delta {
	s := g.subs[id]
	var out []Delta
	switch w := s.window.(type) {
	case ChatListWindow:
		next := BuildChatList(g.reader, w)
		for _, d := range DiffChatList(s.list, next) {
			out = append(out, Delta{Sub: id, ChatList: &d})
		}
		s.list = next
	case TopicListWindow:
		next := BuildTopicList(g.reader, w)
		for _, d := range DiffTopicList(s.topics, next) {
			out = append(out, Delta{Sub: id, TopicList: &d})
		}
		s.topics = next
	case AccountWindow:
		next := BuildAccount(g.reader)
		if !s.accountTold || !sameAccount(s.account, next) {
			out = append(out, Delta{Sub: id, Account: &AccountDelta{Contents: next}})
		}
		s.account, s.accountTold = next, true
	case HistoryWindow:
		next := BuildHistory(g.reader, w)
		// Pin a first-unread window to the message it actually opened on. The
		// anchor is otherwise a function of the read pointer, which moves as the
		// user reads: the window would walk forward and drop the messages above
		// out from under them. Pinning happens on the first rebuild that resolves
		// an anchor, so a window opened before its history arrived pins once the
		// backfill lands.
		if w.Anchor.Kind == AnchorFirstUnread && w.Anchor.MsgID == 0 && next.AnchorMsgID != 0 {
			w.Anchor.MsgID = next.AnchorMsgID
			s.window = w
		}
		for _, d := range DiffHistory(s.chat, next) {
			out = append(out, Delta{Sub: id, History: &d})
		}
		s.chat = next
	}
	return out
}
