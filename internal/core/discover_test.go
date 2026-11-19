package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// discoverConn answers the dialog lists the way Telegram does after a first
// message: the chat is in it, with the message already inside its counters.
type discoverConn struct {
	stubConn
	archived     []domain.Chat
	dialogsErr   error
	archivedCall atomic.Int32
}

func (c *discoverConn) GetDialogs(ctx context.Context) ([]domain.Chat, error) {
	if c.dialogsErr != nil {
		c.dialogCalls.Add(1)
		return nil, c.dialogsErr
	}
	return c.stubConn.GetDialogs(ctx)
}

func (c *discoverConn) GetArchivedDialogs(context.Context) ([]domain.Chat, error) {
	c.archivedCall.Add(1)
	return c.archived, nil
}

func newDiscoverOwner(t *testing.T, conn *discoverConn) (*Owner, chan store.Event, store.Store) {
	t.Helper()
	o, events, st := newTestOwner(t)
	o.client = conn
	o.discoverDelay = 0
	return o, events, st
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 2*time.Second, 5*time.Millisecond)
}

func TestDiscover_MessageForUnknownChatAddsItToTheList(t *testing.T) {
	conn := &discoverConn{}
	conn.dialogs = []domain.Chat{{
		ID: 7, Title: "Stranger", UnreadCount: 1,
		LastMessage: &domain.Message{ID: 5, ChatID: 7, Text: "hi"},
	}}
	o, events, st := newDiscoverOwner(t, conn)
	list := testClient(o)
	list.Subscribe(project.ChatListWindow{Limit: 10})
	_, _ = recvDelta(t, list)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 5, ChatID: 7, Text: "hi"}}

	eventually(t, func() bool { _, ok := st.GetChat(7); return ok })
	c, _ := st.GetChat(7)
	assert.Equal(t, 1, c.UnreadCount, "the dialog list's count already holds the message: not counted twice")
	require.NotNil(t, c.LastMessage)
	assert.Equal(t, 5, c.LastMessage.ID)
	assert.Equal(t, 5, st.TailMessageID(7), "the message that started it stays in the history")

	var rows int
	for {
		d, ok := recvDelta(t, list)
		if !ok {
			break
		}
		if d.ChatList != nil && d.ChatList.Kind == project.ChatListReset {
			rows = len(d.ChatList.Rows)
		}
	}
	assert.Equal(t, 1, rows, "the open chat list gets the new row without a restart")
}

// A first message sent from another device is outgoing: it adds the chat and
// counts as nothing unread.
func TestDiscover_OutgoingFirstMessageAddsChat(t *testing.T) {
	conn := &discoverConn{}
	conn.dialogs = []domain.Chat{{ID: 8, Title: "Friend"}}
	o, events, st := newDiscoverOwner(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 2, ChatID: 8, IsOut: true}}

	eventually(t, func() bool { _, ok := st.GetChat(8); return ok })
	c, _ := st.GetChat(8)
	assert.Zero(t, c.UnreadCount)
}

func TestDiscover_ArchivedFlagComesFromTheArchivedList(t *testing.T) {
	conn := &discoverConn{}
	conn.archived = []domain.Chat{{ID: 9, Title: "Quiet", IsArchived: true}}
	o, events, st := newDiscoverOwner(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 1, ChatID: 9}}

	eventually(t, func() bool { _, ok := st.GetChat(9); return ok })
	c, _ := st.GetChat(9)
	assert.True(t, c.IsArchived)
}

// A burst of first messages is one reload, and a chat that was known does not
// have its counters replaced by the list.
func TestDiscover_BurstIsOneReloadAndKnownChatsAreKept(t *testing.T) {
	conn := &discoverConn{}
	conn.dialogs = []domain.Chat{
		{ID: 1, Title: "Known", UnreadCount: 0},
		{ID: 10, Title: "A"}, {ID: 11, Title: "B"}, {ID: 12, Title: "C"},
	}
	o, events, st := newDiscoverOwner(t, conn)
	o.discoverDelay = 100 * time.Millisecond
	st.SetChat(domain.Chat{ID: 1, Title: "Known", UnreadCount: 4})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	for i, id := range []int64{10, 11, 12} {
		events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: i + 1, ChatID: id}}
	}

	eventually(t, func() bool { _, ok := st.GetChat(12); return ok })
	for _, id := range []int64{10, 11, 12} {
		_, ok := st.GetChat(id)
		assert.True(t, ok)
	}
	assert.EqualValues(t, 1, conn.dialogCalls.Load(), "one dialog list read answers the burst")
	known, _ := st.GetChat(1)
	assert.Equal(t, 4, known.UnreadCount, "a known chat keeps what the update stream counted")
}

// A chat in no dialog list is not looked for again on every message.
func TestDiscover_UnresolvedChatBacksOff(t *testing.T) {
	conn := &discoverConn{}
	o, events, st := newDiscoverOwner(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 1, ChatID: 77}}
	eventually(t, func() bool { return conn.dialogCalls.Load() == 1 })
	eventually(t, func() bool {
		o.discovery.mu.Lock()
		defer o.discovery.mu.Unlock()
		return !o.discovery.running
	})
	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 2, ChatID: 77}}
	// A later event the loop must reach proves the second one was handled.
	st.SetChat(domain.Chat{ID: 1})
	events <- store.Event{Kind: store.EventReadInbox, ChatID: 1, ReadMaxID: 1}
	time.Sleep(50 * time.Millisecond)

	assert.EqualValues(t, 1, conn.dialogCalls.Load())
}

func TestDiscover_FailedReloadIsLoggedAndRetriedLater(t *testing.T) {
	conn := &discoverConn{dialogsErr: errors.New("offline")}
	o, events, st := newDiscoverOwner(t, conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go o.RunUpdates(ctx)

	events <- store.Event{Kind: store.EventNewMessage, Message: domain.Message{ID: 1, ChatID: 5}}
	eventually(t, func() bool { return conn.dialogCalls.Load() == 1 })

	_, ok := st.GetChat(5)
	assert.False(t, ok)
	assert.Zero(t, conn.archivedCall.Load(), "no archived read after a failed main read")
}
