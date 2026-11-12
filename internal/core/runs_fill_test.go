package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// telegramHistory is Telegram holding one chat's whole history and answering
// each way of paging through it as the API does, recording what it was asked.
type telegramHistory struct {
	Connection
	mu  sync.Mutex
	all []domain.Message // oldest first

	asked []string
}

func (s *telegramHistory) Updates() <-chan store.Event { return nil }

func (s *telegramHistory) note(call string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, call)
}

func (s *telegramHistory) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

func (s *telegramHistory) GetHistory(_ context.Context, _ domain.Peer, offsetID, limit int) ([]domain.Message, error) {
	s.note("older")
	var older []domain.Message
	for _, m := range s.all {
		if offsetID == 0 || m.ID < offsetID {
			older = append(older, m)
		}
	}
	if len(older) > limit {
		older = older[len(older)-limit:]
	}
	return older, nil
}

func (s *telegramHistory) GetHistoryAfter(_ context.Context, _ domain.Peer, _, afterID, limit int) ([]domain.Message, error) {
	s.note("newer")
	var newer []domain.Message
	for _, m := range s.all {
		if m.ID > afterID {
			newer = append(newer, m)
		}
	}
	if len(newer) == 0 {
		// Nothing newer: the window slides back over what the caller has.
		if len(s.all) > limit {
			return s.all[len(s.all)-limit:], nil
		}
		return s.all, nil
	}
	if len(newer) > limit {
		newer = newer[:limit]
	}
	return newer, nil
}

func (s *telegramHistory) GetHistoryAround(_ context.Context, _ domain.Peer, _, id, limit int) ([]domain.Message, error) {
	s.note("around")
	var older, newer []domain.Message
	for _, m := range s.all {
		if m.ID <= id {
			older = append(older, m)
		} else {
			newer = append(newer, m)
		}
	}
	half := limit / 2
	if len(newer) > half {
		newer = newer[:half]
	}
	if keep := limit - len(newer); len(older) > keep {
		older = older[len(older)-keep:]
	}
	return append(older, newer...), nil
}

func msgRange(lo, hi int) []domain.Message {
	out := make([]domain.Message, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, domain.Message{ID: id, ChatID: 7, Date: time.Unix(int64(id), 0)})
	}
	return out
}

// historyOwner is an owner over chat 7, whose whole history on Telegram is
// server, and which holds tail on the store.
func historyOwner(t *testing.T, server []domain.Message, tail []domain.Message) (*Owner, *state.State, *telegramHistory) {
	t.Helper()
	tg := &telegramHistory{all: server}
	o, s := newOwnerWithClient(t, tg)
	s.Store().SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7}})
	if len(tail) > 0 {
		s.Store().SetMessages(7, tail)
	}
	return o, s, tg
}

func runIDs(s *state.State) [][]int {
	var out [][]int
	for _, r := range s.Store().Runs(domain.HistoryKey{ChatID: 7}) {
		ids := make([]int, 0, len(r.Messages))
		for _, m := range r.Messages {
			ids = append(ids, m.ID)
		}
		out = append(out, ids)
	}
	return out
}

// waitWindow waits for the window to hold want.
func waitWindow(t *testing.T, cl *Attachment, want int) project.HistoryContents {
	t.Helper()
	return waitContents(t, cl, func(c project.HistoryContents) bool { return holds(c.Messages, want) })
}

// waitContents follows a client's history deltas the way a client keeps its
// copy, until the copy satisfies done.
func waitContents(t *testing.T, cl *Attachment, done func(project.HistoryContents) bool) project.HistoryContents {
	t.Helper()
	var c project.HistoryContents
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("the window never got there; last: %v", ids(c.Messages))
		default:
		}
		d, ok := recvDelta(t, cl)
		if !ok || d.History == nil {
			continue
		}
		switch h := d.History; h.Kind {
		case project.HistoryReset, project.HistoryHeaderUpdate:
			// Both carry the contents whole.
			c = h.Contents
		case project.HistoryOlder:
			c.Messages = append(append([]domain.Message(nil), h.Messages...), c.Messages...)
		case project.HistoryNewer:
			c.Messages = append(c.Messages, h.Messages...)
		case project.HistoryAppend:
			c.Messages = append(c.Messages, h.Message)
		}
		if done(c) {
			return c
		}
	}
}

