package screens

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

func (m *ChatListModel) SetChats(chats []store.Chat) { m.chats = chats }
func (m *ChatListModel) Cursor() int                 { return m.cursor }

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
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		badge := formatUnread(m.chats[i].UnreadCount)
		title := m.chats[i].Title
		var line string
		if badge == "" {
			line = title
		} else {
			maxTitle := w - len(badge) - 1
			if maxTitle < 0 {
				maxTitle = 0
			}
			runes := []rune(title)
			if len(runes) > maxTitle {
				runes = runes[:maxTitle]
			}
			pad := w - len(runes) - len(badge)
			if pad < 0 {
				pad = 0
			}
			line = string(runes) + strings.Repeat(" ", pad) + badge
		}
		if i == m.cursor {
			lines = append(lines, selectedChatStyle.Inline(true).Width(w).MaxWidth(w).Render(line))
		} else {
			lines = append(lines, normalChatStyle.Inline(true).Width(w).MaxWidth(w).Render(line))
		}
	}
	return strings.Join(lines, "\n")
}
