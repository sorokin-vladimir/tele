package ui_test

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gotd/td/tgerr"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTGClient struct {
	history           []store.Message
	historyErr        error
	sendFunc          func() int
	lastReplyToMsgID  int
	downloadPhotoFunc func() (image.Image, error)
	refreshFunc       func(msgID int) (store.Message, error)
}

func (m *mockTGClient) GetDialogs(_ context.Context) ([]store.Chat, error) { return nil, nil }
func (m *mockTGClient) GetDialogFilters(_ context.Context) ([]store.FolderFilter, error) {
	return nil, nil
}
func (m *mockTGClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	return m.history, nil
}
func (m *mockTGClient) RefreshMessage(_ context.Context, _ store.Peer, msgID int) (store.Message, error) {
	if m.refreshFunc != nil {
		return m.refreshFunc(msgID)
	}
	return store.Message{}, nil
}
func (m *mockTGClient) SendMessage(_ context.Context, _ store.Peer, _ string, replyToMsgID int) (int, error) {
	m.lastReplyToMsgID = replyToMsgID
	if m.sendFunc != nil {
		return m.sendFunc(), nil
	}
	return 42, nil
}
func (m *mockTGClient) MarkRead(_ context.Context, _ store.Peer, _ int) error { return nil }
func (m *mockTGClient) DownloadPhoto(_ context.Context, _ store.PhotoRef) (image.Image, error) {
	if m.downloadPhotoFunc != nil {
		return m.downloadPhotoFunc()
	}
	return nil, nil
}
func (m *mockTGClient) DownloadDocument(_ context.Context, _ store.DocumentRef) ([]byte, error) {
	return nil, nil
}
func (m *mockTGClient) DownloadDocumentThumb(_ context.Context, _ store.DocumentRef) (image.Image, error) {
	return nil, nil
}
func (m *mockTGClient) EditMessage(_ context.Context, _ store.Peer, _ int, _ string) error {
	return nil
}
func (m *mockTGClient) DeleteMessages(_ context.Context, _ store.Peer, _ []int, _ bool) error {
	return nil
}
func (m *mockTGClient) SendReaction(_ context.Context, _ store.Peer, _ int, _ string) error {
	return nil
}

func (m *mockTGClient) SetTyping(_ context.Context, _ store.Peer, _ store.TypingAction) error {
	return nil
}
func (m *mockTGClient) Updates() <-chan store.Event { return make(chan store.Event) }

var _ internaltg.Client = (*mockTGClient)(nil)

func TestRoot_EventNewMessage_FiresPhotoDownload(t *testing.T) {
	mc := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mc) // chat ID 1 is the active chat

	newMsg := store.Message{ID: 101, ChatID: 1, Photo: &store.PhotoRef{ID: 9}}
	_, cmd := m.Update(store.Event{Kind: store.EventNewMessage, Message: newMsg})
	require.NotNil(t, cmd) // download command batched
}

func TestRoot_HistoryChunk_FiresPhotoDownload(t *testing.T) {
	mc := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mc) // chat ID 1 is the active chat

	older := []store.Message{{ID: 150, ChatID: 1, Photo: &store.PhotoRef{ID: 5}}}
	_, cmd := m.Update(ui.HistoryChunkMsgForTest(1, older))
	require.NotNil(t, cmd)
}

func TestRoot_ChatOpenFailure_ClearsSpinnerAndShowsError(t *testing.T) {
	mc := &mockTGClient{historyErr: errors.New("timeout")}
	st := store.NewMemory()
	chat := store.Chat{ID: 7, Title: "Bob", Peer: store.Peer{ID: 7, Type: store.PeerUser}}
	st.SetChat(chat)
	m := ui.NewRootModel(mc, st, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)

	newM, cmd := m.Update(screens.OpenChatMsg{Chat: chat})
	require.NotNil(t, cmd)
	// drain the batched open cmd; one branch yields the chat load error
	m = newM.(ui.RootModel)
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm, _ := m.Update(inner)
		m = nm.(ui.RootModel)
	}
	assert.Contains(t, m.View().Content, "timeout")
}

