package screens_test

import (
	"fmt"
	"strings"
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

// drainMsgs flattens a (possibly batched) command result into its messages.
func drainMsgs(msg tea.Msg) []tea.Msg {
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c != nil {
			out = append(out, c())
		}
	}
	return out
}

// typeRunes feeds each rune as a key press, returning the final command.
func typeRunes(m *screens.SearchModel, s string) (*screens.SearchModel, tea.Cmd) {
	var cmd tea.Cmd
	for _, r := range s {
		m, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m, cmd
}

func TestSearch_TypingPastMinLengthEmitsDebounce(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	_, cmd := typeRunes(m, "ab")
	require.NotNil(t, cmd, "expected a debounce command after reaching min length")
}

func TestSearch_TypingBelowMinLengthNoDebounce(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	_, cmd := typeRunes(m, "a")
	assert.Nil(t, cmd, "expected no command below min length")
}

func TestSearch_DebounceCurrentSerialRequestsSearch(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, cmd := typeRunes(m, "zz")
	require.NotNil(t, cmd)
	m, cmd = m.Update(cmd()) // feed the debounce tick back in
	assert.True(t, m.GlobalLoading(), "current-serial debounce must set loading")
	require.NotNil(t, cmd)
	var found bool
	for _, mm := range drainMsgs(cmd()) {
		if req, ok := mm.(screens.SearchUsersRequest); ok && req.Query == "zz" {
			found = true
		}
	}
	assert.True(t, found, "expected SearchUsersRequest{Query:\"zz\"} in the batch")
}

func TestSearch_DebounceStaleSerialNoOp(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, cmd := typeRunes(m, "zz")
	require.NotNil(t, cmd)
	staleTick := cmd()       // debounce tick for serial S1
	m, _ = typeRunes(m, "z") // advances serial to S2, query "zzz"
	m, cmd = m.Update(staleTick)
	assert.False(t, m.GlobalLoading(), "stale debounce must not set loading")
	if cmd != nil {
		for _, mm := range drainMsgs(cmd()) {
			if _, ok := mm.(screens.SearchUsersRequest); ok {
				t.Fatal("stale debounce must not emit a SearchUsersRequest")
			}
		}
	}
}

func TestSearch_ResultPopulatesGlobalWithDedup(t *testing.T) {
	chats := []store.Chat{{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}}
	m := screens.NewSearchModel(chats, 80, 24, nil)
	m, cmd := typeRunes(m, "zz")
	require.NotNil(t, cmd)
	m, cmd = m.Update(cmd())
	var req screens.SearchUsersRequest
	for _, mm := range drainMsgs(cmd()) {
		if r, ok := mm.(screens.SearchUsersRequest); ok {
			req = r
		}
	}
	m, _ = m.Update(screens.SearchUsersResult{
		Serial: req.Serial,
		Chats: []store.Chat{
			{ID: 2, Title: "Bob", Peer: store.Peer{ID: 2, Type: store.PeerUser}}, // dup → dropped
			{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser}},
		},
	})
	assert.False(t, m.GlobalLoading(), "loading should clear on result")
	g := m.GlobalResults()
	require.Len(t, g, 1, "want only [99] after dedup, got %+v", g)
	assert.Equal(t, int64(99), g[0].ID)
}

func TestSearch_StaleResultIgnored(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, cmd := typeRunes(m, "zz")
	require.NotNil(t, cmd)
	m, cmd = m.Update(cmd())
	var req screens.SearchUsersRequest
	for _, mm := range drainMsgs(cmd()) {
		if r, ok := mm.(screens.SearchUsersRequest); ok {
			req = r
		}
	}
	m, _ = m.Update(screens.SearchUsersResult{Serial: req.Serial - 1, Chats: []store.Chat{{ID: 99}}})
	assert.Empty(t, m.GlobalResults(), "stale result must be ignored")
}

func TestSearch_ClearingQueryClearsGlobal(t *testing.T) {
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m, cmd := typeRunes(m, "zz")
	require.NotNil(t, cmd)
	m, cmd = m.Update(cmd())
	var req screens.SearchUsersRequest
	for _, mm := range drainMsgs(cmd()) {
		if r, ok := mm.(screens.SearchUsersRequest); ok {
			req = r
		}
	}
	m, _ = m.Update(screens.SearchUsersResult{Serial: req.Serial, Chats: []store.Chat{
		{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser}},
	}})
	require.NotEmpty(t, m.GlobalResults())
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace}) // "z" → below min length
	assert.Empty(t, m.GlobalResults(), "dropping below min length must clear global results")
	assert.False(t, m.GlobalLoading())
}

