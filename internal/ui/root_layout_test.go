package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestComputeLayout_TwoPane(t *testing.T) {
	// width=100, height=30, composerHeight=3, no folders.
	// SplitHorizontal(100, .30) -> left=30, right=70. contentH=height-3=27.
	lay := computeLayout(100, 30, 3, false)

	assert.False(t, lay.hasFolders)
	// Chat list content sits inside the left box (border inset 1 on each side).
	assert.Equal(t, 1, lay.chatList.Top)
	assert.Equal(t, 1, lay.chatList.Left)
	assert.Equal(t, 28, lay.chatList.Width) // leftW-2
	assert.Equal(t, 27, lay.chatList.Height)
	// Messages occupy the top of the right box, above the composer.
	assert.Equal(t, 1, lay.messages.Top)
	assert.Equal(t, 31, lay.messages.Left)   // leftW+1
	assert.Equal(t, 68, lay.messages.Width)  // rightW-2
	assert.Equal(t, 24, lay.messages.Height) // contentH-composerHeight
	// Composer is the bottom composerHeight rows of the right box.
	assert.Equal(t, 25, lay.composer.Top) // 1+messages.Height
	assert.Equal(t, 31, lay.composer.Left)
	assert.Equal(t, 68, lay.composer.Width)
	assert.Equal(t, 3, lay.composer.Height)
	// Status bar is the final row, full width.
	assert.Equal(t, 29, lay.statusBar.Top) // height-1
	assert.Equal(t, 0, lay.statusBar.Left)
	assert.Equal(t, 100, lay.statusBar.Width)
	assert.Equal(t, 1, lay.statusBar.Height)
}

func TestComputeLayout_ThreePaneWithFolders(t *testing.T) {
	// width=120, height=40, composerHeight=3, folders on.
	// SplitThree(120, 18, .30): sidebar=18, remaining=102, mid=int(102*.3)=30, right=72.
	lay := computeLayout(120, 40, 3, true)
	assert.True(t, lay.hasFolders)
	assert.Equal(t, 1, lay.folders.Left)     // 0+1
	assert.Equal(t, 16, lay.folders.Width)   // sidebarW-2
	assert.Equal(t, 19, lay.chatList.Left)   // 18+1
	assert.Equal(t, 28, lay.chatList.Width)  // chatlistW(30)-2
	assert.Equal(t, 49, lay.messages.Left)   // 18+30+1
	assert.Equal(t, 70, lay.messages.Width)  // chatW(72)-2
	assert.Equal(t, 37, lay.chatList.Height) // contentH=height-3
}

func TestWindowSize_SetsPaneSizesFromLayout(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := NewRootModel(nil, st, 50, false).WithScreen(ScreenMain)
	m.chatList.SetChats(st.Chats())

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	rm := next.(RootModel)

	lay := computeLayout(100, 30, rm.chat.ComposerHeight(), false)
	// The chat list's content height must equal the layout it will be hit-tested against.
	assert.Equal(t, lay.chatList.Height, rm.chatList.Height())
}
