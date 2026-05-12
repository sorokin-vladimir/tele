package screens

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sorokin-vladimir/tele/internal/store"
)

type CloseSearchMsg struct{}

var (
	searchBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("63"))
	searchActiveRow = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	searchNormalRow = lipgloss.NewStyle()
	searchPrompt    = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
)

const searchOverlayWidth = 50
const searchMaxResults = 8

type SearchModel struct {
	query   string
	all     []store.Chat
	results []store.Chat
	cursor  int
	width   int
	height  int
}

func NewSearchModel(chats []store.Chat, width, height int) *SearchModel {
	m := &SearchModel{all: chats, width: width, height: height}
	m.results = make([]store.Chat, len(chats))
	copy(m.results, chats)
	return m
}

func (m *SearchModel) Cursor() int { return m.cursor }

func (m *SearchModel) Update(msg tea.Msg) (*SearchModel, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch km.Type {
	case tea.KeyEsc:
		return m, func() tea.Msg { return CloseSearchMsg{} }
	case tea.KeyEnter:
		if len(m.results) > 0 {
			chat := m.results[m.cursor]
			return m, func() tea.Msg { return OpenChatMsg{Chat: chat} }
		}
		return m, nil
	case tea.KeyBackspace:
		if len(m.query) > 0 {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			m.filter()
		}
		return m, nil
	case tea.KeyRunes:
		switch string(km.Runes) {
		case "j":
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		default:
			m.query += string(km.Runes)
			m.filter()
		}
	}
	return m, nil
}

func (m *SearchModel) filter() {
	q := strings.ToLower(m.query)
	filtered := make([]store.Chat, 0, len(m.all))
	for _, c := range m.all {
		if q == "" || strings.Contains(strings.ToLower(c.Title), q) {
			filtered = append(filtered, c)
		}
	}
	m.results = filtered
	if m.cursor >= len(m.results) {
		m.cursor = 0
	}
}

func (m *SearchModel) View() string {
	w := searchOverlayWidth
	if m.width > 0 && m.width < w {
		w = m.width
	}
	inner := w - 2

	queryLine := searchPrompt.Render("> ") + m.query + "█"
	divider := strings.Repeat("─", inner)

	lines := []string{queryLine, divider}
	for i, c := range m.results {
		if i >= searchMaxResults {
			break
		}
		row := c.Title
		if i == m.cursor {
			lines = append(lines, searchActiveRow.Inline(true).Width(inner).MaxWidth(inner).Render(row))
		} else {
			lines = append(lines, searchNormalRow.Inline(true).Width(inner).MaxWidth(inner).Render(row))
		}
	}
	if len(m.results) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(inner).Render("no results"))
	}

	return searchBorderStyle.Width(inner).Render(strings.Join(lines, "\n"))
}
