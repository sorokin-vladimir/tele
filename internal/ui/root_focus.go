package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// focusedContext maps the focused pane to its key-binding context.
func (m RootModel) focusedContext() keys.Context {
	switch m.focus {
	case FocusFolders:
		return keys.ContextFolders
	case FocusChatList:
		return keys.ContextChatList
	default:
		return keys.ContextChat
	}
}

func (m RootModel) focusPrev() (tea.Model, tea.Cmd) {
	hasFolders := m.folderBar != nil && m.folderBar.HasFolders()
	switch m.focus {
	case FocusChat:
		return m.focusPane(FocusChatList)
	case FocusChatList:
		if hasFolders {
			return m.focusPane(FocusFolders)
		}
		return m, nil
	case FocusFolders:
		return m.focusPane(FocusChat)
	}
	return m, nil
}

func (m RootModel) focusNext() (tea.Model, tea.Cmd) {
	hasFolders := m.folderBar != nil && m.folderBar.HasFolders()
	switch m.focus {
	case FocusFolders:
		return m.focusPane(FocusChatList)
	case FocusChatList:
		return m.focusPane(FocusChat)
	case FocusChat:
		if hasFolders {
			return m.focusPane(FocusFolders)
		}
		return m, nil
	}
	return m, nil
}

func (m RootModel) focusPane(target Focus) (tea.Model, tea.Cmd) {
	if target == m.focus {
		return m, nil
	}
	m.matcher.Reset()
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeVisual {
		m.endVisual()
	}
	// Exit insert mode when leaving chat
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeInsert {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
		newPane, _ := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
		m.chat = newPane.(*screens.ChatModel)
	}
	m.focus = target
	m.chatList.SetFocused(target == FocusChatList)
	if m.forum != nil {
		m.forum.SetFocused(target == FocusChatList)
	}
	m.chat.SetFocused(target == FocusChat)
	if m.folderBar != nil {
		m.folderBar.SetFocused(target == FocusFolders)
	}
	switch target {
	case FocusFolders:
		m.statusBar.SetActivePane("folders")
	case FocusChatList:
		m.statusBar.SetActivePane("chatlist")
	case FocusChat:
		m.statusBar.SetActivePane("chat")
		// A reaction that arrived while the pane was not focused is only being
		// looked at now, so this is when it counts as seen.
		if m.chatUnreadReactions > 0 {
			return m, m.readReactionsCmd(m.currentHistory())
		}
	}
	return m, nil
}
