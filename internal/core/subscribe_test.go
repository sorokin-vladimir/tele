package core

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// stubConn answers GetHistory and nothing else. The embedded interface is nil,
// so any other call panics — which is the point: a test that reaches further
// than it declared should fail loudly.
type stubConn struct {
	internaltg.Client
	history []domain.Message
	calls   atomic.Int32
	release chan struct{}
}

func (s *stubConn) Connect(context.Context, *config.Config, *internaltg.AuthFlow, chan<- struct{}, func(int64, string)) error {
	return nil
}

func (s *stubConn) Updates() <-chan store.Event { return nil }

func (s *stubConn) GetHistory(_ context.Context, _ domain.Peer, _ int, _ int) ([]domain.Message, error) {
	s.calls.Add(1)
	if s.release != nil {
		<-s.release
	}
	return s.history, nil
}

func newOwnerWithClient(t *testing.T, c Connection) (*Owner, *state.State) {
	t.Helper()
	s := state.New(store.NewMemory())
	cfg := &config.Config{}
	cfg.UI.HistoryLimit = 20
	o := New(cfg, zap.NewNop(), s, c, nopNotifier{})
	return o, s
}

func TestOwner_SubscribeDeliversInitialContents(t *testing.T) {
	o, events, st := newTestOwner(t)
	_ = events
	st.SetChat(domain.Chat{ID: 1, Title: "Ada"})

	id := o.Subscribe(project.ChatListWindow{Limit: 10})

	d, ok := recvDelta(t, o.Deltas())
	require.True(t, ok)
	assert.Equal(t, id, d.Sub)
	require.NotNil(t, d.ChatList)
	assert.Equal(t, project.ChatListReset, d.ChatList.Kind)
	require.Len(t, d.ChatList.Rows, 1)
	assert.Equal(t, "Ada", d.ChatList.Rows[0].Title)
}

func TestOwner_UnsubscribedClientGetsNothing(t *testing.T) {
	o, events, st := newTestOwner(t)
	st.SetChat(domain.Chat{ID: 1})
	id := o.Subscribe(project.ChatListWindow{Limit: 10})
	_, _ = recvDelta(t, o.Deltas())
	o.Unsubscribe(id)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventMuteUpdate, ChatID: 1, Muted: true}

	_, more := recvDelta(t, o.Deltas())
	assert.False(t, more, "an unsubscribed window must cost nothing")
}

func TestOwner_SubscribingToAnUnfilledChatWindowBackfills(t *testing.T) {
	c := &stubConn{history: []domain.Message{
		{ID: 1, ChatID: 7, Text: "old", Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 7, Text: "new", Date: time.Unix(2, 0)},
	}}
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7}})

	o.Subscribe(project.ChatWindow{
		ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 20,
	})

	// The initial delta is the empty window; the fetched history follows.
	_, _ = recvDelta(t, o.Deltas())
	d, ok := recvDelta(t, o.Deltas())
	require.True(t, ok, "the client must not have to know the store was empty")
	require.NotNil(t, d.Chat)
	assert.Len(t, d.Chat.Contents.Messages, 2)
}

func TestOwner_FullWindowDoesNotBackfill(t *testing.T) {
	c := &stubConn{}
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7}})
	s.Store().SetMessages(7, []domain.Message{
		{ID: 1, ChatID: 7, Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 7, Date: time.Unix(2, 0)},
		{ID: 3, ChatID: 7, Date: time.Unix(3, 0)},
	})

	o.Subscribe(project.ChatWindow{
		ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 1,
	})

	_, _ = recvDelta(t, o.Deltas())
	assert.Zero(t, c.calls.Load(), "the store already held everything the window asked for")
}

// Rapid scroll-up fires many window moves. Without the guard each one starts its
// own fetch and the overlapping pages stack into a repeating date range (#120).
func TestOwner_ConcurrentBackfillsCollapseToOneFetch(t *testing.T) {
	c := &stubConn{release: make(chan struct{})}
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7}})
	w := project.ChatWindow{ChatID: 7, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 20}
	id := o.Subscribe(w)

	for i := 0; i < 5; i++ {
		o.MoveWindow(id, project.ChatWindow{
			ChatID: 7, Anchor: w.Anchor, Before: 20 + i,
		})
	}
	// Let every goroutine reach the guard before releasing the first fetch.
	assert.Eventually(t, func() bool { return c.calls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	inFlight := c.calls.Load()
	close(c.release)

	assert.Equal(t, int32(1), inFlight, "one fetch per subscription may be in flight")
}