func TestSearch_ForwardModeNeverSearchesGlobally(t *testing.T) {
	m := screens.NewForwardPicker(makeForwardChats(), 555, 80, 24, nil)
	_, cmd := typeRunes(m, "ab")
	assert.Nil(t, cmd, "forward mode must not emit a debounce command")
}

// loadGlobal drives the model from a query to a populated global-results state.
func loadGlobal(t *testing.T, m *screens.SearchModel, query string, results []store.Chat) *screens.SearchModel {
	t.Helper()
	m, cmd := typeRunes(m, query)
	require.NotNil(t, cmd)
	m, cmd = m.Update(cmd())
	var req screens.SearchUsersRequest
	for _, mm := range drainMsgs(cmd()) {
		if r, ok := mm.(screens.SearchUsersRequest); ok {
			req = r
		}
	}
	m, _ = m.Update(screens.SearchUsersResult{Serial: req.Serial, Chats: results})
	return m
}

func TestSearch_CursorSpansBothSections(t *testing.T) {
	// "zz" matches none of the existing chats → 0 existing, 1 global.
	m := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	m = loadGlobal(t, m, "zz", []store.Chat{
		{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser}},
	})
	// Cursor at 0 must resolve to the global contact: Enter opens it.
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	open, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(99), open.Chat.ID)
}

func TestSearch_EnterOnNewContactOpensChat(t *testing.T) {
	m := screens.NewSearchModel(nil, 80, 24, nil) // no existing chats
	m = loadGlobal(t, m, "zo", []store.Chat{
		{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser, AccessHash: 7}},
	})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	open, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(99), open.Chat.ID)
	assert.Equal(t, int64(7), open.Chat.Peer.AccessHash)
}

func TestSearch_LongListIsWindowed(t *testing.T) {
	var chats []store.Chat
	for i := 0; i < 50; i++ {
		chats = append(chats, store.Chat{ID: int64(i), Title: fmt.Sprintf("Chat%02d", i)})
	}
	m := screens.NewSearchModel(chats, 80, 24, nil)
	view := m.View()
	count := strings.Count(view, "Chat")
	assert.LessOrEqual(t, count, 8, "list must be windowed, got %d rows", count)
	assert.NotContains(t, view, "Chat49", "rows beyond the window must not render")
}

func TestSearch_CtrlNavigationRussianLayout(t *testing.T) {
	var chats []store.Chat
	for i := 0; i < 20; i++ {
		chats = append(chats, store.Chat{ID: int64(i), Title: fmt.Sprintf("Chat%02d", i)})
	}
	m := screens.NewSearchModel(chats, 80, 24, nil)
	assert.Equal(t, 0, m.Cursor())
	// ctrl+о: Cyrillic 'о' sits on the physical J key (ЙЦУКЕН) → move down.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'о', Mod: tea.ModCtrl})
	assert.Equal(t, 1, m.Cursor())
	// ctrl+л: Cyrillic 'л' on the physical K key → move up.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'л', Mod: tea.ModCtrl})
	assert.Equal(t, 0, m.Cursor())
}

func TestSearch_ScrollbarThumbWhenOverflowing(t *testing.T) {
	var chats []store.Chat
	for i := 0; i < 50; i++ {
		chats = append(chats, store.Chat{ID: int64(i), Title: fmt.Sprintf("Chat%02d", i)})
	}
	// "█" also renders as the query-input cursor, so compare counts: the
	// overflowing list adds a scrollbar thumb on top of that single cursor block.
	small := screens.NewSearchModel(makeSearchChats(), 80, 24, nil)
	big := screens.NewSearchModel(chats, 80, 24, nil)
	assert.Greater(t, strings.Count(big.View(), "█"), strings.Count(small.View(), "█"),
		"overflowing list must render extra thumb blocks on the border")
}

func TestSearch_ViewShowsNewContactsHeaderWhenResults(t *testing.T) {
	m := screens.NewSearchModel(nil, 80, 24, nil)
	m = loadGlobal(t, m, "zo", []store.Chat{
		{ID: 99, Title: "Zoe", Peer: store.Peer{ID: 99, Type: store.PeerUser}},
	})
	view := m.View()
	assert.Contains(t, view, "New contacts")
	assert.Contains(t, view, "Zoe")
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
	// status-bar hint style: accented key in place, dim descriptions, " · ".
	// Keys and descriptions sit in separate styled runs, so assert tokens.
	assert.Contains(t, view, "↑/↓")
	assert.Contains(t, view, "move")
	assert.Contains(t, view, "open")
	assert.Contains(t, view, "close")
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