func TestDownloadPhotoCmd_RefreshesOnExpiredRef(t *testing.T) {
	calls := 0
	mc := &mockTGClient{
		downloadPhotoFunc: func() (image.Image, error) {
			calls++
			if calls == 1 {
				return nil, &tgerr.Error{Code: 400, Type: "FILE_REFERENCE_EXPIRED"}
			}
			return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
		},
		refreshFunc: func(msgID int) (store.Message, error) {
			return store.Message{ID: msgID, ChatID: 7, Photo: &store.PhotoRef{ID: 1, FileReference: []byte("fresh")}}, nil
		},
	}
	cmd := ui.DownloadPhotoCmdForTest(mc, store.Peer{ID: 7, Type: store.PeerUser}, 100, store.PhotoRef{ID: 1})

	msgs := drainMsgs(cmd())
	assert.Equal(t, 2, calls) // retried once after refresh
	var ready *ui.PhotoReadyMsg
	for _, m := range msgs {
		if r, ok := m.(ui.PhotoReadyMsg); ok {
			rr := r
			ready = &rr
		}
	}
	require.NotNil(t, ready)
	assert.NotNil(t, ready.Image)
	assert.Len(t, msgs, 2) // ready image + store-update after refresh
}

// drainMsgs flattens a (possibly batched) cmd result into its concrete messages.
func drainMsgs(msg tea.Msg) []tea.Msg {
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

func TestRoot_StatusErrMsg_SetsAndSchedulesClear(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	newM, cmd := newM.(ui.RootModel).Update(ui.StatusErrMsg{Text: "network down", Sev: components.SeverityError})
	root := newM.(ui.RootModel)
	assert.Contains(t, root.View().Content, "network down")
	require.NotNil(t, cmd) // an auto-clear tick was scheduled
}

func TestRoot_ClearStatusErrMsg_StaleSerialKeepsError(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false).WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m2, _ := newM.(ui.RootModel).Update(ui.StatusErrMsg{Text: "first", Sev: components.SeverityError})
	root := m2.(ui.RootModel)
	m3, _ := root.Update(ui.ClearStatusErrMsg{Serial: -999}) // never a real serial
	assert.Contains(t, m3.(ui.RootModel).View().Content, "first")
}

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_InitialChatList_IsFocused(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.True(t, m.ChatList().Focused(), "chatList must be focused from the start so cursor highlight is visible")
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
		Message: store.Message{ID: 1, ChatID: 2, Text: "hi"},
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
		Message: store.Message{ID: 1, ChatID: 2, Text: "first"},
	}
	newM, _ := m.Update(evt)
	m = newM.(ui.RootModel)

	evt2 := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 2, ChatID: 2, Text: "second"},
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

func TestRoot_NewMessageEvent_NoUnreadForOutgoingMessage(t *testing.T) {
	m, _ := newRootWithTwoChats(t)

	evt := store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ChatID: 2, Text: "sent from phone", IsOut: true},
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
	assert.Equal(t, 0, chat2.UnreadCount)
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

func TestRootModel_PhotoDownloadDispatchedOnHistory(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	m2, cmd := m.Update(ui.ChatHistoryMsg{
		ChatID: 1,
		Messages: []store.Message{
			{ID: 10, ChatID: 1, Text: "hello"},
			{ID: 11, ChatID: 1, Photo: &store.PhotoRef{ID: 77, ThumbSize: "m"}},
		},
	})
	_ = m2
	require.NotNil(t, cmd, "should return cmd (download + markread) for messages with photo")
}

func TestRootModel_PhotoReadyMsg_StoresImage(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	m2, _ := m.Update(ui.PhotoReadyMsg{PhotoID: 55, Image: img})
	_ = m2
	// No panic — image cache updated without crashing
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

func TestRoot_Space_OpensContextMenu(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.True(t, m.ContextMenuOpen())
}

func TestRoot_Space_NoMenuWhenNoMessages(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)
	// No messages added — SelectedMessageID() returns 0

	newM, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen(), "menu should not open when no message is selected")
}

func TestRoot_ContextMenu_EscCloses(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open menu
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	// close with esc
	newM, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)

	// dispatch the CloseContextMenuMsg cmd if present
	require.NotNil(t, cmd, "esc should return a CloseContextMenuMsg cmd")
	newM, _ = m.Update(cmd())
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen())
}

