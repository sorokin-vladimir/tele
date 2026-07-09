package screens

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

type OpenChatMsg struct{ Chat store.Chat }

// ForwardToChatRequest is emitted by the forward-mode chat picker when the user
// confirms a target chat. The source peer is resolved by the root model.
type ForwardToChatRequest struct {
	ToPeer  store.Peer
	MsgID   int
	Comment string // optional; sent as a separate message before the forward
}

var (
	selectedChatStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	normalChatStyle   = lipgloss.NewStyle()
	activeChatStyle   = lipgloss.NewStyle().Bold(true)
	onlineDotStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	// reactionStyle tints the unread-reaction glyph pink to stand apart from the
	// numeric unread badge.
	reactionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	// chatHighlightBase is the tone the chat-row title highlight fades toward
	// (approximates default light-grey text on a dark background).
	chatHighlightBase = lipgloss.Color("250")
)

func formatUnread(count int) string {
	if count <= 0 {
		return ""
	}
	if count > 99 {
		return "[99+]"
	}
	return fmt.Sprintf("[%d]", count)
}

// formatReactions renders the unread-reaction token: empty when none, a bare
// heart for one, or a heart with the count for many.
func formatReactions(count int) string {
	switch {
	case count <= 0:
		return ""
	case count == 1:
		return "♥"
	default:
		return fmt.Sprintf("♥%d", count)
	}
}

