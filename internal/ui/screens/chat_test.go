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

func TestChat_Indicator_VisibleByDefault(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now()}})
	assert.Contains(t, m.View(), "┃")
}

func TestChat_Indicator_HiddenWhenComposerFocused(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now()}})

	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	assert.NotContains(t, m.View(), "┃")

	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionNormal})
	m = newPane.(*screens.ChatModel)
	assert.Contains(t, m.View(), "┃")
}

func TestChat_SelectedMessageID_ReturnsLastVisible(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "first", Date: time.Now()},
		{ID: 2, ChatID: 1, Text: "second", Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "third", Date: time.Now()},
	})
	assert.Equal(t, 3, m.SelectedMessageID())
}

func TestChat_SelectedMessageIsOut_Outgoing(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", IsOut: true, Date: time.Now()}})
	assert.True(t, m.SelectedMessageIsOut())
}

func TestChat_SelectedMessageIsOut_Incoming(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", IsOut: false, Date: time.Now()}})
	assert.False(t, m.SelectedMessageIsOut())
}

func TestChat_SelectedMessageIsOut_NoMessages(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	assert.False(t, m.SelectedMessageIsOut())
}

func TestChat_SetReply_SetsState(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetReply(10, "▌ Alice\n▌ hello")
	assert.Equal(t, 10, m.ReplyToMsgID())
}

func TestChat_ClearPendingAction_ZerosState(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetReply(10, "▌ Alice\n▌ hello")
	m.ClearPendingAction()
	assert.Equal(t, 0, m.ReplyToMsgID())
}

func TestChat_ActionNormal_ClearsReplyState(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	// enter composer
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetReply(10, "▌ Alice\n▌ hello")
	// press Esc
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionNormal})
	m = newPane.(*screens.ChatModel)
	assert.Equal(t, 0, m.ReplyToMsgID())
	assert.False(t, m.ComposerFocused())
}

func TestChat_SendMessage_CarriesReplyToMsgID(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetReply(5, "▌ Bob\n▌ original")
	m.SetComposerValue("my reply")
	newPane, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = newPane
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.SendMsgRequest)
	require.True(t, ok)
	assert.Equal(t, "my reply", req.Text)
	assert.Equal(t, 5, req.ReplyToMsgID)
}

func TestChat_SendMessage_ClearsReplyStateAfterSend(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetReply(5, "▌ Bob\n▌ original")
	m.SetComposerValue("my reply")
	newPane, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newPane.(*screens.ChatModel)
	assert.Equal(t, 0, m.ReplyToMsgID())
}

func TestChat_ShiftEnter_DoesNotSend(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")

	newPane, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	_ = newPane
	require.NotNil(t, cmd, "shift+enter should produce a cmd, not nil")
	msg := cmd()
	_, isSend := msg.(screens.SendMsgRequest)
	assert.False(t, isSend, "shift+enter must not send message")
}