func TestWithKeyMap_RebindOpensContextMenu(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"open_context_menu": {"m"}},
	})
	require.Empty(t, warns)
	m = m.WithKeyMap(km)

	// "m" now opens the context menu (was "space").
	newM, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = newM.(ui.RootModel)
	assert.True(t, m.ContextMenuOpen())
}

func TestRoot_DeleteMsgRequest_RemovesFromStore(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	require.Len(t, st.Messages(1), 1)

	newM, _ = m.Update(components.DeleteMsgRequest{MsgID: 10, Revoke: false})
	_ = newM

	assert.Empty(t, st.Messages(1), "message removed from store")
}

func TestRoot_ContextMenu_QuitKeyDoesNotQuit(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "hello", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open context menu
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	// q while menu is open must not close the app
	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = newM.(ui.RootModel)

	assert.True(t, m.ContextMenuOpen(), "context menu must stay open after q")
	assert.Nil(t, cmd, "q while menu is open must not produce a quit cmd")
}

func TestRoot_ReplyMsgRequest_ClosesMenuAndFocusesComposer(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// open context menu first
	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	newM, _ = m.Update(components.ReplyMsgRequest{MsgID: 10})
	m = newM.(ui.RootModel)

	assert.False(t, m.ContextMenuOpen(), "context menu must close after ReplyMsgRequest")
	assert.True(t, m.Chat().ComposerFocused(), "composer must be focused after ReplyMsgRequest")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "ReplyMsgRequest must switch root to insert mode")
}

func TestRoot_Send_WithReply_PassesReplyToMsgID(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	_, cmd := m.Update(screens.SendMsgRequest{
		Peer:         store.Peer{ID: 1, Type: store.PeerUser},
		Text:         "my reply",
		ReplyToMsgID: 10,
	})
	require.NotNil(t, cmd)
	cmd() // triggers mock.SendMessage

	assert.Equal(t, 10, mock.lastReplyToMsgID)
}

func TestRoot_R_Key_ActivatesReplyMode(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = newM.(ui.RootModel)

	assert.True(t, m.Chat().ComposerFocused(), "r key must activate reply mode and focus composer")
	assert.Equal(t, 10, m.Chat().ReplyToMsgID(), "r key must set reply target")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "r key must switch root to insert mode")
}

func TestRoot_OpenChat_ClearsPendingReply(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "original", SenderName: "Alice", Date: time.Now()})
	newM, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	// activate reply mode
	newM, _ = m.Update(components.ReplyMsgRequest{MsgID: 10})
	m = newM.(ui.RootModel)
	require.Equal(t, 10, m.Chat().ReplyToMsgID(), "reply must be active before switching chat")

	// switch to a different chat
	st.SetChat(store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}})
	newM, _ = m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}})
	m = newM.(ui.RootModel)

	assert.Equal(t, 0, m.Chat().ReplyToMsgID(), "switching chat must clear pending reply")
}

func TestRoot_Send_SentinelCarriesReplyToMsgID(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)

	_, _ = m.Update(screens.SendMsgRequest{
		Peer:         store.Peer{ID: 1, Type: store.PeerUser},
		Text:         "my reply",
		ReplyToMsgID: 10,
	})

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 10, msgs[0].ReplyToMsgID, "sentinel must carry ReplyToMsgID")
}

func TestRoot_h_CyclesFocusLeft(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChatList, root.CurrentFocus())
}

func TestRoot_l_CyclesFocusRight(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_FolderSelectedMsg_FiltersChatList(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, IsContact: true})
	st.SetChat(store.Chat{ID: 2, Title: "Group", Peer: store.Peer{ID: 2, Type: store.PeerGroup}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	filter := store.FolderFilter{ID: 1, Title: "Contacts", Contacts: true}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: []store.FolderFilter{filter}})
	m = newM.(ui.RootModel)

	// Select the Contacts folder
	selectedFilter := filter
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &selectedFilter})
	root := newM.(ui.RootModel)

	// Only the contact chat should be in the chatlist
	chats := root.ChatList().Chats()
	require.Len(t, chats, 1)
	assert.Equal(t, int64(1), chats[0].ID)
}

