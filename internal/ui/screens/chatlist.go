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

var (
	selectedChatStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	normalChatStyle   = lipgloss.NewStyle()
	activeChatStyle   = lipgloss.NewStyle().Bold(true)
	onlineDotStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
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

type ChatListModel struct {
	chats     []store.Chat
	cursor    int
	activeIdx int
	width     int
	height    int
	focused   bool
	spinner   components.Spinner
}

func NewChatListModel() *ChatListModel {
	return &ChatListModel{}
}

// TickSpinner advances the spinner frame. Called by root on SpinnerTickMsg.
func (m *ChatListModel) TickSpinner() { m.spinner.Tick() }

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
		badge := formatUnread(m.chats[i].UnreadCount)
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
			content = runewidth.Truncate(title, inner, "…")
			lw := lipgloss.Width(content)
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
			content = truncTitle + strings.Repeat(" ", pad) + badge
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
