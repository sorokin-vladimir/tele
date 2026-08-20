package ui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

func (m RootModel) View() tea.View {
	var content string
	if m.screen == ScreenLogin {
		logoView := m.logo.View()
		// Place fills everything around the centred block with whitespace, which
		// on the login screen is almost the whole terminal. Left to itself that
		// whitespace carries no background at all.
		fill := lipgloss.WithWhitespaceStyle(theme.NewStyle())
		if m.login.CurrentStep() < 0 {
			combined := joinCentred(logoView, "\n"+theme.S().Body.Render("connecting..."))
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined, fill)
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
			padded := theme.NewStyle().Padding(loginPadV, loginPadH).Render(loginContent)
			loginBox := components.RenderBox(padded, "Telegram", "", "", "", b, nil, innerW+2, innerH+2)
			combined := joinCentred(logoView, "\n", loginBox)
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined, fill)
		}
	} else {
		paneH := m.height + 1
		innerH := paneH - 2*borderSize

		activeBorder := lipgloss.DoubleBorder()
		inactiveBorder := lipgloss.NormalBorder()

		activeFg := theme.T().BorderPaneActive

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
		} else if m.chat.PeerOnline() {
			chatDot = theme.NewStyle().Foreground(theme.T().StatusOnline).Render("●")
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
			foldersView := components.RenderBox(m.folderBar.View(), "[0] Folders", "", "", "", foldersBorder, foldersFg, sidebarW, innerH, foldersSB)
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", "", chatListBorder, chatListFg, chatlistW, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", "", chatBorder, chatFg, chatW, innerH, chatSB)
			main = joinPanes(foldersView, chatListView, chatView)
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
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", "", chatListBorder, chatListFg, chatListWidth, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", "", chatBorder, chatFg, chatWidth, innerH, chatSB)
			main = joinPanes(chatListView, chatView)
			chatPanelLeft = chatListWidth
			chatBoxW = chatWidth
			chatListLeft = 0
			chatListBoxW = chatListWidth
		}

		content = main + "\n" + m.statusBar.View()
		if m.searchModel != nil {
			content = overlayCenter(dimBackground(content), m.searchModel.View(), m.width, m.height)
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
		if m.mentionPopup != nil {
			content = m.overlayAboveComposer(content, m.mentionPopup.View(), chatPanelLeft)
		}
		if m.openPicker != nil {
			content = m.overlayMenuNearBubble(content, m.openPicker.View(), chatPanelLeft, chatBoxW)
		}
		if m.filePicker != nil {
			content = overlayCenter(dimBackground(content), m.filePicker.View(), m.width, m.height)
		}
		if m.videoPlayer != nil {
			// Overlay the modal over the chat using integer geometry (the chat's
			// Kitty placeholders defeat lipgloss-based stamping).
			content = m.videoPlayerView(dimBackground(content))
		}
		if m.photoViewer != nil {
			content = m.photoViewerView(dimBackground(content))
		}
		if m.help != nil {
			content = overlayCenter(dimBackground(content), m.help.View(), m.width, m.height)
		}
		// Centred like the help modal, and for the same reason: it is a
		// reference rather than something anchored to what is behind it.
		if m.settings != nil {
			content = overlayCenter(dimBackground(content), m.settings.View(), m.width, m.height)
		}
		// The profile is centred over the whole view rather than anchored to the
		// row it was opened from: it is opened from three different places and
		// each has a different anchor, while the centre is the same for all
		// (#222).
		if m.profile != nil {
			content = overlayCenter(dimBackground(content), m.profile.View(), m.width, m.height)
		}
		// Bottom-anchored toasts must clear the composer: a limit warning is
		// useless on top of the field it is about (#126). The composer grows with
		// the draft, so the inset is read per frame.
		if m.chat != nil {
			m.toasts.SetBottomInset(m.chat.ComposerHeight())
		}
		// Toasts are stamped last so they float above every other overlay.
		for _, z := range m.toasts.Zones() {
			content = overlayAt(content, z.Block, m.width, m.height, z.Top, z.Left)
		}
	}
	// Stamped after both screen branches so a startup notice covers the login
	// splash too: the migration and deprecation messages matter most exactly
	// when the session is invalid or the network is down.
	if m.noticeActive() {
		content = m.noticeView(content)
	}
	content = m.fillCanvas(content)
	v := tea.NewView(content)
	v.AltScreen = true
	// Enable mouse reporting (clicks + wheel). CellMotion delivers button and
	// wheel events; motion events while dragging are ignored.
	v.MouseMode = tea.MouseModeCellMotion
	// Focus reporting drives the fallback re-read of the terminal background
	// color on focus regain, for terminals without OS color-scheme reporting
	// (issue #148). Terminals that do not support it simply never send the event.
	v.ReportFocus = true
	return v
}

