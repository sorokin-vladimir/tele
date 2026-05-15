package screens_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func makeTestChats() []store.Chat {
	return []store.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
		{ID: 3, Title: "Charlie"},
	}
}

func TestChatList_View_ShowsChats(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats(makeTestChats())
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Bob")
}

func TestChatList_Down_MovesCursor(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	cl := newPane.(*screens.ChatListModel)
	assert.Equal(t, 1, cl.Cursor())
}

func TestChatList_Confirm_EmitsOpenChat(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats())
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.IsType(t, screens.OpenChatMsg{}, msg)
	assert.Equal(t, int64(1), msg.(screens.OpenChatMsg).Chat.ID)
}

func TestChatList_Context(t *testing.T) {
	m := screens.NewChatListModel()
	assert.Equal(t, keys.ContextChatList, m.Context())
}

func TestChatList_ShowsUnreadBadge(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats([]store.Chat{
		{ID: 1, Title: "Alice", UnreadCount: 3},
		{ID: 2, Title: "Bob", UnreadCount: 0},
	})
	view := m.View()
	assert.Contains(t, view, "[3]")
	assert.NotContains(t, view, "[0]")
}

func TestChatList_Badge99Plus(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats([]store.Chat{{ID: 1, Title: "Spam", UnreadCount: 150}})
	view := m.View()
	assert.Contains(t, view, "[99+]")
}

func TestChatList_NoBadgeWhenZero(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats([]store.Chat{{ID: 1, Title: "Quiet", UnreadCount: 0}})
	view := m.View()
	assert.NotContains(t, view, "[0]")
}

func TestChatList_SetChats_PreservesCursorByID(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats()) // [A(1), B(2), C(3)], cursor at 0 (A)
	assert.Equal(t, 0, m.Cursor())

	// Reorder: A moves to index 1. Cursor must follow A to index 1.
	m.SetChats([]store.Chat{
		{ID: 2, Title: "Bob"},
		{ID: 1, Title: "Alice"},
		{ID: 3, Title: "Charlie"},
	})
	assert.Equal(t, 1, m.Cursor()) // cursor followed A(id=1) to its new position
}

func TestChatList_SetChats_CursorClampsWhenChatRemoved(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats()) // [A(1), B(2), C(3)]
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionDown}) // cursor -> 2 (C)
	m = newPane.(*screens.ChatListModel)
	assert.Equal(t, 2, m.Cursor())

	m.SetChats([]store.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
	})
	assert.Equal(t, 0, m.Cursor())
}
