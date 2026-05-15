package screens_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func makeSearchChats() []store.Chat {
	return []store.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
		{ID: 3, Title: "Alexander"},
	}
}

func TestSearch_InitiallyShowsAllChats(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Bob")
	assert.Contains(t, view, "Alexander")
}

func TestSearch_FiltersByQuery(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Alexander")
	assert.NotContains(t, view, "Bob")
}

func TestSearch_CursorNavigation(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	assert.Equal(t, 0, m.Cursor())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, m.Cursor())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_CursorClamped(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_EnterEmitsOpenChatMsg(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	oc, ok := msg.(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(1), oc.Chat.ID)
}

func TestSearch_EscEmitsCloseSearchMsg(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.IsType(t, screens.CloseSearchMsg{}, msg)
}

func TestSearch_BackspaceDeletesQuery(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	view := m.View()
	assert.Contains(t, view, "Alice")
}

func TestSearch_CursorResetOnFilter(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_SpaceInQuery(t *testing.T) {
	m := screens.NewSearchModel([]store.Chat{
		{ID: 1, Title: "John Doe"},
		{ID: 2, Title: "Alice"},
	}, 80, 24)
	// Type "John" then space — "john " is a substring of "john doe"
	for _, r := range "John" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	view := m.View()
	assert.Contains(t, view, "John Doe")
	assert.NotContains(t, view, "Alice")
}
