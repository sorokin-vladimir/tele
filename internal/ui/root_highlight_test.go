package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rootOnOpenChatWithMsg builds a root on an open chat (id 1) holding one
// incoming message with the given id, ready for jump-to-highlight tests.
func rootOnOpenChatWithMsg(t *testing.T, msgID int) RootModel {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ScreenMain)
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(RootModel)
	newM, _ = m.Update(store.Event{Kind: store.EventNewMessage,
		Message: store.Message{ID: msgID, ChatID: 1, Text: "target", Date: time.Now()}})
	return newM.(RootModel)
}

func TestRoot_JumpToMsg_StartsHighlight(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 5)

	newM, cmd := m.Update(components.JumpToMsgRequest{MsgID: 5})
	root := newM.(RootModel)

	assert.Equal(t, 5, root.Chat().HighlightedMsgID())
	assert.Equal(t, components.HighlightInitialStep, root.Chat().HighlightStep())
	require.NotNil(t, cmd, "a fade tick command should be scheduled")
}

func TestRoot_MsgHighlightFade_DecrementsOnTick(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 5)
	newM, _ := m.Update(components.JumpToMsgRequest{MsgID: 5})
	m = newM.(RootModel)
	require.Equal(t, components.HighlightInitialStep, m.Chat().HighlightStep())

	// One fade tick (matching serial) decrements the step.
	newM, cmd := m.Update(msgHighlightFadeMsg{serial: m.msgHighlightSerial})
	m = newM.(RootModel)
	assert.Equal(t, components.HighlightInitialStep-1, m.Chat().HighlightStep())
	assert.NotNil(t, cmd, "still active, so another tick is scheduled")
}

func TestRoot_MsgHighlightFade_StaleSerialIgnored(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 5)
	newM, _ := m.Update(components.JumpToMsgRequest{MsgID: 5})
	m = newM.(RootModel)
	before := m.Chat().HighlightStep()

	// A tick from a superseded highlight (wrong serial) must not decrement.
	newM, cmd := m.Update(msgHighlightFadeMsg{serial: m.msgHighlightSerial - 1})
	m = newM.(RootModel)
	assert.Equal(t, before, m.Chat().HighlightStep())
	assert.Nil(t, cmd)
}

func TestRoot_IncomingMsg_HighlightsNonOpenChat(t *testing.T) {
	m, _ := newRootWithTwoChatsInternal(t)

	newM, cmd := m.Update(store.Event{Kind: store.EventNewMessage,
		Message: store.Message{ID: 1, ChatID: 2, Text: "hi", Date: time.Now()}})
	root := newM.(RootModel)

	assert.Equal(t, int64(2), root.ChatList().HighlightedChatID())
	assert.Equal(t, components.HighlightInitialStep, root.ChatList().HighlightStep())
	require.NotNil(t, cmd, "a chat fade tick should be scheduled")
}

func TestRoot_IncomingMsg_NoHighlightForOpenChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ScreenMain)
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(RootModel)

	newM, _ = m.Update(store.Event{Kind: store.EventNewMessage,
		Message: store.Message{ID: 9, ChatID: 1, Text: "hi", Date: time.Now()}})
	root := newM.(RootModel)

	assert.Equal(t, int64(0), root.ChatList().HighlightedChatID(),
		"the currently open chat must not be highlighted")
}

func TestRoot_IncomingMsg_NoHighlightForOutgoing(t *testing.T) {
	m, _ := newRootWithTwoChatsInternal(t)

	newM, _ := m.Update(store.Event{Kind: store.EventNewMessage,
		Message: store.Message{ID: 1, ChatID: 2, Text: "hi", IsOut: true, Date: time.Now()}})
	root := newM.(RootModel)

	assert.Equal(t, int64(0), root.ChatList().HighlightedChatID(),
		"an outgoing message must not highlight the chat row")
}

func TestRoot_ChatHighlightFade_DecrementsAndStaleIgnored(t *testing.T) {
	m, _ := newRootWithTwoChatsInternal(t)
	newM, _ := m.Update(store.Event{Kind: store.EventNewMessage,
		Message: store.Message{ID: 1, ChatID: 2, Text: "hi", Date: time.Now()}})
	m = newM.(RootModel)

	// Matching serial decrements.
	newM, cmd := m.Update(chatHighlightFadeMsg{serial: m.chatHighlightSerial})
	m = newM.(RootModel)
	assert.Equal(t, components.HighlightInitialStep-1, m.ChatList().HighlightStep())
	assert.NotNil(t, cmd)

	// Stale serial is ignored.
	before := m.ChatList().HighlightStep()
	newM, cmd = m.Update(chatHighlightFadeMsg{serial: m.chatHighlightSerial - 1})
	m = newM.(RootModel)
	assert.Equal(t, before, m.ChatList().HighlightStep())
	assert.Nil(t, cmd)
}

// drainBatch expands a tea.BatchMsg into the messages its commands produce; a
// non-batch message is returned as a single-element slice.
func drainBatch(msg tea.Msg) []tea.Msg {
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, c())
	}
	return out
}

func TestRoot_DeleteMsgFailed_StartsErrorHighlight(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 5) // open chat is 1
	restored := []store.Message{{ID: 5, ChatID: 1, Text: "target", Date: time.Now()}}

	newM, cmd := m.Update(deleteMsgFailedMsg{chatID: 1, msgID: 5, messages: restored})
	root := newM.(RootModel)

	assert.Equal(t, 5, root.Chat().HighlightedMsgID())
	assert.Equal(t, components.HighlightError, root.Chat().HighlightKind())
	assert.Equal(t, components.HighlightInitialStep, root.Chat().HighlightStep())
	require.NotNil(t, cmd)

	var sawToast bool
	for _, mm := range drainBatch(cmd()) {
		if _, ok := mm.(StatusErrMsg); ok {
			sawToast = true
		}
	}
	assert.True(t, sawToast, "the failure toast must still be emitted alongside the highlight")
}

func TestRoot_EditMsgFailed_StartsErrorHighlight(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 7)
	restored := []store.Message{{ID: 7, ChatID: 1, Text: "orig", Date: time.Now()}}

	newM, cmd := m.Update(editMsgFailedMsg{chatID: 1, msgID: 7, messages: restored})
	root := newM.(RootModel)

	assert.Equal(t, 7, root.Chat().HighlightedMsgID())
	assert.Equal(t, components.HighlightError, root.Chat().HighlightKind())
	require.NotNil(t, cmd)
}

func TestRoot_MsgFailed_NoHighlightForOtherChat(t *testing.T) {
	m := rootOnOpenChatWithMsg(t, 5) // open chat is 1
	restored := []store.Message{{ID: 5, ChatID: 2, Text: "x", Date: time.Now()}}

	newM, cmd := m.Update(deleteMsgFailedMsg{chatID: 2, msgID: 5, messages: restored})
	root := newM.(RootModel)

	assert.Equal(t, 0, root.Chat().HighlightedMsgID(),
		"a rollback in another chat must not highlight the open chat")
	require.NotNil(t, cmd, "the failure toast is still emitted")
}

// newRootWithTwoChatsInternal mirrors newRootWithTwoChats from root_test.go for
// use inside package ui (where that external helper is not visible).
func newRootWithTwoChatsInternal(t *testing.T) (RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	m := NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	return newM.(RootModel), st
}
