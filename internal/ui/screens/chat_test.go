package screens_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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
	newPane, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = newPane
	require.NotNil(t, cmd)
	msg := cmd()
	req, ok := msg.(screens.SendMsgRequest)
	assert.True(t, ok)
	assert.Equal(t, "hello", req.Text)
}

func TestChatModel_LoadMoreMsg_OnUpAtTop(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 42, Title: "Test"}
	m.SetChat(chat)
	msgs := make([]store.Message, 3)
	for i := range msgs {
		msgs[i] = store.Message{ID: i + 1, ChatID: 42, Text: "msg", Date: time.Now()}
	}
	m.SetMessages(msgs)
	// 3 messages in ~20-row window → viewStart=0 → AtTop()
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionUp})
	require.NotNil(t, cmd)
	msg := cmd()
	lm, ok := msg.(screens.LoadMoreMsg)
	require.True(t, ok, "expected LoadMoreMsg, got %T", msg)
	assert.Equal(t, int64(42), lm.ChatID)
	assert.Equal(t, 1, lm.OffsetID) // oldest message ID
}

func TestChatModel_LoadMoreMsg_OnGoTop(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Title: "X"}
	m.SetChat(chat)
	msgs := []store.Message{{ID: 5, ChatID: 10, Text: "hi", Date: time.Now()}}
	m.SetMessages(msgs)
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionGoTop})
	require.NotNil(t, cmd)
	msg := cmd()
	lm, ok := msg.(screens.LoadMoreMsg)
	require.True(t, ok)
	assert.Equal(t, int64(10), lm.ChatID)
	assert.Equal(t, 5, lm.OffsetID)
}

func TestChatModel_NoLoadMore_WhenNotAtTop(t *testing.T) {
	m := screens.NewChatModel(80, 3) // height=3 so viewport is small
	chat := &store.Chat{ID: 1, Title: "Y"}
	m.SetChat(chat)
	msgs := make([]store.Message, 10)
	for i := range msgs {
		msgs[i] = store.Message{ID: i + 1, ChatID: 1, Text: "m", Date: time.Now()}
	}
	m.SetMessages(msgs) // with height=3, viewStart = 10-3=7 → not at top
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionUp})
	if cmd != nil {
		msg := cmd()
		_, isLoadMore := msg.(screens.LoadMoreMsg)
		assert.False(t, isLoadMore)
	}
}
