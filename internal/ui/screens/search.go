package screens

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

type CloseSearchMsg struct{}

var (
	searchActiveRow = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("0"))
	searchNormalRow = lipgloss.NewStyle()
	searchPrompt    = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
)

const searchOverlayWidth = 50
const searchMaxResults = 8

type forwardPhase int

const (
	forwardSelect forwardPhase = iota
	forwardComment
)

type SearchModel struct {
	query   string
	all     []store.Chat
	results []store.Chat
	list    *components.ListView
	width   int
	height  int
	keyMap  keys.KeyMap
	// forwardMsgID > 0 puts the picker in forward mode: confirming a chat emits
	// ForwardToChatRequest{ToPeer, MsgID} and rows show the unread count.
	forwardMsgID int
	phase        forwardPhase // forward mode only: select (default) | comment
	comment      string
	target       store.Chat // chat chosen when entering the comment phase
}

func NewSearchModel(chats []store.Chat, width, height int, km keys.KeyMap) *SearchModel {
	m := &SearchModel{all: chats, width: width, height: height, keyMap: km, list: components.NewListView(false)}
	m.results = make([]store.Chat, len(chats))
	copy(m.results, chats)
	m.list.SetCount(len(m.results))
	return m
}

// NewForwardPicker builds the chat picker in forward mode: confirming a chat
// emits ForwardToChatRequest{ToPeer, MsgID} and rows show the unread count.
func NewForwardPicker(chats []store.Chat, msgID int, width, height int, km keys.KeyMap) *SearchModel {
	m := NewSearchModel(chats, width, height, km)
	m.forwardMsgID = msgID
	return m
}

func (m *SearchModel) hint() string {
	if m.keyMap == nil {
		return ""
	}
	cancel := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionCancel)
	confirm := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionConfirm)
	down := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionDown)
	up := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionUp)

	if m.forwardMsgID != 0 && m.phase == forwardComment {
		parts := []string{}
		if confirm != "" {
			parts = append(parts, confirm+" -> send")
		}
		if cancel != "" {
			parts = append(parts, cancel+" -> back")
		}
		return strings.Join(parts, " | ")
	}

	downSym := arrowSym(down)
	upSym := arrowSym(up)
	parts := []string{}
	if downSym != "" || upSym != "" {
		parts = append(parts, upSym+"/"+downSym+" -> move")
	}
	if confirm != "" {
		verb := "open"
		if m.forwardMsgID != 0 {
			verb = "forward"
		}
		parts = append(parts, confirm+" -> "+verb)
	}
	if m.forwardMsgID != 0 {
		parts = append(parts, "tab -> comment")
	}
	if cancel != "" {
		parts = append(parts, cancel+" -> close")
	}
	return strings.Join(parts, " | ")
}

func arrowSym(key string) string {
	switch key {
	case "up":
		return "↑"
	case "down":
		return "↓"
	default:
		return key
	}
}

func (m *SearchModel) Cursor() int           { return m.list.Cursor() }
func (m *SearchModel) Query() string         { return m.query }
func (m *SearchModel) Results() []store.Chat { return m.results }

func (m *SearchModel) Update(msg tea.Msg) (*SearchModel, tea.Cmd) {
	if m.forwardMsgID != 0 && m.phase == forwardComment {
		return m.updateComment(msg)
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.query += paste.Content
		m.filter()
		return m, nil
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch km.Code {
	case tea.KeyTab:
		if m.forwardMsgID != 0 && len(m.results) > 0 {
			m.target = m.results[m.list.Cursor()]
			m.comment = ""
			m.phase = forwardComment
		}
		return m, nil
	case tea.KeyEsc:
		return m, func() tea.Msg { return CloseSearchMsg{} }
	case tea.KeyEnter:
		if len(m.results) > 0 {
			chat := m.results[m.list.Cursor()]
			if m.forwardMsgID != 0 {
				msgID := m.forwardMsgID
				peer := chat.Peer
				return m, func() tea.Msg { return ForwardToChatRequest{ToPeer: peer, MsgID: msgID} }
			}
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
	case tea.KeyDown:
		m.list.MoveDown()
		return m, nil
	case tea.KeyUp:
		m.list.MoveUp()
		return m, nil
	default:
		switch km.String() {
		case "ctrl+j":
			m.list.MoveDown()
		case "ctrl+k":
			m.list.MoveUp()
		default:
			if km.Text != "" {
				m.query += km.Text
				m.filter()
			}
		}
		return m, nil
	}
}

// updateComment handles the forward-mode comment phase: typed text (any layout,
// via km.Text) builds the comment, Enter emits the forward request with it, and
// Esc returns to the select phase.
func (m *SearchModel) updateComment(msg tea.Msg) (*SearchModel, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.comment += paste.Content
		return m, nil
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch km.Code {
	case tea.KeyEsc:
		m.phase = forwardSelect
		return m, nil
	case tea.KeyEnter:
		comment := m.comment
		peer := m.target.Peer
		msgID := m.forwardMsgID
		return m, func() tea.Msg {
			return ForwardToChatRequest{ToPeer: peer, MsgID: msgID, Comment: comment}
		}
	case tea.KeyBackspace:
		if len(m.comment) > 0 {
			r := []rune(m.comment)
			m.comment = string(r[:len(r)-1])
		}
		return m, nil
	default:
		if km.Text != "" {
			m.comment += km.Text
		}
		return m, nil
	}
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
	m.list.SetCount(len(m.results))
}

func (m *SearchModel) View() string {
	w := searchOverlayWidth
	if m.width > 0 && m.width < w {
		w = m.width
	}

	hint := m.hint()
	if hintW := lipgloss.Width(" " + hint + " "); hintW+2 > w-2 {
		w = hintW + 4
	}

	inner := w - 2

	if m.forwardMsgID != 0 && m.phase == forwardComment {
		title := "Comment to " + m.target.Title
		promptLine := searchPrompt.Render("> ") + m.comment + "█"
		lines := []string{title, strings.Repeat("─", inner), promptLine}
		content := strings.Join(lines, "\n")
		h := len(lines) + 2
		return components.RenderBox(content, "", "", hint, lipgloss.RoundedBorder(), nil, w, h)
	}

	queryLine := searchPrompt.Render("> ") + m.query + "█"
	divider := strings.Repeat("─", inner)

	lines := []string{queryLine, divider}
	rowFn := func(i int, selected bool) string {
		c := m.results[i]
		row := c.Title
		if m.forwardMsgID != 0 && c.UnreadCount > 0 {
			row = fmt.Sprintf("%s (%d)", c.Title, c.UnreadCount)
		}
		if selected {
			return searchActiveRow.Inline(true).Width(inner).MaxWidth(inner).Render(row)
		}
		return searchNormalRow.Inline(true).Width(inner).MaxWidth(inner).Render(row)
	}
	lines = append(lines, m.list.Render(searchMaxResults, rowFn)...)
	if len(m.results) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Width(inner).Render("no results"))
	}

	content := strings.Join(lines, "\n")
	h := len(lines) + 2
	return components.RenderBox(content, "", "", hint, lipgloss.RoundedBorder(), nil, w, h)
}
