package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// region identifies a clickable/scrollable area of the main screen.
type region int

const (
	regionNone region = iota
	regionFolders
	regionChatList
	regionMessages
	regionComposer
	regionStatusBar
)

func rectContains(r components.Rect, x, y int) bool {
	return x >= r.Left && x < r.Left+r.Width && y >= r.Top && y < r.Top+r.Height
}

// hitTest maps a screen cell (x, y) to the pane region under it and the
// coordinates local to that region's content rect. Returns regionNone for
// borders, the status bar gaps, or anywhere outside a pane.
func (m RootModel) hitTest(x, y int) (region, int, int) {
	lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(),
		m.folderBar != nil && m.folderBar.HasFolders())
	switch {
	case lay.hasFolders && rectContains(lay.folders, x, y):
		return regionFolders, x - lay.folders.Left, y - lay.folders.Top
	case rectContains(lay.chatList, x, y):
		return regionChatList, x - lay.chatList.Left, y - lay.chatList.Top
	case rectContains(lay.messages, x, y):
		return regionMessages, x - lay.messages.Left, y - lay.messages.Top
	case rectContains(lay.composer, x, y):
		return regionComposer, x - lay.composer.Left, y - lay.composer.Top
	case rectContains(lay.statusBar, x, y):
		return regionStatusBar, x - lay.statusBar.Left, y - lay.statusBar.Top
	}
	return regionNone, 0, 0
}

// mouseBlocked reports whether mouse events should be ignored: on the login
// screen and while any overlay/modal owns input. Overlay-menu clicks are Phase B.
func (m RootModel) mouseBlocked() bool {
	if m.screen == ScreenLogin {
		return true
	}
	return m.searchModel != nil || m.filePicker != nil || m.contextMenu != nil ||
		m.chatMenu != nil || m.reactionPicker != nil || m.photoViewer != nil ||
		m.videoPlayer != nil || m.openPicker != nil || m.mentionPopup != nil
}

// handleMouse dispatches wheel and left-click events to the pane under the cursor.
func (m RootModel) handleMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.mouseBlocked() {
		return m, nil
	}
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(e.Mouse())
	case tea.MouseClickMsg:
		return m.handleMouseClick(e.Mouse())
	}
	return m, nil
}

// defaultWheelStep is how many lines or rows one wheel notch moves when
// ui.mouse_scroll_lines names nothing.
const defaultWheelStep = 3

// wheelApply applies action to a pane step times, returning the moved pane and
// the last non-nil command - the one fetch a scroll may owe.
func wheelApply[P layout.Pane](p P, action keys.Action, step int) (P, tea.Cmd) {
	var cmd tea.Cmd
	for i := 0; i < step; i++ {
		next, c := p.Update(keys.ActionMsg{Action: action})
		p = next.(P)
		if c != nil {
			cmd = c
		}
	}
	return p, cmd
}

// handleMouseWheel scrolls the pane under the cursor by reusing the existing
// up/down key actions, independent of which pane holds keyboard focus.
func (m RootModel) handleMouseWheel(mo tea.Mouse) (tea.Model, tea.Cmd) {
	var action keys.Action
	switch mo.Button {
	case tea.MouseWheelUp:
		action = keys.ActionUp
	case tea.MouseWheelDown:
		action = keys.ActionDown
	default:
		return m, nil
	}
	step := m.wheelStep
	if step < 1 {
		step = defaultWheelStep
	}
	r, _, _ := m.hitTest(mo.X, mo.Y)
	switch r {
	case regionChatList:
		if m.forum != nil {
			// A forum's topic list fills the pane (#275).
			pane, cmd := wheelApply(m.forum, action, step)
			m.forum = pane
			m.syncTopicListWindow()
			return m, cmd
		}
		pane, cmd := wheelApply(m.chatList, action, step)
		m.chatList = pane
		// The wheel moves the cursor as surely as a key does, so it has to ask
		// for the next window just the same.
		m.syncChatListWindow()
		return m, cmd
	case regionMessages:
		pane, cmd := wheelApply(m.chat, action, step)
		m.chat = pane
		return m, tea.Batch(cmd, m.markReadCmd())
	case regionFolders:
		if m.folderBar != nil {
			pane, cmd := wheelApply(m.folderBar, action, step)
			m.folderBar = pane
			return m, cmd
		}
	}
	return m, nil
}

// handleMouseClick handles a left-button press: it moves keyboard focus into the
// clicked pane (focus-follows-click), opens the clicked chat, focuses/blurs the
// composer, and otherwise selects the pane. Non-left buttons are ignored.
func (m RootModel) handleMouseClick(mo tea.Mouse) (tea.Model, tea.Cmd) {
	if mo.Button != tea.MouseLeft {
		return m, nil
	}
	// Toasts float above everything, so a click on a toast action wins over
	// focus-follows-click.
	if msg, ok := m.toasts.HitTest(mo.X, mo.Y); ok {
		return m, func() tea.Msg { return msg }
	}
	r, _, localY := m.hitTest(mo.X, mo.Y)

	// Any click outside the composer blurs it.
	var blurCmd tea.Cmd
	if r != regionComposer && m.chat.ComposerFocused() {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
		m.chat = newPane.(*screens.ChatModel)
		blurCmd = cmd
	}

	switch r {
	case regionChatList:
		res, fcmd := m.focusPane(FocusChatList)
		m = res.(RootModel)
		if m.forum != nil {
			idx, ok := m.forum.TopicIndexAtViewportRow(localY)
			if !ok {
				return m, tea.Batch(blurCmd, fcmd)
			}
			m.forum.SetCursor(idx)
			newPane, ocmd := m.forum.Update(keys.ActionMsg{Action: keys.ActionConfirm})
			m.forum = newPane.(*screens.TopicListModel)
			return m, tea.Batch(blurCmd, fcmd, ocmd)
		}
		idx, ok := m.chatList.ChatIndexAtViewportRow(localY)
		if !ok {
			return m, tea.Batch(blurCmd, fcmd)
		}
		m.chatList.SetCursor(idx)
		newPane, ocmd := m.chatList.Update(keys.ActionMsg{Action: keys.ActionConfirm})
		m.chatList = newPane.(*screens.ChatListModel)
		return m, tea.Batch(blurCmd, fcmd, ocmd)

	case regionMessages:
		res, fcmd := m.focusPane(FocusChat)
		return res, tea.Batch(blurCmd, fcmd)

	case regionComposer:
		res, fcmd := m.focusPane(FocusChat)
		m = res.(RootModel)
		if !m.chat.ComposerFocused() {
			m.vimState.Mode = keys.ModeInsert
			m.statusBar.SetMode(keys.ModeInsert)
			newPane, icmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionInsert})
			m.chat = newPane.(*screens.ChatModel)
			return m, tea.Batch(fcmd, icmd)
		}
		return m, fcmd

	case regionFolders:
		res, fcmd := m.focusPane(FocusFolders)
		return res, tea.Batch(blurCmd, fcmd)
	}
	return m, blurCmd
}