// joinPanes places blocks side by side, padding with the canvas. It replaces
// lipgloss.JoinHorizontal(lipgloss.Top, ...) for the same reason joinCentred
// replaces JoinVertical: the padding it adds to square blocks off is bare
// spaces, and it offers no whitespace style.
//
// The padding is not hypothetical even though the panes are equal-height boxes.
// A block whose lines are not all the same width — which is what a pane that
// overflows its box produces — gets its short lines filled out to the widest,
// and that fill lands between the panes, where it is most visible.
func joinPanes(blocks ...string) string {
	split := make([][]string, len(blocks))
	widths := make([]int, len(blocks))
	rows := 0
	for i, b := range blocks {
		split[i] = strings.Split(b, "\n")
		if len(split[i]) > rows {
			rows = len(split[i])
		}
		for _, l := range split[i] {
			if lw := lipgloss.Width(l); lw > widths[i] {
				widths[i] = lw
			}
		}
	}

	out := make([]string, rows)
	for r := range rows {
		var sb strings.Builder
		for i, lines := range split {
			line := ""
			if r < len(lines) {
				line = lines[r]
			}
			sb.WriteString(line)
			sb.WriteString(theme.PadTo(lipgloss.Width(line), widths[i]))
		}
		out[r] = sb.String()
	}
	return strings.Join(out, "\n")
}

// joinCentred stacks blocks centred on their common width, padding with the
// canvas. It replaces lipgloss.JoinVertical(lipgloss.Center, ...), which does
// the same thing but pads with bare spaces and, unlike Place, offers no
// whitespace style to fix that with. On the login screen the padding it adds
// around the logo is most of the block.
func joinCentred(blocks ...string) string {
	var lines []string
	for _, b := range blocks {
		lines = append(lines, strings.Split(b, "\n")...)
	}
	w := 0
	for _, l := range lines {
		if lw := lipgloss.Width(l); lw > w {
			w = lw
		}
	}
	for i, l := range lines {
		gap := w - lipgloss.Width(l)
		left := gap / 2
		lines[i] = theme.Pad(left) + l + theme.Pad(gap-left)
	}
	return strings.Join(lines, "\n")
}

// fillCanvas carries the composed view out to the full terminal: every row to
// m.width, the view to m.height rows. Whatever it adds goes after the row's own
// content, which is after whatever reset that content ends in — the only
// position a background survives from.
//
// The panes do sum to exactly m.width today, in both layout branches, so this is
// mostly redundant mostly of the time. That is the point: the guarantee that
// every cell is painted should not rest on arithmetic in three places continuing
// to agree through every future change to how the screen is split.
func (m RootModel) fillCanvas(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return content
	}
	if theme.IsNone(theme.T().Background) {
		// Nothing to fill with. Padding rows out to the terminal width would be
		// invisible and would cost a pass over the whole screen every frame.
		return content
	}
	lines := strings.Split(content, "\n")
	for len(lines) < m.height {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = l + theme.PadTo(lipgloss.Width(l), m.width)
	}
	return strings.Join(lines, "\n")
}

// overlayMenuNearBubble places a menu next to the selected message bubble: left
// of outgoing bubbles, right of incoming, top-aligned, clamped to the chat
// panel. If the bubble geometry is unavailable (no selection, scrolled out,
// empty chat) it falls back to the bottom-right corner.
// overlayAboveComposer stamps the mention popup just above the composer box, at
// the left edge of the chat panel. The composer sits at the bottom of the chat
// pane above the 1-row status bar; the popup is placed directly on top of it.
func (m RootModel) overlayAboveComposer(content, popup string, chatPanelLeft int) string {
	_, popupH := measureBox(popup)
	// h-1 is the status bar row; the composer occupies ComposerHeight() rows
	// above it; place the popup immediately above the composer.
	top := m.height - 1 - m.chat.ComposerHeight() - popupH
	if top < 0 {
		top = 0
	}
	left := chatPanelLeft + 1
	return overlayAt(content, popup, m.width, m.height, top, left)
}

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
