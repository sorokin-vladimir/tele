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

	var sb strings.Builder
	for i := start; i < end; i++ {
		w := m.width - 2
		if w < 1 {
			w = 1
		}
		line := fmt.Sprintf(" %-*s", w, m.chats[i].Title)
		if i == m.cursor {
			sb.WriteString(selectedChatStyle.Render(line))
		} else {
			sb.WriteString(normalChatStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
