package screens

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// OpenTopicMsg asks the root model to open one topic of a forum. The title is
// what the header draws until the topic's own projection arrives (#275).
type OpenTopicMsg struct {
	ChatID  int64
	TopicID int
	Title   string
}

// LeaveForumMsg asks the root model to turn the middle pane back from a forum's
// topic list to the chat list.
type LeaveForumMsg struct {
	ChatID int64
}

// TopicTitle is what a topic is called on screen: its title, or its id for a
// topic whose messages arrived before anything about it did.
func TopicTitle(r project.TopicRow) string {
	if r.ID == domain.AllTopicsID {
		return "All"
	}
	if r.Title != "" {
		return r.Title
	}
	return "Topic " + strconv.Itoa(r.ID)
}

// TopicListModel renders a window onto one forum's topics, in the middle pane
// where the chat list was. A topic row reads like a chat row, with a marker in
// front where a chat shows its presence: * for All, # for an open topic, - for
// a closed one (#275).
type TopicListModel struct {
	rowList[project.TopicRow, int64]
	chatID int64
	title  string
	width  int
}

func NewTopicListModel(chatID int64, title string) *TopicListModel {
	return &TopicListModel{
		rowList: rowList[project.TopicRow, int64]{keyOf: func(r project.TopicRow) int64 { return int64(r.ID) }},
		chatID:  chatID,
		title:   title,
	}
}

// ChatID is the forum the list shows; Title its name, for the pane's frame.
func (m *TopicListModel) ChatID() int64 { return m.chatID }
func (m *TopicListModel) Title() string { return m.title }

func (m *TopicListModel) SetActive(id int64)     { m.SetActiveKey(id) }
func (m *TopicListModel) SetActiveByID(id int64) { m.SetActiveByKey(id) }
func (m *TopicListModel) SetCursorByID(id int64) { m.SetCursorByKey(id) }

// HighlightTopic starts a fade highlight on a topic's row.
func (m *TopicListModel) HighlightTopic(id int) { m.highlight(int64(id)) }

// HighlightedTopicID returns the topic currently flashed (0 when none).
func (m *TopicListModel) HighlightedTopicID() int64 { return m.highlightKey }

// CursorTopic returns the row under the cursor.
func (m *TopicListModel) CursorTopic() (project.TopicRow, bool) { return m.rowAt(m.cursor) }

// TopicIndexAtViewportRow maps a viewport row to a whole-list index.
func (m *TopicListModel) TopicIndexAtViewportRow(row int) (int, bool) {
	return m.indexAtViewportRow(row)
}

func (m *TopicListModel) Context() keys.Context { return keys.ContextChatList }

func (m *TopicListModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

func (m *TopicListModel) Init() tea.Cmd { return nil }

func (m *TopicListModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	action, ok := msg.(keys.ActionMsg)
	if !ok {
		return m, nil
	}
	if m.move(action.Action) {
		return m, nil
	}
	switch action.Action {
	case keys.ActionConfirm:
		if row, ok := m.rowAt(m.cursor); ok {
			m.activeKey = int64(row.ID)
			chatID, title := m.chatID, TopicTitle(row)
			return m, func() tea.Msg { return OpenTopicMsg{ChatID: chatID, TopicID: row.ID, Title: title} }
		}
	case keys.ActionCancel:
		chatID := m.chatID
		return m, func() tea.Msg { return LeaveForumMsg{ChatID: chatID} }
	}
	return m, nil
}

func (m *TopicListModel) View() string {
	if m.total == 0 {
		return theme.S().Body.Render("No topics yet")
	}
	start, end := m.visibleRange()
	// Subtract 1 for outer container safety and 4 for the selection + marker prefix.
	const prefixW = 4
	inner := max(max(m.width, 1)-1-prefixW, 1)

	activeIdx := m.ActiveIdx()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row, ok := m.rowAt(i)
		if !ok {
			lines = append(lines, "")
			continue
		}
		base := rowBase(i == m.cursor && m.focused, i == activeIdx)
		lead := "  "
		if i == activeIdx {
			lead = "▶ "
		}
		marker := "#"
		titleBase := base
		if row.ID == domain.AllTopicsID {
			marker = "*"
		} else if row.Closed {
			marker = "-"
			titleBase = base.Foreground(theme.T().TextMuted)
		}
		prefix := base.Render(lead) + titleBase.Render(marker) + base.Render(" ")
		badge := topicIndicators(row, base)
		id := int64(row.ID)
		lines = append(lines, prefix+rowContent(TopicTitle(row), badge, inner, base, func(t string) string {
			return styleHighlighted(&m.rowList, i, id, t, titleBase, theme.T().HighlightBaseChat)
		}))
	}
	return strings.Join(lines, "\n")
}

// topicIndicators is a topic row's status column: the same tokens, in the same
// order, as a chat row's.
func topicIndicators(r project.TopicRow, base lipgloss.Style) string {
	return rowIndicators(project.ChatRow{
		Unread:    r.Unread,
		Mentions:  r.Mentions,
		Reactions: r.Reactions,
		Muted:     r.Muted,
	}, base)
}
