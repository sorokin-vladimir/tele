package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// The owner reads its settings where it acts on them, so installing a reloaded
// config is the whole of applying it. These show that it is: each changes one
// setting with SetConfig, which is what a reload hands the owner, and checks
// that the next thing the owner does follows it (#239).

// arrive delivers the message with the given id from Bob, with the test client
// attached to hear it.
func arrive(o *Owner, id int) {
	testClient(o)
	o.handleEvent(store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ID: id, ChatID: 2, Text: "hello there", Date: time.Now()}})
}

// toasted reports whether a notification was published for a client to draw,
// and takes it off the stream.
func toasted(o *Owner) (Notification, bool) {
	ns := only[Notification](queued(testClient(o)))
	if len(ns) == 0 {
		return Notification{}, false
	}
	return ns[0], true
}

func TestTakesHold_ui_notifications_desktop(t *testing.T) {
	n := &mockNotifier{}
	o, st := newTestOwnerNotified(t, n)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	arrive(o, 1)
	require.Len(t, n.calls, 1)

	o.SetConfig(notifyConfig(false, true))
	arrive(o, 2)

	assert.Len(t, n.calls, 1, "the next message reached the operating system after it was switched off")
}

func TestTakesHold_ui_notifications_toast(t *testing.T) {
	o, st := newTestOwnerNotified(t, &mockNotifier{})
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	arrive(o, 1)
	_, ok := toasted(o)
	require.True(t, ok)

	o.SetConfig(notifyConfig(true, false))
	arrive(o, 2)

	_, ok = toasted(o)
	assert.False(t, ok, "the next message was drawn as a toast after toasts were switched off")
}

func TestTakesHold_ui_notifications_preview(t *testing.T) {
	n := &mockNotifier{}
	o, st := newTestOwnerNotified(t, n)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	arrive(o, 1)
	require.Len(t, n.calls, 1)
	require.Equal(t, "hello there", n.calls[0].body)

	cfg := notifyConfig(true, true)
	cfg.UI.Notifications.Preview = false
	o.SetConfig(cfg)
	arrive(o, 2)

	require.Len(t, n.calls, 2)
	assert.NotContains(t, n.calls[1].body, "hello there", "the next notification still carried the text")
}

// limitConn answers a history fetch with nothing and remembers how much was
// asked for.
type limitConn struct {
	internaltg.Client
	limits []int
}

func (c *limitConn) Connect(context.Context, *internaltg.AuthFlow, chan<- struct{}, func(domain.User)) error {
	return nil
}

func (c *limitConn) Updates() <-chan store.Event { return nil }

func (c *limitConn) GetHistory(_ context.Context, _ domain.Peer, _ int, limit int) ([]domain.Message, error) {
	c.limits = append(c.limits, limit)
	return nil, nil
}

func TestTakesHold_ui_history_limit(t *testing.T) {
	historyLimit := func(n int) *config.Config {
		cfg := &config.Config{}
		cfg.UI.HistoryLimit = n
		return cfg
	}
	st := store.NewMemory()
	conn := &limitConn{}
	o := New(historyLimit(50), zap.NewNop(), state.New(st), conn, nopNotifier{})
	chat := domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}}
	st.SetChat(chat)
	o.tailReload(context.Background(), chat)

	o.SetConfig(historyLimit(120))
	o.tailReload(context.Background(), chat)

	assert.Equal(t, []int{50, 120}, conn.limits, "the next fetch did not ask for the new limit")
}
