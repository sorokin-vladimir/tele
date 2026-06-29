package ui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

func (m RootModel) View() tea.View {
	var content string
	if m.screen == ScreenLogin {
		logoView := m.logo.View()
		if m.login.CurrentStep() < 0 {
			combined := lipgloss.JoinVertical(lipgloss.Center, logoView, "\n"+"connecting...")
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
		} else {
			loginContent := m.login.View().Content
			b := lipgloss.RoundedBorder()
			loginLines := strings.Split(loginContent, "\n")
			loginContentH := len(loginLines)
			loginContentW := 0
			for _, l := range loginLines {
				if w := lipgloss.Width(l); w > loginContentW {
					loginContentW = w
				}
			}
			const loginPadV, loginPadH = 1, 3
			innerW := loginContentW + 2*loginPadH
			innerH := loginContentH + 2*loginPadV
			padded := lipgloss.NewStyle().Padding(loginPadV, loginPadH).Render(loginContent)
			loginBox := components.RenderBox(padded, "Telegram", "", "", b, nil, innerW+2, innerH+2)
			combined := lipgloss.JoinVertical(lipgloss.Center, logoView, "\n", loginBox)
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
		}
	} else {
		paneH := m.height + 1
		innerH := paneH - 2*borderSize

		activeBorder := lipgloss.DoubleBorder()
		inactiveBorder := lipgloss.NormalBorder()

		lightDark := lipgloss.LightDark(m.hasDarkBackground)
		activeFg := lightDark(lipgloss.Color("22"), lipgloss.Color("10"))

		foldersBorder := inactiveBorder
		chatListBorder := inactiveBorder
		chatBorder := inactiveBorder
		var foldersFg, chatListFg, chatFg color.Color
		switch m.focus {
		case FocusFolders:
			foldersBorder = activeBorder
			foldersFg = activeFg
		case FocusChatList:
			chatListBorder = activeBorder
			chatListFg = activeFg
		case FocusChat:
			chatBorder = activeBorder
			chatFg = activeFg
		}

		chatListTitle := "[1] Chats"
		chatTitle := "[2] " + m.chat.Title()
		chatDot := ""
		if m.chat.IsTyping() {
			chatDot = m.chat.TypingLabel()
		} else if m.currentChatID != 0 && m.st != nil {
			if chat, ok := m.st.GetChat(m.currentChatID); ok && chat.Peer.IsUser() && chat.Online {
				chatDot = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
			}
		}

		var main string
		var chatPanelLeft, chatBoxW int
		var chatListLeft, chatListBoxW int
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
			foldersSB := &components.Scrollbar{Info: m.folderBar.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatListSB := &components.Scrollbar{Info: m.chatList.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatSB := &components.Scrollbar{Info: m.chat.ScrollInfo(), TrackTop: 0, TrackLen: m.chat.MessageListHeight()}
			foldersView := components.RenderBox(m.folderBar.View(), "[0] Folders", "", "", foldersBorder, foldersFg, sidebarW, innerH, foldersSB)
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatlistW, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatW, innerH, chatSB)
			main = lipgloss.JoinHorizontal(lipgloss.Top, foldersView, chatListView, chatView)
			chatPanelLeft = sidebarW + chatlistW
			chatBoxW = chatW
			chatListLeft = sidebarW
			chatListBoxW = chatlistW
		} else {
			leftW, rightW := layout.SplitHorizontal(m.width, m.height, 0.30)
			chatListWidth := leftW - 2*borderSize + 2
			chatWidth := rightW - 2*borderSize + 2
			chatListSB := &components.Scrollbar{Info: m.chatList.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatSB := &components.Scrollbar{Info: m.chat.ScrollInfo(), TrackTop: 0, TrackLen: m.chat.MessageListHeight()}
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatListWidth, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatWidth, innerH, chatSB)
			main = lipgloss.JoinHorizontal(lipgloss.Top, chatListView, chatView)
			chatPanelLeft = chatListWidth
			chatBoxW = chatWidth
			chatListLeft = 0
			chatListBoxW = chatListWidth
		}

		content = main + "\n" + m.statusBar.View()
		if m.searchModel != nil {
			content = overlayCenter(dimBackground(content, m.hasDarkBackground), m.searchModel.View(), m.width, m.height)
		}
		if m.contextMenu != nil {
			content = m.overlayMenuNearBubble(content, m.contextMenu.View(), chatPanelLeft, chatBoxW)
		}
		if m.chatMenu != nil {
			content = m.overlayMenuNearChatRow(content, m.chatMenu.View(), chatListLeft, chatListBoxW)
		}
		if m.reactionPicker != nil {
			content = m.overlayMenuNearBubble(content, m.reactionPicker.View(), chatPanelLeft, chatBoxW)
		}
		if m.filePicker != nil {
			content = overlayCenter(dimBackground(content, m.hasDarkBackground), m.filePicker.View(), m.width, m.height)
		}
		if m.videoPlayer != nil {
			// Overlay the modal over the chat using integer geometry (the chat's
			// Kitty placeholders defeat lipgloss-based stamping).
			content = m.videoPlayerView(dimBackground(content, m.hasDarkBackground))
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// Focus reporting drives the fallback re-read of the terminal background
	// color on focus regain, for terminals without OS color-scheme reporting
	// (issue #148). Terminals that do not support it simply never send the event.
	v.ReportFocus = true
	return v
}

// overlayMenuNearBubble places a menu next to the selected message bubble: left
// of outgoing bubbles, right of incoming, top-aligned, clamped to the chat
// panel. If the bubble geometry is unavailable (no selection, scrolled out,
// empty chat) it falls back to the bottom-right corner.
func (m RootModel) overlayMenuNearBubble(content, menu string, chatPanelLeft, chatBoxW int) string {
	rect, ok := m.chat.SelectedBubbleRect()
	if !ok {
		return overlayBottomRight(content, menu, m.width, m.height, m.chat.ComposerHeight()+1)
	}

	// rect is local to the message list's output. The chat box sits at terminal
	// row 0; RenderBox adds a 1-cell top/left border; the message list is at the
	// top of the chat content, so no extra vertical offset is needed.
	bubble := components.Rect{
		Top:    1 + rect.Top,
		Left:   chatPanelLeft + 1 + rect.Left,
		Height: rect.Height,
		Width:  rect.Width,
	}
	area := components.Rect{
		Top:    1,
		Left:   chatPanelLeft + 1,
		Height: m.chat.MessageListHeight(),
		Width:  chatBoxW - 2,
	}

	menuW, menuH := measureBox(menu)
	top, left := anchorMenu(bubble, area, menuW, menuH, m.chat.SelectedMessageIsOut())
	return overlayAt(content, menu, m.width, m.height, top, left)
}

// overlayMenuNearChatRow places a menu to the right of the selected
// chat-list row, top-aligned to that row and clamped to the main content
// area so it stays on screen.
func (m RootModel) overlayMenuNearChatRow(content, menu string, chatListLeft, chatListBoxW int) string {
	row := m.chatList.CursorViewportRow()
	// The chat-list box sits at terminal row 0; RenderBox adds a 1-cell
	// top/left border, so the first row of content is terminal row 1.
	rowRect := components.Rect{
		Top:    1 + row,
		Left:   chatListLeft,
		Height: 1,
		Width:  chatListBoxW,
	}
	area := components.Rect{
		Top:    1,
		Left:   chatListLeft,
		Height: m.chatList.Height(),
		Width:  m.width - chatListLeft,
	}
	menuW, menuH := measureBox(menu)
	// onLeft=false anchors to the right of the row (into the chat pane).
	top, left := anchorMenu(rowRect, area, menuW, menuH, false)
	return overlayAt(content, menu, m.width, m.height, top, left)
}
