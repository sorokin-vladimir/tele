package ui_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func TestRoot_InitialScreen_Login(t *testing.T) {
	m := ui.NewRootModel(nil, nil)
	assert.Equal(t, ui.ScreenLogin, m.CurrentScreen())
}

func TestRoot_Tab_SwitchesFocus(t *testing.T) {
	m := ui.NewRootModel(nil, nil)
	m = m.WithScreen(ui.ScreenMain)
	assert.Equal(t, ui.FocusChatList, m.CurrentFocus())
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.FocusChat, root.CurrentFocus())
}

func TestRoot_TransitionToMain(t *testing.T) {
	m := ui.NewRootModel(nil, nil)
	newM, _ := m.Update(screens.TransitionToMainMsg{})
	root := newM.(ui.RootModel)
	assert.Equal(t, ui.ScreenMain, root.CurrentScreen())
}

func TestRoot_CtrlC_Quits(t *testing.T) {
	m := ui.NewRootModel(nil, nil)
	m = m.WithScreen(ui.ScreenMain)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	assert.NotNil(t, cmd)
	msg := cmd()
	_, isQuit := msg.(tea.QuitMsg)
	assert.True(t, isQuit)
}
