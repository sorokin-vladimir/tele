package ui_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type mockTGClient struct {
	history []store.Message
}

func (m *mockTGClient) GetDialogs(_ context.Context) ([]store.Chat, error) { return nil, nil }
func (m *mockTGClient) GetHistory(_ context.Context, _ store.Peer, _ int, _ int) ([]store.Message, error) {
	return m.history, nil
}
func (m *mockTGClient) SendMessage(_ context.Context, _ store.Peer, _ string) error { return nil }
func (m *mockTGClient) Updates() <-chan store.Event { return make(chan store.Event) }

var _ internaltg.Client = (*mockTGClient)(nil)

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_CtrlL_FocusesChat(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_CtrlH_FocusesChatList(t *testing.T) {
	m := ui.NewRootModel(nil, nil, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	m = m.WithFocus(ui.FocusChat)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
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
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
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
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	root := newM.(ui.RootModel)
	assert.True(t, root.SearchActive())
}

func TestRoot_CloseSearchMsg_DeactivatesSearch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Alice"})
	m := ui.NewRootModel(nil, st, 50, false)
	m = m.WithScreen(ui.ScreenMain)
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
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
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = newM.(ui.RootModel)
	newM, _ = m.Update(screens.OpenChatMsg{Chat: store.Chat{ID: 1, Title: "Alice"}})
	m = newM.(ui.RootModel)
	assert.False(t, m.SearchActive())
}