// rowIndicators builds the right-aligned status column for a chat row. Tokens
// appear in order [mute] [reaction] [unread], each separated by a single space
// and omitted when empty: the dim mute marker, the pink unread-reaction glyph,
// then the unread token (numeric badge, or a manual-unread dot when marked
// unread with no real count).
func rowIndicators(c store.Chat) string {
	var unread string
	switch {
	case c.UnreadCount > 0:
		unread = formatUnread(c.UnreadCount)
	case c.UnreadMark:
		unread = "[•]"
	}
	var reaction string
	if c.UnreadReactionsCount > 0 {
		reaction = reactionStyle.Render(formatReactions(c.UnreadReactionsCount))
	}
	var muted string
	if c.IsMuted {
		muted = mutedStyle.Render("×")
	}
	parts := make([]string, 0, 3)
	for _, p := range []string{muted, reaction, unread} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

type ChatListModel struct {
	chats             []store.Chat
	cursor            int
	activeIdx         int
	width             int
	height            int
	focused           bool
	hasDarkBackground bool
	spinner           components.Spinner

	// highlightChatID is the chat currently flashed because a new incoming
	// message bumped it to the top; highlightStep counts down to 0 (none).
	// Tracked by id so the highlight survives SetChats reorders.
	highlightChatID int64
	highlightStep   int
}

func NewChatListModel() *ChatListModel {
	return &ChatListModel{}
}

// TickSpinner advances the spinner frame. Called by root on SpinnerTickMsg.
func (m *ChatListModel) TickSpinner() { m.spinner.Tick() }

// IsLoadingChats reports whether the chat list is still showing its
// "Loading chats..." spinner (no chats received yet), matching View. Drives the
// spinner tick loop (issue #147).
func (m *ChatListModel) IsLoadingChats() bool { return len(m.chats) == 0 }

func (m *ChatListModel) SetChats(chats []store.Chat) {
	var cursorID int64
	if m.cursor < len(m.chats) {
		cursorID = m.chats[m.cursor].ID
	}
	var activeID int64
	if m.activeIdx < len(m.chats) {
		activeID = m.chats[m.activeIdx].ID
	}

	m.chats = chats

	m.cursor = 0
	for i, c := range m.chats {
		if c.ID == cursorID {
			m.cursor = i
			break
		}
	}

	m.activeIdx = 0
	for i, c := range m.chats {
		if c.ID == activeID {
			m.activeIdx = i
			break
		}
	}
}

// HighlightChat starts a fade highlight on the chat-list row for the given id.
func (m *ChatListModel) HighlightChat(id int64) {
	m.highlightChatID = id
	m.highlightStep = components.HighlightInitialStep
}

// StepChatHighlight advances the chat-row highlight fade by one step. Returns
// true while still active; clears the highlight and returns false at 0. No-op
// (false) when no highlight is active.
func (m *ChatListModel) StepChatHighlight() bool {
	if m.highlightStep <= 0 {
		return false
	}
	m.highlightStep--
	if m.highlightStep <= 0 {
		m.highlightChatID = 0
		return false
	}
	return true
}

// HighlightedChatID returns the currently highlighted chat id (0 when none).
func (m *ChatListModel) HighlightedChatID() int64 { return m.highlightChatID }

// HighlightStep returns the current chat-row fade step (0 when none).
func (m *ChatListModel) HighlightStep() int { return m.highlightStep }

// styleTitle applies the fade-accent foreground to a row's (already truncated)
// title while that row is the active highlight target. The focused-cursor row
// keeps its selection background instead, so it is left unstyled here.
func (m *ChatListModel) styleTitle(i int, truncated string) string {
	if m.highlightStep <= 0 || m.chats[i].ID != m.highlightChatID {
		return truncated
	}
	if i == m.cursor && m.focused {
		return truncated
	}
	fg := components.FadeAccentColor(components.HighlightAccentFor(m.hasDarkBackground), chatHighlightBase, m.highlightStep, components.HighlightFadeSteps)
	return lipgloss.NewStyle().Foreground(fg).Render(truncated)
}

// SetDarkBackground records whether the terminal background is dark, so the
// row highlight picks the accent tone suited to the theme.
func (m *ChatListModel) SetDarkBackground(isDark bool) { m.hasDarkBackground = isDark }

func (m *ChatListModel) Cursor() int         { return m.cursor }
func (m *ChatListModel) ActiveIdx() int      { return m.activeIdx }
func (m *ChatListModel) Chats() []store.Chat { return m.chats }

func (m *ChatListModel) SetActiveByID(id int64) {
	for i, c := range m.chats {
		if c.ID == id {
			m.activeIdx = i
			m.cursor = i
			return
		}
	}
}

func (m *ChatListModel) SetChatUnread(chatID int64, count int) {
	for i := range m.chats {
		if m.chats[i].ID == chatID {
			m.chats[i].UnreadCount = count
			return
		}
	}
}

func (m *ChatListModel) SelectedChat() (store.Chat, bool) {
	if len(m.chats) == 0 || m.activeIdx >= len(m.chats) {
		return store.Chat{}, false
	}
	return m.chats[m.activeIdx], true
}

func (m *ChatListModel) SetCursorByID(id int64) {
	for i, c := range m.chats {
		if c.ID == id {
			m.cursor = i
			return
		}
	}
}
func (m *ChatListModel) Context() keys.Context { return keys.ContextChatList }
func (m *ChatListModel) Focused() bool         { return m.focused }
func (m *ChatListModel) SetFocused(f bool)     { m.focused = f }

func (m *ChatListModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// CursorChat returns the chat currently under the cursor.
func (m *ChatListModel) CursorChat() (store.Chat, bool) {
	if m.cursor < 0 || m.cursor >= len(m.chats) {
		return store.Chat{}, false
	}
	return m.chats[m.cursor], true
}

// Height returns the pane's content height in rows.
func (m *ChatListModel) Height() int { return m.height }

// ScrollInfo reports the chat list's scroll position for the pane scrollbar.
// Offset mirrors the scroll math in View / CursorViewportRow.
func (m *ChatListModel) ScrollInfo() components.ScrollInfo {
	if m.height <= 0 {
		return components.ScrollInfo{Total: len(m.chats), Visible: len(m.chats), Offset: 0}
	}
	start := 0
	if m.cursor >= m.height {
		start = m.cursor - m.height + 1
	}
	return components.ScrollInfo{Total: len(m.chats), Visible: m.height, Offset: start}
}

// CursorViewportRow returns the cursor's row index within the visible
// viewport, accounting for scroll. It mirrors the scroll math in View.
func (m *ChatListModel) CursorViewportRow() int {
	visible := m.height
	if visible <= 0 {
		return m.cursor
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	return m.cursor - start
}

// SetCursor moves the selection cursor to i, clamped to the valid range. It is
// a no-op when the list is empty.
func (m *ChatListModel) SetCursor(i int) {
	if len(m.chats) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(m.chats) {
		i = len(m.chats) - 1
	}
	m.cursor = i
}

// ChatIndexAtViewportRow maps a content row (0-based, within the visible
// viewport) to a chat index, mirroring the scroll math in View: each chat
// occupies exactly one row and the viewport starts at
// start = max(0, cursor-visible+1). ok is false when the row is empty
// (past the last visible chat or negative).
func (m *ChatListModel) ChatIndexAtViewportRow(row int) (int, bool) {
	if row < 0 || len(m.chats) == 0 {
		return 0, false
	}
	visible := m.height
	if visible <= 0 {
		visible = len(m.chats)
	}
	if row >= visible {
		return 0, false
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	idx := start + row
	if idx >= len(m.chats) {
		return 0, false
	}
	return idx, true
}

func (m *ChatListModel) Init() tea.Cmd { return nil }

func (m *ChatListModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case keys.ActionMsg:
		switch msg.Action {
		case keys.ActionDown:
			if m.cursor < len(m.chats)-1 {
				m.cursor++
			}
		case keys.ActionUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case keys.ActionGoTop:
			m.cursor = 0
		case keys.ActionGoBottom:
			if len(m.chats) > 0 {
				m.cursor = len(m.chats) - 1
			}
		case keys.ActionScrollHalfDown:
			step := m.height * 2 / 3
			if step < 1 {
				step = 1
			}
			m.cursor += step
			if m.cursor >= len(m.chats) {
				m.cursor = len(m.chats) - 1
			}
		case keys.ActionScrollHalfUp:
			step := m.height * 2 / 3
			if step < 1 {
				step = 1
			}
			m.cursor -= step
			if m.cursor < 0 {
				m.cursor = 0
			}
		case keys.ActionConfirm:
			if len(m.chats) > 0 {
				m.activeIdx = m.cursor
				chat := m.chats[m.cursor]
				return m, func() tea.Msg { return OpenChatMsg{Chat: chat} }
			}
		}
	}
	return m, nil
}

func (m *ChatListModel) View() string {
	if len(m.chats) == 0 {
		return m.spinner.View() + " Loading chats..."
	}
	visible := m.height
	if visible <= 0 {
		visible = len(m.chats)
	}
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := start + visible
	if end > len(m.chats) {
		end = len(m.chats)
	}

	w := m.width
	if w < 1 {
		w = 1
	}
	// Subtract 1 for outer container safety and 4 for the selection + presence prefix.
	const prefixW = 4
	inner := w - 1 - prefixW
	if inner < 1 {
		inner = 1
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		badge := rowIndicators(m.chats[i])
		title := m.chats[i].Title

		prefix := "    "
		if i == m.activeIdx {
			prefix = "▶   "
		}
		if m.chats[i].Peer.IsUser() && m.chats[i].Online {
			dot := onlineDotStyle.Render("●")
			if i == m.activeIdx {
				prefix = "▶ " + dot + " "
			} else {
				prefix = "  " + dot + " "
			}
		}

		var content string
		if badge == "" {
			trunc := runewidth.Truncate(title, inner, "…")
			lw := lipgloss.Width(trunc)
			content = m.styleTitle(i, trunc)
			if lw < inner {
				content += strings.Repeat(" ", inner-lw)
			}
		} else {
			badgeW := lipgloss.Width(badge)
			maxTitleW := inner - badgeW - 1
			if maxTitleW < 0 {
				maxTitleW = 0
			}
			truncTitle := runewidth.Truncate(title, maxTitleW, "…")
			titleW := lipgloss.Width(truncTitle)
			pad := inner - titleW - badgeW
			if pad < 0 {
				pad = 0
			}
			content = m.styleTitle(i, truncTitle) + strings.Repeat(" ", pad) + badge
		}

		line := prefix + content

		style := normalChatStyle
		if i == m.cursor && m.focused {
			style = selectedChatStyle
		} else if i == m.activeIdx {
			style = activeChatStyle
		}
		lines = append(lines, style.Inline(true).Render(line))
	}
	return strings.Join(lines, "\n")
}