// openAt opens a window on message id of chat 7 and waits for it to show.
func openAt(t *testing.T, o *Owner, id, before, after int) (*Attachment, project.SubID, project.HistoryWindow) {
	t.Helper()
	cl := testClient(o)
	w := project.HistoryWindow{
		ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: id}, Before: before, After: after,
	}
	sub := cl.Subscribe(w)
	waitWindow(t, cl, id)
	return cl, sub, w
}

// Scrolling up a run far back fetches what is older than that run, not older
// than the oldest message the history holds.
func TestFill_ScrollingUpAFarRunGrowsItDownwards(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	cl, sub, w := openAt(t, o, 500, 5, 5)

	w.Before += 20
	cl.MoveWindow(sub, w)

	waitWindow(t, cl, 480)
	assert.Equal(t, []string{"around", "older"}, tg.calls())
	assert.Equal(t, 471, runIDs(s)[0][0], "twenty older than the page around 500, which began at 491")
}

// Scrolling up the tail run fetches what is older than the tail run, even
// with a run far back stored below it.
func TestFill_ScrollingUpTheTailRunIgnoresARunFarBack(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	s.Store().MergeAround(domain.HistoryKey{ChatID: 7}, msgRange(495, 505))
	cl := testClient(o)
	w := project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 19}
	sub := cl.Subscribe(w)
	waitWindow(t, cl, 1020)

	w.Before += 10
	cl.MoveWindow(sub, w)

	waitWindow(t, cl, 991)
	assert.Equal(t, []string{"older"}, tg.calls())
	assert.Equal(t, [][]int{idRange(495, 505), idRange(981, 1020)}, runIDs(s))
}

func idRange(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, id)
	}
	return out
}

// Scrolling down a run far back fetches what is newer than it.
func TestFill_ScrollingDownAFarRunGrowsItUpwards(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	cl, sub, w := openAt(t, o, 500, 5, 5)

	w.After += 20
	cl.MoveWindow(sub, w)

	waitWindow(t, cl, 520)
	assert.Equal(t, []string{"around", "newer"}, tg.calls())
	far := runIDs(s)[0]
	assert.Equal(t, 530, far[len(far)-1])
}

// A run far back grown until it meets the tail run becomes one with it, and
// the window is at the tail again.
func TestFill_AFarRunThatReachesTheTailJoinsIt(t *testing.T) {
	o, s, _ := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	cl, sub, w := openAt(t, o, 990, 5, 5)

	w.After += 20
	cl.MoveWindow(sub, w)

	waitContents(t, cl, func(c project.HistoryContents) bool { return holds(c.Messages, 1010) && c.AtTail })
	assert.Len(t, runIDs(s), 1)
}

// A message Telegram no longer has is not among the page around where it was.
// The client hears which message is nearest, to open there instead.
func TestFill_AJumpToAMessageThatIsGoneSaysSo(t *testing.T) {
	server := append(msgRange(1, 499), msgRange(501, 1020)...)
	o, _, _ := historyOwner(t, server, msgRange(1001, 1020))
	cl := testClient(o)

	cl.Subscribe(project.HistoryWindow{
		ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 500}, Before: 5, After: 5,
	})

	gone := waitEvent[MessageGone](t, cl)
	assert.Equal(t, MessageGone{ChatID: 7, MsgID: 500, Nearest: 499}, gone)
}

// A history opened at a far message holds no tail run. A window at the tail
// fetches the newest page, which is a run of its own.
func TestFill_AWindowAtTheTailWithNoTailRunFetchesTheNewest(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1020), nil)
	s.Store().MergeAround(domain.HistoryKey{ChatID: 7}, msgRange(495, 505))
	cl := testClient(o)

	cl.Subscribe(project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 5})

	c := waitWindow(t, cl, 1020)
	assert.True(t, c.AtTail)
	assert.Equal(t, []string{"older"}, tg.calls())
	assert.Len(t, runIDs(s), 2)
}

