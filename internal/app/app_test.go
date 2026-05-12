package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
)

type mockNotifier struct {
	calls []struct{ title, body string }
}

func (m *mockNotifier) Notify(title, body string) error {
	m.calls = append(m.calls, struct{ title, body string }{title, body})
	return nil
}

func TestMaybeNotify_SendsForOtherChat(t *testing.T) {
	n := &mockNotifier{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "hello there"},
	}
	maybeNotify(n, st, evt, 1)
	require.Len(t, n.calls, 1)
	assert.Equal(t, "Bob", n.calls[0].title)
	assert.Equal(t, "hello there", n.calls[0].body)
}

func TestMaybeNotify_SilentForOpenChat(t *testing.T) {
	n := &mockNotifier{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 1, Text: "hey"},
	}
	maybeNotify(n, st, evt, 1)
	assert.Empty(t, n.calls)
}

func TestMaybeNotify_TruncatesLongText(t *testing.T) {
	n := &mockNotifier{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 2, Title: "C"})
	b := make([]byte, 200)
	for i := range b {
		b[i] = 'x'
	}
	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: string(b)},
	}
	maybeNotify(n, st, evt, 0)
	require.Len(t, n.calls, 1)
	body := n.calls[0].body
	runes := []rune(body)
	assert.Equal(t, 101, len(runes))           // 100 chars + U+2026 ellipsis
	assert.Equal(t, "…", string(runes[100:])) // last rune is ellipsis
}

func TestMaybeNotify_IgnoresNonMessageEvents(t *testing.T) {
	n := &mockNotifier{}
	st := store.NewMemory()
	evt := store.Event{Kind: store.EventKind(99)}
	maybeNotify(n, st, evt, 0)
	assert.Empty(t, n.calls)
}
