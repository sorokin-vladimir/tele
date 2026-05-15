package screens

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

type OpenChatMsg struct{ Chat store.Chat }

var (
	selectedChatStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	normalChatStyle   = lipgloss.NewStyle()
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
	chats   []store.Chat
	cursor  int
	width   int
	height  int
	focused bool
}

func NewChatListModel() *ChatListModel {
	return &ChatListModel{}
}

func (m *ChatListModel) SetChats(chats []store.Chat) {
	// preserve unread counts accumulated since last store sync
	oldUnread := make(map[int64]int, len(m.chats))
	for _, c := range m.chats {
		if c.UnreadCount > 0 {
			oldUnread[c.ID] = c.UnreadCount
		}
	}

	var selectedID int64
	if m.cursor < len(m.chats) {
		selectedID = m.chats[m.cursor].ID
	}
	m.chats = chats
	for i, c := range m.chats {
		if n, ok := oldUnread[c.ID]; ok {
			m.chats[i].UnreadCount = n
		}
	}
	m.cursor = 0
	for i, c := range m.chats {
		if c.ID == selectedID {
			m.cursor = i
			break
		}
	}
}
func (m *ChatListModel) Cursor() int                 { return m.cursor }
func (m *ChatListModel) Chats() []store.Chat         { return m.chats }

func (m *ChatListModel) IncrementUnread(chatID int64) {
	for i := range m.chats {
		if m.chats[i].ID == chatID {
			m.chats[i].UnreadCount++
			return
		}
	}
}

func (m *ChatListModel) SelectedChat() (store.Chat, bool) {
	if len(m.chats) == 0 || m.cursor >= len(m.chats) {
		return store.Chat{}, false
	}
	return m.chats[m.cursor], true
}
func (m *ChatListModel) Context() keys.Context       { return keys.ContextChatList }
func (m *ChatListModel) Focused() bool               { return m.focused }
func (m *ChatListModel) SetFocused(f bool)           { m.focused = f }

func (m *ChatListModel) SetSize(width, height int) {
	m.width = width
	m.height = height
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
		case keys.ActionConfirm:
			if len(m.chats) > 0 {
				chat := m.chats[m.cursor]
				return m, func() tea.Msg { return OpenChatMsg{Chat: chat} }
			}
		}
	}
	return m, nil
}

func (m *ChatListModel) View() string {
	if len(m.chats) == 0 {
		return "Loading chats..."
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
	// Build lines to w-1 display columns so that the outer lipgloss container's
	// Width(w) padding never triggers word-wrap (which fires when content >= w).
	inner := w - 1
	if inner < 1 {
		inner = 1
	}

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		badge := formatUnread(m.chats[i].UnreadCount)
		title := m.chats[i].Title
		var line string
		if badge == "" {
			line = runewidth.Truncate(title, inner, "")
			lw := runewidth.StringWidth(line)
			if lw < inner {
				line += strings.Repeat(" ", inner-lw)
			}
		} else {
			badgeW := len(badge) // badge is ASCII only
			maxTitleW := inner - badgeW - 1
			if maxTitleW < 0 {
				maxTitleW = 0
			}
			truncTitle := runewidth.Truncate(title, maxTitleW, "")
			titleW := runewidth.StringWidth(truncTitle)
			pad := inner - titleW - badgeW
			if pad < 0 {
				pad = 0
			}
			line = truncTitle + strings.Repeat(" ", pad) + badge
		}
		style := normalChatStyle
		if i == m.cursor {
			style = selectedChatStyle
		}
		lines = append(lines, style.Inline(true).Render(line))
	}
	return strings.Join(lines, "\n")
}
