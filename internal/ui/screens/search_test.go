package screens_test

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSearchChats() []store.Chat {
	return []store.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
		{ID: 3, Title: "Alexander"},
	}
}

func TestSearch_InitiallyShowsAllChats(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Bob")
	assert.Contains(t, view, "Alexander")
}

func TestSearch_FiltersByQuery(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Alexander")
	assert.NotContains(t, view, "Bob")
}

func TestSearch_CursorNavigation(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	assert.Equal(t, 0, m.Cursor())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	assert.Equal(t, 1, m.Cursor())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_CursorClamped(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	assert.Equal(t, 0, m.Cursor())
}

func makeForwardChats() []store.Chat {
	return []store.Chat{
		{ID: 1, Title: "Alice", Peer: store.Peer{ID: 1, Type: store.PeerUser}, UnreadCount: 3},
		{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}},
	}
}

func TestForwardPicker_EnterEmitsForwardToChatRequest(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.ForwardToChatRequest)
	require.True(t, ok)
	assert.Equal(t, 55, req.MsgID)
	assert.Equal(t, int64(1), req.ToPeer.ID)
}

func TestForwardPicker_RendersUnreadCount(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, nil)
	assert.Contains(t, m.View(), "3")
}

func TestForwardPicker_FiltersLikeSearch(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'b', Text: "b"})
	view := m.View()
	assert.Contains(t, view, "Bob")
	assert.NotContains(t, view, "Alice")
}

func TestSearch_CursorBelowWindow_StaysVisible(t *testing.T) {
	chats := make([]store.Chat, 20)
	for i := range chats {
		chats[i] = store.Chat{ID: int64(i + 1), Title: fmt.Sprintf("Chat%02d", i)}
	}
	m := screens.NewSearchModel(chats, 80, 24, nil)
	for i := 0; i < 12; i++ { // move well past the 8-row window
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view := m.View()
	assert.Equal(t, 12, m.Cursor())
	assert.Contains(t, view, "Chat12", "selected row must stay within the rendered window")
}

func TestForwardPicker_Tab_EntersCommentPhase_EnterSendsComment(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // enter comment phase for Alice
	// type a comment, including a Cyrillic rune
	m, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m, _ = m.Update(tea.KeyPressMsg{Text: "п"})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.ForwardToChatRequest)
	require.True(t, ok)
	assert.Equal(t, 55, req.MsgID)
	assert.Equal(t, int64(1), req.ToPeer.ID) // Alice
	assert.Equal(t, "hiп", req.Comment)
}

func TestForwardPicker_CommentBackspaceAndEscBack(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	// esc returns to select; a subsequent Enter forwards with empty comment
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.ForwardToChatRequest)
	require.True(t, ok)
	assert.Equal(t, "", req.Comment)
}

func TestSearch_Tab_IgnoredInSearchMode(t *testing.T) {
	m := screens.NewSearchModel(makeForwardChats(), 80, 24, nil)
	// Tab in plain search mode must not switch phases; Enter still opens a chat.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	_, ok := cmd().(screens.OpenChatMsg)
	assert.True(t, ok)
}

func TestForwardPicker_HintShowsTabComment(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 55, 80, 24, keys.DefaultKeyMap())
	assert.Contains(t, m.View(), "comment")
}

func TestSearch_EnterEmitsOpenChatMsg(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	oc, ok := msg.(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(1), oc.Chat.ID)
}

func TestSearch_EscEmitsCloseSearchMsg(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.IsType(t, screens.CloseSearchMsg{}, msg)
}

func TestSearch_BackspaceDeletesQuery(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	view := m.View()
	assert.Contains(t, view, "Alice")
}

func TestSearch_CursorResetOnFilter(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_SpaceInQuery(t *testing.T) {
	m := screens.NewSearchModel([]store.Chat{
		{ID: 1, Title: "John Doe"},
		{ID: 2, Title: "Alice"},
	}, 80, 24, nil)
	// Type "John" then space — "john " is a substring of "john doe"
	for _, r := range "John" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	view := m.View()
	assert.Contains(t, view, "John Doe")
	assert.NotContains(t, view, "Alice")
}

func TestSearch_HintInBottomBorder(t *testing.T) {
	km := keys.DefaultKeyMap()
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, km)
	view := m.View()
	assert.Contains(t, view, "esc -> close")
	assert.Contains(t, view, "enter -> open")
	assert.Contains(t, view, "↑/↓ -> move")
}

func TestSearch_PasteMsg_UpdatesQuery(t *testing.T) {
	km := keys.DefaultKeyMap()
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, km)

	newM, _ := m.Update(tea.PasteMsg{Content: "Ali"})
	m = newM

	assert.Equal(t, "Ali", m.Query())
	assert.Len(t, m.Results(), 1) // only "Alice" contains "ali"
}

func TestSearch_NoHintWithoutKeyMap(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	view := m.View()
	assert.NotContains(t, view, "->")
}
