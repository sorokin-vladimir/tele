package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func newSizedRoot(t *testing.T, w, h int) RootModel {
	t.Helper()
	st := store.NewMemory()
	for i := int64(1); i <= 5; i++ {
		st.SetChat(store.Chat{ID: i, Peer: store.Peer{ID: i, Type: store.PeerUser}, Title: "chat"})
	}
	m := NewRootModel(nil, st, 50, false).WithScreen(ScreenMain)
	m.chatList.SetChats(st.Chats())
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(RootModel)
}

func TestHitTest_Regions(t *testing.T) {
	m := newSizedRoot(t, 100, 30)

	// Chat list content starts at col 1, row 1.
	r, lx, ly := m.hitTest(3, 2)
	assert.Equal(t, regionChatList, r)
	assert.Equal(t, 2, lx)
	assert.Equal(t, 1, ly)

	// Messages region: right box top area.
	r, _, _ = m.hitTest(40, 2)
	assert.Equal(t, regionMessages, r)

	// Composer region: bottom rows of the right box (composerHeight=3 -> Top=25).
	r, _, _ = m.hitTest(40, 26)
	assert.Equal(t, regionComposer, r)

	// Status bar: last row.
	r, _, _ = m.hitTest(10, 29)
	assert.Equal(t, regionStatusBar, r)

	// Off-screen / border cell: none.
	r, _, _ = m.hitTest(0, 0)
	assert.Equal(t, regionNone, r)
}

func TestWheel_OverMessages_ScrollsChatPane(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	// A wheel event over the messages region must not error and must be routed
	// (regression guard: the terminal no longer synthesizes arrow keys).
	next, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{X: 40, Y: 2, Button: tea.MouseWheelDown}))
	_, ok := next.(RootModel)
	assert.True(t, ok)
}

func TestWheel_OverChatList_MovesCursorDown(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChatList
	before := m.chatList.Cursor()
	next, _ := m.Update(tea.MouseWheelMsg(tea.Mouse{X: 3, Y: 2, Button: tea.MouseWheelDown}))
	rm := next.(RootModel)
	assert.Equal(t, before+1, rm.chatList.Cursor())
}

func TestClick_ChatList_OpensChatAndFocuses(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	m.focus = FocusChat // start focused elsewhere

	// Click the second visible chat row (content row 1 -> screen y = 2).
	next, cmd := m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 2, Button: tea.MouseLeft}))
	rm := next.(RootModel)

	assert.Equal(t, FocusChatList, rm.focus) // focus-follows-click
	assert.Equal(t, 1, rm.chatList.Cursor()) // cursor moved to clicked row
	require.NotNil(t, cmd)                   // OpenChatMsg command emitted
	msg := cmd()
	_, ok := msg.(screens.OpenChatMsg)
	assert.True(t, ok, "expected OpenChatMsg, got %T", msg)
}

func TestClick_Composer_FocusesComposer(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 40, Y: 26, Button: tea.MouseLeft}))
	rm := next.(RootModel)
	assert.Equal(t, FocusChat, rm.focus)
	assert.True(t, rm.chat.ComposerFocused())
	assert.Equal(t, keys.ModeInsert, rm.vimState.Mode)
}

func TestClick_OutsideComposer_Blurs(t *testing.T) {
	m := newSizedRoot(t, 100, 30)
	// Focus the composer first.
	next, _ := m.Update(tea.MouseClickMsg(tea.Mouse{X: 40, Y: 26, Button: tea.MouseLeft}))
	m = next.(RootModel)
	require.True(t, m.chat.ComposerFocused())

	// Click the chat list -> composer blurs.
	next, _ = m.Update(tea.MouseClickMsg(tea.Mouse{X: 3, Y: 2, Button: tea.MouseLeft}))
	rm := next.(RootModel)
	assert.False(t, rm.chat.ComposerFocused())
	assert.Equal(t, keys.ModeNormal, rm.vimState.Mode)
}
