package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

type mockNotifier struct {
	calls []struct{ title, body string }
}

func (m *mockNotifier) Notify(title, body string) error {
	m.calls = append(m.calls, struct{ title, body string }{title, body})
	return nil
}

// focusOn builds the focus input the policy takes: the chats some client has
// open. Passing none means nobody is looking at anything.
func focusOn(ids ...int64) func(domain.HistoryKey) bool {
	return func(h domain.HistoryKey) bool {
		for _, want := range ids {
			if h.ChatID == want && want != 0 {
				return true
			}
		}
		return false
	}
}

func longText(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

func TestDecideNotification(t *testing.T) {
	now := time.Now()
	fresh := now.Add(-time.Second)
	stale := now.Add(-time.Minute)

	tests := []struct {
		name      string
		chats     []domain.Chat
		evt       store.Event
		focused   func(domain.HistoryKey) bool
		preview   bool
		wantOK    bool
		wantTitle string
		wantBody  string
	}{
		{
			name:  "fresh message in an unfocused chat notifies",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "hello there", Date: fresh}},
			focused: focusOn(1), preview: true,
			wantOK: true, wantTitle: "Bob", wantBody: "hello there",
		},
		{
			name:  "preview off hides the text",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "secret text", Date: fresh}},
			focused: focusOn(), preview: false,
			wantOK: true, wantTitle: "Bob", wantBody: "New message",
		},
		{
			name:  "long text is truncated to 100 runes",
			chats: []domain.Chat{{ID: 2, Title: "C"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: longText(200), Date: fresh}},
			focused: focusOn(), preview: true,
			wantOK: true, wantTitle: "C", wantBody: longText(100) + "…",
		},
		{
			name:  "the focused chat stays silent",
			chats: []domain.Chat{{ID: 1, Title: "Alice"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 1, Text: "hey", Date: fresh}},
			focused: focusOn(1), preview: true,
		},
		{
			name:  "outgoing messages never notify",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "sent by me", IsOut: true, Date: fresh}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "muted chats stay silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob", IsMuted: true}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "hey", Date: fresh}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "archived chats stay silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob", IsArchived: true}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "hey", Date: fresh}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "catch-up backlog is stale and stays silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "old", Date: stale}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a zero event time stays silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "no date"}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a chat the store does not know stays silent",
			chats: nil,
			evt: store.Event{Kind: store.EventNewMessage,
				Message: domain.Message{ChatID: 2, Text: "hey", Date: fresh}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a group reaction to our message notifies",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 2, MsgID: 10,
				ReactionsUnread: new(true), ReactionEmoji: "❤", ReactionDate: fresh},
			focused: focusOn(), preview: true,
			wantOK: true, wantTitle: "Bob", wantBody: "reacted ❤ to your message",
		},
		{
			name:  "a custom-emoji reaction degrades to a bare body",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 2, MsgID: 10,
				ReactionsUnread: new(true), ReactionDate: fresh},
			focused: focusOn(), preview: true,
			wantOK: true, wantTitle: "Bob", wantBody: "reacted to your message",
		},
		{
			name:  "a reactions update with nothing unread stays silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 2, MsgID: 10,
				ReactionEmoji: "❤", ReactionDate: fresh},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a 1:1 reaction arrives as a hidden edit and notifies",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventEditMessage,
				Message:       domain.Message{ChatID: 2, ID: 10, IsOut: true, HasUnreadReactions: true},
				ReactionEmoji: "👍", ReactionDate: fresh},
			focused: focusOn(), preview: true,
			wantOK: true, wantTitle: "Bob", wantBody: "reacted 👍 to your message",
		},
		{
			name:  "a genuine text edit never notifies",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventEditMessage,
				Message: domain.Message{ChatID: 2, ID: 10, Text: "edited text"}},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a reaction in the focused chat stays silent",
			chats: []domain.Chat{{ID: 1, Title: "Alice"}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 1,
				ReactionsUnread: new(true), ReactionEmoji: "❤", ReactionDate: fresh},
			focused: focusOn(1), preview: true,
		},
		{
			name:  "a reaction in a muted chat stays silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob", IsMuted: true}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 2,
				ReactionsUnread: new(true), ReactionEmoji: "❤", ReactionDate: fresh},
			focused: focusOn(), preview: true,
		},
		{
			name:  "a stale reaction stays silent",
			chats: []domain.Chat{{ID: 2, Title: "Bob"}},
			evt: store.Event{Kind: store.EventReactionsUpdate, ChatID: 2,
				ReactionsUnread: new(true), ReactionEmoji: "❤", ReactionDate: stale},
			focused: focusOn(), preview: true,
		},
		{
			name:    "an unrelated event kind never notifies",
			chats:   []domain.Chat{{ID: 2, Title: "Bob"}},
			evt:     store.Event{Kind: store.EventDeleteMessages, ChatID: 2},
			focused: focusOn(), preview: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemory()
			for _, c := range tc.chats {
				st.SetChat(c)
			}
			n, ok := decideNotification(st, tc.evt, tc.focused, tc.preview, now)
			if !tc.wantOK {
				assert.False(t, ok)
				assert.Equal(t, Notification{}, n, "a refused decision carries nothing")
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.wantTitle, n.Title)
			assert.Equal(t, tc.wantBody, n.Body)
		})
	}
}

func TestDecideNotification_CarriesTheChatItIsAbout(t *testing.T) {
	// The client opens this chat when the toast is clicked, so the id has to
	// survive the decision.
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 42, Title: "Bob"})
	evt := store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ChatID: 42, Text: "hey", Date: time.Now()}}
	n, ok := decideNotification(st, evt, focusOn(), true, time.Now())
	require.True(t, ok)
	assert.Equal(t, int64(42), n.ChatID)
}

func TestDecideNotification_OneFocusedClientOfTwoIsEnough(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 7, Title: "Bob"})
	evt := store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ChatID: 7, Text: "hey", Date: time.Now()}}

	_, ok := decideNotification(st, evt, focusOn(9, 7), true, time.Now())
	assert.False(t, ok, "somebody is looking at the chat, so nobody needs interrupting")

	_, ok = decideNotification(st, evt, focusOn(9), true, time.Now())
	assert.True(t, ok, "no client is showing it any more")
}