func TestRoot_FolderFiltersMsg_SetsFolders(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := newM.(ui.RootModel)
	assert.True(t, root.HasFolders())
}

func TestRoot_0_FocusesFolders(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	m2, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	root := m2.(ui.RootModel)
	newM, _ := root.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	root2 := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusFolders, root2.CurrentFocus())
}

func TestRoot_FocusNext_DoesNotAutoOpenChat(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	st.SetChat(store.Chat{ID: 2, Title: "Bob"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChatList, m.CurrentFocus())

	newM, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = newM.(ui.RootModel)
	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChat, m.CurrentFocus())
	assert.Nil(t, cmd, "switching focus must not open a chat")
}

func TestRoot_FolderSelectedMsg_FocusesChatList(t *testing.T) {
	st := store.NewMemory()
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	filters := []store.FolderFilter{{ID: 1, Title: "Work"}}
	newM, _ := m.Update(ui.FolderFiltersMsg{Filters: filters})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: '0', Text: "0"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusFolders, m.CurrentFocus())

	filter := store.FolderFilter{ID: 1, Title: "Work"}
	newM, _ = m.Update(screens.FolderSelectedMsg{Filter: &filter})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_OpenSameChatAgain_OnlyFocusesChatPane(t *testing.T) {
	mock := &mockTGClient{}
	m, _ := newRootWithOpenChat(t, mock)

	newM, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChatList, m.CurrentFocus())

	newM, cmd := m.Update(screens.OpenChatMsg{Chat: store.Chat{
		ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser},
	}})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChat, m.CurrentFocus())
	assert.Nil(t, cmd, "re-opening same chat must not trigger a history reload")
}

func TestRoot_EventDeleteMessages_Channel_RemovesFromCurrentChat(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	now := time.Now()
	st.SetMessages(1, []store.Message{
		{ID: 10, ChatID: 1, Text: "hello", Date: now},
		{ID: 11, ChatID: 1, Text: "world", Date: now},
	})
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 1,
		MsgIDs: []int{10},
	}
	newM, _ = m.Update(evt)
	_ = newM.(ui.RootModel)

	msgs := st.Messages(1)
	require.Len(t, msgs, 1)
	assert.Equal(t, 11, msgs[0].ID)
}

func TestRoot_EventDeleteMessages_NonChannel_ScansAllChats(t *testing.T) {
	m, st := newRootWithTwoChats(t)
	now := time.Now()
	st.SetMessages(1, []store.Message{{ID: 5, ChatID: 1, Text: "a", Date: now}})
	st.SetMessages(2, []store.Message{{ID: 5, ChatID: 2, Text: "b", Date: now}})

	evt := store.Event{
		Kind:   store.EventDeleteMessages,
		ChatID: 0,
		MsgIDs: []int{5},
	}
	newM, _ := m.Update(evt)
	_ = newM.(ui.RootModel)

	assert.Empty(t, st.Messages(1))
	assert.Empty(t, st.Messages(2))
}

func TestRoot_ContextMenu_PhotoMessage_ShowsOpenInViewer(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(store.Message{
		ID:     10,
		ChatID: 1,
		Text:   "photo msg",
		Photo:  &store.PhotoRef{ID: 77},
		Date:   time.Now(),
	})
	newM, _ = m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())
	assert.Contains(t, m.View().Content, "Open in viewer")
}

func TestRoot_ContextMenu_NonPhotoMessage_HidesOpenInViewer(t *testing.T) {
	mock := &mockTGClient{}
	m, st := newRootWithOpenChat(t, mock)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = newM.(ui.RootModel)
	st.AppendMessage(store.Message{ID: 10, ChatID: 1, Text: "text msg", Date: time.Now()})
	newM, _ = m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = newM.(ui.RootModel)

	newM, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = newM.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())
	assert.NotContains(t, m.View().Content, "Open in viewer")
}

func TestRoot_EventUserPresence_UpdatesChatOnline(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m.ChatList().SetChats(st.Chats())

	newM, _ := m.Update(store.Event{
		Kind:   store.EventUserPresence,
		ChatID: 1,
		Online: true,
	})
	_ = newM.(ui.RootModel)

	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.Online)
}

