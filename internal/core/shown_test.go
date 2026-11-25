package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// fullHistory fills chat 7 with as many messages as the cap allows, ids
// 1001 up, and returns what Telegram holds older than them.
func fullHistory(t *testing.T, s *state.State) []domain.Message {
	t.Helper()
	s.Store().SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7}})
	held := make([]domain.Message, 0, store.MaxMessagesPerChat)
	for id := 1001; id < 1001+store.MaxMessagesPerChat; id++ {
		held = append(held, domain.Message{ID: id, ChatID: 7, Date: time.Unix(int64(id), 0)})
	}
	s.Store().SetMessages(7, held)
	var older []domain.Message
	for id := 981; id <= 1000; id++ {
		older = append(older, domain.Message{ID: id, ChatID: 7, Date: time.Unix(int64(id), 0)})
	}
	return older
}

func held7(s *state.State) int { return len(s.Store().Messages(domain.HistoryKey{ChatID: 7})) }

// A person who scrolled past the cap asks for more than the store holds. The
// page fetched for them is what their window shows, and the cap keeps it.
func TestOwner_AWindowPastTheCapKeepsWhatItScrolledTo(t *testing.T) {
	c := &stubConn{}
	o, s := newOwnerWithClient(t, c)
	c.history = fullHistory(t, s)
	cl := testClient(o)
	w := project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: store.MaxMessagesPerChat - 1}
	id := cl.Subscribe(w)
	_, _ = recvDelta(t, cl)

	w.Before += 20
	cl.MoveWindow(id, w)

	require.Eventually(t, func() bool { return c.calls.Load() >= 1 }, time.Second, time.Millisecond)
	assert.Eventually(t, func() bool { return held7(s) == store.MaxMessagesPerChat+20 }, time.Second, time.Millisecond,
		"the page under the window is kept")
}

// A window opened at the first unread is pinned to the message it opened on and
// runs to the newest: a message arriving while the person reads far back must
// not push their scrollback out.
func TestOwner_AFirstUnreadWindowKeepsItsScrollbackAsMessagesArrive(t *testing.T) {
	c := &stubConn{}
	o, s := newOwnerWithClient(t, c)
	c.history = fullHistory(t, s)
	cl := testClient(o)
	w := project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: store.MaxMessagesPerChat - 1}
	id := cl.Subscribe(w)
	_, _ = recvDelta(t, cl)
	w.Before += 20
	cl.MoveWindow(id, w)
	require.Eventually(t, func() bool { return held7(s) == store.MaxMessagesPerChat+20 }, time.Second, time.Millisecond)

	s.Store().AppendMessage(domain.Message{ID: 5000, ChatID: 7, Date: time.Unix(5000, 0)})

	assert.Equal(t, store.MaxMessagesPerChat+21, held7(s))
}

// Once nobody looks, the scrollback is given up at the next write.
func TestOwner_AClosedWindowGivesItsScrollbackUp(t *testing.T) {
	c := &stubConn{}
	o, s := newOwnerWithClient(t, c)
	c.history = fullHistory(t, s)
	cl := testClient(o)
	w := project.HistoryWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: store.MaxMessagesPerChat + 19}
	id := cl.Subscribe(w)
	require.Eventually(t, func() bool { return held7(s) == store.MaxMessagesPerChat+20 }, time.Second, time.Millisecond)

	cl.Unsubscribe(id)
	s.Store().AppendMessage(domain.Message{ID: 5000, ChatID: 7, Date: time.Unix(5000, 0)})

	assert.Equal(t, store.MaxMessagesPerChat, held7(s))
}
