package screens_test

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func TestChat_View_ShowsMessages(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "hello world", Date: time.Now()},
	})
	assert.Contains(t, m.View(), "hello world")
}

func TestChat_Insert_FocusesComposer(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	cm := newPane.(*screens.ChatModel)
	assert.True(t, cm.ComposerFocused())
}

func TestChat_Context(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	assert.Equal(t, keys.ContextChat, m.Context())
}

func TestChat_SendMessage_EmitsRequest(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	// focus composer
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	// type text via SetValue shortcut
	m.SetComposerValue("hello")
	// press enter
	newPane, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = newPane
	require.NotNil(t, cmd)
	msg := cmd()
	req, ok := msg.(screens.SendMsgRequest)
	assert.True(t, ok)
	assert.Equal(t, "hello", req.Text)
}
