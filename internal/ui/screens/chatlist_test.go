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
