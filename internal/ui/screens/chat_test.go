package screens_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatModel_LoadError_ShownWhenNotLoading(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetLoading(false)
	m.SetLoadError("load history failed: timeout")
	assert.Contains(t, m.View(), "load history failed: timeout")
}

func TestChatModel_LoadError_ClearedByEmptyString(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetLoadError("boom")
	m.SetLoadError("")
	assert.NotContains(t, m.View(), "boom")
}

func TestChat_View_ShowsMessages(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "hello world", Date: time.Now()},
	})
	assert.Contains(t, m.View(), "hello world")
}

func TestChatModel_SelectedBubbleRect_AfterView(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now()}})
	m.View() // populates the rect cache

	rect, ok := m.SelectedBubbleRect()
	require.True(t, ok)
	assert.Equal(t, 0, rect.Left) // incoming
	assert.Greater(t, rect.Width, 0)
	assert.Greater(t, m.MessageListHeight(), 0)
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

func TestChat_SetEdit_SetsState(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetEdit(7, "▌ Edit Message\n▌ hello")
	assert.Equal(t, 7, m.EditMsgID())
}

func TestChat_ActionNormal_ClearsEditState(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetEdit(7, "▌ Edit Message\n▌ hello")
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionNormal})
	m = newPane.(*screens.ChatModel)
	assert.Equal(t, 0, m.EditMsgID())
	assert.False(t, m.ComposerFocused())
}

func TestChat_EditMode_EmitsEditSendRequest(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetEdit(5, "▌ Edit Message\n▌ original")
	m.SetComposerValue("edited text")
	newPane, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = newPane
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.EditSendRequest)
	require.True(t, ok)
	assert.Equal(t, 5, req.MsgID)
	assert.Equal(t, "edited text", req.Text)
}

func TestChat_EditMode_ClearsStateAfterSend(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetEdit(5, "▌ Edit Message\n▌ original")
	m.SetComposerValue("edited text")
	newPane, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = newPane.(*screens.ChatModel)
	assert.Equal(t, 0, m.EditMsgID())
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

func TestChat_AltEnter_DoesNotSend(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")

	newPane, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	_ = newPane
	require.NotNil(t, cmd, "alt+enter should produce a cmd (textarea handled the key)")
	msg := cmd()
	_, isSend := msg.(screens.SendMsgRequest)
	assert.False(t, isSend, "alt+enter must not send message")
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

func TestChatModel_PasteMsg_InsertsTextIntoComposer(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	require.True(t, m.ComposerFocused())

	newPane, _ = m.Update(tea.PasteMsg{Content: "hello world"})
	m = newPane.(*screens.ChatModel)

	assert.Equal(t, "hello world", m.ComposerValue())
}

func TestChatModel_LoadingView_ShowsSpinner(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetLoading(true)
	view := m.View()
	assert.Contains(t, view, "Loading...")
	assert.Contains(t, view, "[=") // spinner frame present somewhere in centered output
}

func TestChatModel_NotLoading_NoSpinner(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetLoading(false)
	view := m.View()
	assert.NotContains(t, view, "Loading...")
}

func TestChatModel_TypingLabel_EmptyByDefault(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	assert.Equal(t, "", m.TypingLabel())
}

func TestChatModel_SetTypingLabel_ShowsInLabel(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetTypingLabel("typing")
	label := m.TypingLabel()
	assert.True(t, strings.HasPrefix(label, "typing"), "got %q", label)
	assert.Equal(t, len("typing")+3, len(label), "dots suffix must be 3 chars")
}

func TestChatModel_ClearTypingLabel_ResetsLabel(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetTypingLabel("typing")
	m.ClearTypingLabel()
	assert.Equal(t, "", m.TypingLabel())
}

func TestChatModel_IsTyping_FalseByDefault(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	assert.False(t, m.IsTyping())
}

func TestChatModel_IsTyping_TrueAfterSet(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetTypingLabel("recording audio")
	assert.True(t, m.IsTyping())
}

func TestChatModel_TickTypingDots_ChangesLabel(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	m.SetTypingLabel("typing")
	before := m.TypingLabel()
	m.TickTypingDots()
	assert.NotEqual(t, before, m.TypingLabel())
}

// collectBatchMsgs executes a tea.Cmd and, if it returns a tea.BatchMsg,
// recursively collects all leaf messages.
func collectBatchMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, collectBatchMsgs(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestChatModel_Typing_EmitsSetTypingRequest_OnKeystroke(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello") // ensure Value() != "" regardless of textarea behaviour

	newPane, cmd := m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	_ = newPane
	require.NotNil(t, cmd)
	msgs := collectBatchMsgs(cmd)
	var found bool
	for _, msg := range msgs {
		if req, ok := msg.(screens.SetTypingRequest); ok {
			assert.Equal(t, store.TypingActionTyping, req.Action)
			assert.Equal(t, int64(10), req.Peer.ID)
			found = true
		}
	}
	assert.True(t, found, "expected SetTypingRequest in batch, got: %v", msgs)
}

func TestChatModel_Typing_NoRequestIfNoChatSet(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	// no SetChat call
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")

	newPane, cmd := m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	_ = newPane
	for _, msg := range collectBatchMsgs(cmd) {
		_, isTyping := msg.(screens.SetTypingRequest)
		assert.False(t, isTyping, "no SetTypingRequest when chat is nil")
	}
}

func TestChatModel_Typing_ThrottlesTo4Seconds(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")

	// First keypress → must emit SetTypingRequest
	newPane, cmd1 := m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	m = newPane.(*screens.ChatModel)
	msgs1 := collectBatchMsgs(cmd1)
	var first bool
	for _, msg := range msgs1 {
		if _, ok := msg.(screens.SetTypingRequest); ok {
			first = true
		}
	}
	assert.True(t, first, "first keystroke must emit SetTypingRequest")

	// Second keypress immediately after → throttled (lastTypingAt just set)
	newPane, cmd2 := m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	_ = newPane
	for _, msg := range collectBatchMsgs(cmd2) {
		_, ok2 := msg.(screens.SetTypingRequest)
		assert.False(t, ok2, "second keystroke within 4s must NOT emit SetTypingRequest")
	}
}

func TestChatModel_Typing_CancelOnSend(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")
	// type a character to arm lastTypingAt
	newPane, _ = m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	m = newPane.(*screens.ChatModel)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msgs := collectBatchMsgs(cmd)
	var found bool
	for _, msg := range msgs {
		if req, ok := msg.(screens.SetTypingRequest); ok && req.Action == store.TypingActionCancel {
			found = true
		}
	}
	assert.True(t, found, "send after typing must emit TypingActionCancel; got: %v", msgs)
}

func TestChatModel_Typing_CancelOnEscape(t *testing.T) {
	m := screens.NewChatModel(80, 24)
	chat := &store.Chat{ID: 10, Peer: store.Peer{ID: 10, Type: store.PeerUser}}
	m.SetChat(chat)
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionInsert})
	m = newPane.(*screens.ChatModel)
	m.SetComposerValue("hello")
	// arm lastTypingAt
	newPane, _ = m.Update(tea.KeyPressMsg{Code: '!', Text: "!"})
	m = newPane.(*screens.ChatModel)

	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionNormal})
	require.NotNil(t, cmd)
	msg := cmd()
	req, ok := msg.(screens.SetTypingRequest)
	require.True(t, ok, "escape after typing must emit SetTypingRequest, got %T", msg)
	assert.Equal(t, store.TypingActionCancel, req.Action)
}
