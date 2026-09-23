package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

// newHelpTestRoot builds a main-screen root focused on the chat list.
func newHelpTestRoot() RootModel {
	m := NewRootModel(50, false)
	m.screen = ScreenMain
	m.width, m.height = 80, 24
	return m
}

func TestRoot_QuestionMarkOpensHelp(t *testing.T) {
	m := newHelpTestRoot()
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	rm := next.(RootModel)
	assert.NotNil(t, rm.help, "'?' opens the help modal")
}

func TestRoot_EscClosesHelp(t *testing.T) {
	m := newHelpTestRoot()
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	rm := next.(RootModel)
	next2, _ := rm.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	rm2 := next2.(RootModel)
	assert.Nil(t, rm2.help, "esc closes the help modal")
}

func TestRoot_QuestionMarkLiteralInInsertMode(t *testing.T) {
	m := newHelpTestRoot()
	m.focus = FocusChat
	m.vimState.Mode = keys.ModeInsert
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	rm := next.(RootModel)
	assert.Nil(t, rm.help, "'?' does not open help while typing")
}