func TestRoot_PasteMsg_WhenComposerFocused_InsertsText(t *testing.T) {
	m, _ := newRootWithOpenChat(t, &mockTGClient{})
	// enter insert mode → focuses composer
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = newM.(ui.RootModel)
	require.True(t, m.Chat().ComposerFocused())

	newM, _ = m.Update(tea.PasteMsg{Content: "pasted text"})
	m = newM.(ui.RootModel)

	assert.Equal(t, "pasted text", m.Chat().ComposerValue())
}

func TestRoot_PasteMsg_WhenSearchOpen_UpdatesQuery(t *testing.T) {
	m, _ := newRootWithOpenChat(t, &mockTGClient{})
	// open search
	newM, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = newM.(ui.RootModel)
	require.True(t, m.SearchActive())

	newM, _ = m.Update(tea.PasteMsg{Content: "alice"})
	m = newM.(ui.RootModel)

	require.True(t, m.SearchActive())
	assert.Equal(t, "alice", m.Search().Query())
}

func TestRoot_Esc_NormalMode_ClosesChatReturnsToChatList(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Open a chat — this sets focus to FocusChat.
	newM, _ := m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)
	require.Equal(t, ui.FocusChat, m.CurrentFocus())

	// Press Esc in normal mode.
	newM, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = newM.(ui.RootModel)

	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
}

func TestRoot_SetTmpDir(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m.SetTmpDir("/tmp/tele-test")
	assert.Equal(t, "/tmp/tele-test", m.TmpDir())
}

// TestRoot_NewMessageEvent_AlreadyReadElsewhere reproduces issue #88.
// gotd dispatches OtherUpdates (including updateReadHistoryInbox) before NewMessages
// in a getDifference response. A message whose ID is covered by the read pointer
// must not produce a false unread badge when it arrives after the read event.
func TestRoot_NewMessageEvent_AlreadyReadElsewhere(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{
		ID:             2,
		Title:          "Bob",
		ReadInboxMaxID: 100,
		UnreadCount:    0,
	})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	m = newM.(ui.RootModel)

	// EventReadInbox arrives first (gotd OtherUpdates before NewMessages)
	newM, _ = m.Update(store.Event{
		Kind:      store.EventReadInbox,
		ChatID:    2,
		ReadMaxID: 100,
	})
	m = newM.(ui.RootModel)

	// EventNewMessage for a message already covered by the read pointer
	newM, _ = m.Update(store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 99, ChatID: 2, Text: "read elsewhere"},
	})
	root := newM.(ui.RootModel)

	var chat2 store.Chat
	for _, c := range root.ChatList().Chats() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.UnreadCount, "message read elsewhere must not increment unread badge")
}

// TestRoot_StartupCatchup_ServerReadClearsStaleBadge reproduces issue #88.
// At startup the updates manager begins replaying getDifference catch-up events
// BEFORE GetDialogs finishes. A new-message event arrives while the store still
// holds the previous session's read pointer, so the badge increments. The read
// acknowledgement for that chat is dropped during getDifference. GetDialogs then
// writes the authoritative server state (read elsewhere → UnreadCount 0), which
// must win — the list badge must not stay stuck on the stale increment.
func TestRoot_StartupCatchup_ServerReadClearsStaleBadge(t *testing.T) {
	st := store.NewMemory()
	// Persisted from previous session: read up to 100, no unread.
	st.SetChat(store.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 100, UnreadCount: 0})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)

	// Catch-up: new message (already read on another client) arrives before
	// GetDialogs completes and before the read ack (which is dropped).
	newM, _ := m.Update(store.Event{
		Kind:    store.EventNewMessage,
		Message: store.Message{ID: 150, ChatID: 2, Text: "read elsewhere"},
	})
	m = newM.(ui.RootModel)

	// GetDialogs completes: server reports the chat as already read.
	st.SetChat(store.Chat{ID: 2, Title: "Bob", ReadInboxMaxID: 150, UnreadCount: 0})

	// Transition rebuilds the chat list from the authoritative store.
	newM, _ = m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)

	var chat2 store.Chat
	for _, c := range root.ChatList().Chats() {
		if c.ID == 2 {
			chat2 = c
		}
	}
	assert.Equal(t, 0, chat2.UnreadCount, "authoritative server read state must clear stale unread badge")
}
