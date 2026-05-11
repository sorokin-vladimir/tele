package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var (
	outMsgStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	inMsgStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	timeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// MessageList renders a virtual viewport of messages (newest at bottom).
type MessageList struct {
	messages   []store.Message
	viewStart  int
	viewHeight int
	viewWidth  int
	hasMore    bool
}

func NewMessageList(height, width int) *MessageList {
	return &MessageList{viewHeight: height, viewWidth: width}
}

func (ml *MessageList) SetSize(width, height int) {
	ml.viewWidth = width
	ml.viewHeight = height
}

func (ml *MessageList) SetMessages(msgs []store.Message) {
	ml.messages = msgs
	if len(msgs) > ml.viewHeight {
		ml.viewStart = len(msgs) - ml.viewHeight
	} else {
		ml.viewStart = 0
	}
}

func (ml *MessageList) Count() int        { return len(ml.messages) }
func (ml *MessageList) ViewStart() int    { return ml.viewStart }
func (ml *MessageList) SetHasMore(v bool) { ml.hasMore = v }

func (ml *MessageList) ScrollUp() {
	if ml.viewStart > 0 {
		ml.viewStart--
	}
}

func (ml *MessageList) ScrollDown() {
	max := len(ml.messages) - ml.viewHeight
	if max < 0 {
		max = 0
	}
	if ml.viewStart < max {
		ml.viewStart++
	}
}

func (ml *MessageList) View() string {
	if len(ml.messages) == 0 {
		return strings.Repeat("\n", ml.viewHeight-1)
	}
	end := ml.viewStart + ml.viewHeight
	if end > len(ml.messages) {
		end = len(ml.messages)
	}
	visible := ml.messages[ml.viewStart:end]

	lines := make([]string, 0, ml.viewHeight)
	for _, msg := range visible {
		ts := timeStyle.Render(msg.Date.Format("15:04"))
		text := fmt.Sprintf("%s %s", ts, msg.Text)
		if msg.IsOut {
			lines = append(lines, outMsgStyle.Width(ml.viewWidth).Render(text))
		} else {
			lines = append(lines, inMsgStyle.Width(ml.viewWidth).Render(text))
		}
	}
	for len(lines) < ml.viewHeight {
		lines = append(lines, strings.Repeat(" ", ml.viewWidth))
	}
	return strings.Join(lines, "\n")
}