// A tail reload discards the run another client's window is on. That window's
// anchor is then a message the store does not hold, and the page around it is
// fetched again, as a jump would.
func TestFill_ATailReloadRefetchesAWindowItLeftWithNothing(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1700), msgRange(1001, 1020))
	a, _, _ := openAt(t, o, 500, 5, 5)
	// A gap far wider than the chat can hold: the repair gives up and reloads
	// the tail.
	s.Store().MarkGap(7, 1020)

	b := attach(o)
	b.Subscribe(project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 5})

	waitContents(t, a, func(c project.HistoryContents) bool { return len(c.Messages) == 0 })
	waitWindow(t, a, 500)
	assert.Equal(t, 2, count(tg.calls(), "around"), "the page around 500 was fetched again")
	_, gap := s.Store().Gap(7)
	assert.False(t, gap)
}

// A person looking far back in a chat still has that chat focused: a message
// arriving at its tail raises no notification, and stays unread, since being
// focused is not being read.
func TestNotify_LookingFarBackKeepsTheChatFocusedAndTheArrivalUnread(t *testing.T) {
	n := &mockNotifier{}
	o, st := newTestOwnerNotified(t, n)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}, ReadInboxMaxID: 1020})
	far := func(lo, hi int) []domain.Message {
		out := msgRange(lo, hi)
		for i := range out {
			out[i].ChatID = 2
		}
		return out
	}
	st.SetMessages(2, far(1001, 1020))
	st.MergeAround(domain.HistoryKey{ChatID: 2}, far(495, 505))
	c := attach(o)
	c.SetFocus(domain.HistoryKey{ChatID: 2})
	c.Subscribe(project.HistoryWindow{
		ChatID: 2, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 500}, Before: 5, After: 5,
	})

	o.handleEvent(store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ID: 1021, ChatID: 2, Text: "hey", Date: time.Now()}})

	assert.Empty(t, n.calls, "no desktop notification")
	assert.Empty(t, only[Notification](queued(c)), "no toast")
	chat, _ := st.GetChat(2)
	assert.Equal(t, 1, chat.UnreadCount, "the arrival is not on screen, so it is not read")
}

// A message arriving while a window is far back is counted for it: the
// client is told one landed at the tail.
func TestFill_AnArrivalWhileFarBackIsCountedForTheWindow(t *testing.T) {
	o, _, _ := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	cl, _, _ := openAt(t, o, 500, 5, 5)

	o.handleEvent(store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ID: 1021, ChatID: 7, Text: "new", Date: time.Unix(1021, 0)}})

	waitContents(t, cl, func(c project.HistoryContents) bool { return c.NewCount == 1 })
}

func count(calls []string, call string) int {
	n := 0
	for _, c := range calls {
		if c == call {
			n++
		}
	}
	return n
}

// waitEvent waits for the next event of type T, skipping deltas and others.
func waitEvent[T any](t *testing.T, cl *Attachment) T {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			var zero T
			t.Fatalf("no %T arrived", zero)
		case v := <-cl.Stream():
			if e, ok := v.(T); ok {
				return e
			}
		}
	}
}

// A window anchored on a message the store does not hold fetches the page
// around it, and that page is a run of its own: what lies between it and the
// tail was never fetched.
func TestFill_AJumpFetchesThePageAroundTheMessage(t *testing.T) {
	o, s, tg := historyOwner(t, msgRange(1, 1020), msgRange(1001, 1020))
	cl := testClient(o)

	cl.Subscribe(project.HistoryWindow{
		ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 500}, Before: 5, After: 5,
	})

	c := waitWindow(t, cl, 500)
	assert.Equal(t, []int{495, 496, 497, 498, 499, 500, 501, 502, 503, 504, 505}, ids(c.Messages))
	assert.False(t, c.AtTail)
	assert.Equal(t, []string{"around"}, tg.calls())
	require.Len(t, runIDs(s), 2)
}
