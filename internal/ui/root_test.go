package ui_test

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type mockTGClient struct {
	history  []store.Message
	sendFunc func() int
}

func (m *mockTGClient) GetDialogs(_ context.Context) ([]store.Chat, error) { return nil, nil }
func (m *mockTGClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	return m.history, nil
}
func (m *mockTGClient) SendMessage(_ context.Context, _ store.Peer, _ string) (int, error) {
	if m.sendFunc != nil {
		return m.sendFunc(), nil
	}
	return 42, nil
}
func (m *mockTGClient) MarkRead(_ context.Context, _ store.Peer, _ int) error { return nil }
func (m *mockTGClient) Updates() <-chan store.Event { return make(chan store.Event) }

var _ internaltg.Client = (*mockTGClient)(nil)

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_2_FocusesChat(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_1_FocusesChatList(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_TransitionToMain(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.ScreenMain, root.CurrentScreen())
}

func TestRoot_CtrlC_Quits(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.NotNil(t, cmd)
	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestRoot_LoadMoreMsg_DispatchesGetHistory(t *testing.T) {
	mock := &mockTGClient{}
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Set current chat to 1 by sending OpenChatMsg
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(ui.RootModel)

	newM, cmd := m.Update(screens.LoadMoreMsg{ChatID: 1, OffsetID: 5})
	_ = newM
	require.NotNil(t, cmd)
	// cmd should trigger a GetHistory call — verify it returns a non-nil message
	result := cmd()
	assert.NotNil(t, result)
}

func TestRoot_SlashKey_ActivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	root := newM.(ui.RootModel)
	assert.True(t, root.SearchActive())
}

func TestRoot_CloseSearchMsg_DeactivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	require.True(t, m.SearchActive())
	newM, _ = m.Update(screens.CloseSearchMsg{})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}

func TestRoot_SearchOpenChatMsg_ClosesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	newM, _ = m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}

func newRootWithTwoChats(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	return newM.(ui.RootModel), st
}

func TestRoot_NewMessageEvent_UpdatesChatList(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "hi", Date: time.Now()},
	}
	newM, _ := m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	require.Len(t, chats, 2)
	assert.Equal(t, int64(2), chats[0].ID, "chat 2 should bubble to top after new message")
}

func TestRoot_NewMessageEvent_IncrementsUnread(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "hi"},
	}
	newM, _ := m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat2 store.Chat
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 1, chat2.UnreadCount)
}

func TestRoot_NewMessageEvent_UnreadPersistsAcrossMultipleEvents(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "first"},
	}
	newM, _ := m.Update(evt)
	m = newM.(ui.RootModel)

	evt2 := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "second"},
	}
	newM, _ = m.Update(evt2)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat2 store.Chat
	for _, c := range chats {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 2, chat2.UnreadCount, "unread count should accumulate across multiple new-message events")
}

func TestRoot_NewMessageEvent_NoUnreadForCurrentChat(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 1, Text: "hi"},
	}
	newM, _ = m.Update(evt)
	root := newM.(ui.RootModel)

	chats := root.ChatList().Chats()
	var chat1 store.Chat
	for _, c := range chats {
		if c.ID == 1 {
			chat1 = c
		}
	}
	assert.Equal(t, 0, chat1.UnreadCount)
}

func newRootWithOpenChat(t *testing.T, mock *mockTGClient) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(mock, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	return newM.(ui.RootModel), st
}

func TestRoot_Send_ShowsSentinelImmediately(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	_, _ = m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Less(t, msgs[0].ID, 0, "sentinel should have a negative ID")
	assert.Equal(t, "hello", msgs[0].Text)
	assert.True(t, msgs[0].IsOut)
}

func TestRoot_Send_ConfirmationReplacesSentinel(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	newM, cmd := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})
	m = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	confirmMsg := cmd()
	newM, _ = m.Update(confirmMsg)
	_ = newM

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 42, msgs[0].ID, "sentinel should be replaced with real ID")
}

func TestRoot_Send_FailedSendRemovesSentinel(t *testing.T) {
	mock := &mockTGClient{sendFunc: func() int { return 0 }}
	m, st := newRootWithOpenChat(t, mock)

	newM, cmd := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "hello",
	})
	m = newM.(ui.RootModel)
	require.NotNil(t, cmd)

	confirmMsg := cmd()
	newM, _ = m.Update(confirmMsg)
	_ = newM

	msgs := st.Messages(1)
	assert.Empty(t, msgs, "sentinel should be removed when send fails")
}

func TestRoot_Send_ConcurrentSentinelsHaveDistinctIDs(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	// Send first message
	newM, _ := m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "first",
	})
	m = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	id1 := msgs[0].ID
	assert.Less(t, id1, 0, "first sentinel should have a negative ID")

	// Send second message without running any cmds
	newM, _ = m.Update(screens.SendMsgRequest{
		Peer: store.Peer{ID: 1, Type: store.PeerUser},
		Text: "second",
	})
	_ = newM

	msgs = st.Messages(1)
	require.Len(t, msgs, 2)

	id2 := msgs[1].ID
	assert.Less(t, id2, 0, "second sentinel should have a negative ID")
	assert.NotEqual(t, id1, id2, "two sentinel messages must have distinct IDs")
}
