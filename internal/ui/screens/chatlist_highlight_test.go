package screens_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
)

func stripANSIChatlist(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && (r == 'm'):
			inEsc = false
		case inEsc:
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestChatList_HighlightChat_SetsState(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats())
	m.HighlightChat(2)
	assert.Equal(t, int64(2), m.HighlightedChatID())
	assert.Equal(t, components.HighlightInitialStep, m.HighlightStep())
}

func TestChatList_StepChatHighlight_CountsDownAndClears(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetChats(makeTestChats())
	m.HighlightChat(2)
	for i := components.HighlightInitialStep; i > 1; i-- {
		assert.True(t, m.StepChatHighlight())
	}
	assert.False(t, m.StepChatHighlight())
	assert.Equal(t, 0, m.HighlightStep())
	assert.Equal(t, int64(0), m.HighlightedChatID())
}

func TestChatList_Highlight_SurvivesReorderByID(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats(makeTestChats()) // Alice(1), Bob(2), Charlie(3)
	m.HighlightChat(2)
	// Reorder: Bob bubbles to top. Highlight tracks chat 2 by id.
	m.SetChats([]store.Chat{
		{ID: 2, Title: "Bob"},
		{ID: 1, Title: "Alice"},
		{ID: 3, Title: "Charlie"},
	})
	assert.Equal(t, int64(2), m.HighlightedChatID())
	assert.Equal(t, components.HighlightInitialStep, m.HighlightStep())
}

func TestChatList_View_HighlightChangesRowStyling(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats(makeTestChats())
	plain := m.View()

	m.HighlightChat(2)
	highlighted := m.View()

	// Styling differs (accent applied)...
	assert.NotEqual(t, plain, highlighted, "highlight should change the rendered ANSI")
	// ...but the visible text is unchanged.
	assert.Equal(t, stripANSIChatlist(plain), stripANSIChatlist(highlighted))
}

func TestChatList_View_NoHighlightOnFocusedCursorRow(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	m.SetChats(makeTestChats())
	m.SetFocused(true) // cursor at row 0 (Alice, id 1) by default
	plain := m.View()

	m.HighlightChat(1) // same row the cursor is on
	got := m.View()

	// Selection background wins on the focused-cursor row: no accent change.
	assert.Equal(t, plain, got)
}
