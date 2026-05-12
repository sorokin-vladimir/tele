package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var (
	inBubbleBg  = lipgloss.AdaptiveColor{Dark: "237", Light: "252"}
	outBubbleBg = lipgloss.AdaptiveColor{Dark: "17", Light: "153"}

	inNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	outNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
)

// MessageList renders a virtual viewport of messages (newest at bottom).
type MessageList struct {
	messages   []store.Message
	viewStart  int
	viewHeight int
	viewWidth  int
	isGroup    bool
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
	ml.viewStart = ml.positionAtBottom()
}

func (ml *MessageList) Count() int        { return len(ml.messages) }
func (ml *MessageList) ViewStart() int    { return ml.viewStart }
func (ml *MessageList) AtTop() bool       { return ml.viewStart == 0 }
func (ml *MessageList) SetIsGroup(v bool) { ml.isGroup = v }

// PrependMessages inserts older messages at the front and shifts viewStart so
// that the currently-visible messages stay on screen.
func (ml *MessageList) PrependMessages(older []store.Message) {
	if len(older) == 0 {
		return
	}
	ml.messages = append(older, ml.messages...)
	ml.viewStart += len(older)
}

func (ml *MessageList) OldestID() int {
	if len(ml.messages) == 0 {
		return 0
	}
	return ml.messages[0].ID
}

func (ml *MessageList) ScrollUp() {
	if ml.viewStart > 0 {
		ml.viewStart--
	}
}

func (ml *MessageList) ScrollDown() {
	if bottom := ml.positionAtBottom(); ml.viewStart < bottom {
		ml.viewStart++
	}
}

// positionAtBottom returns the message index such that messages from that
// index to the end fill at most viewHeight lines (newest messages visible).
func (ml *MessageList) positionAtBottom() int {
	lineCount := 0
	for i := len(ml.messages) - 1; i >= 0; i-- {
		h := ml.msgHeight(ml.messages[i])
		if lineCount+h > ml.viewHeight {
			return i + 1
		}
		lineCount += h
	}
	return 0
}

// msgHeight estimates the rendered line count for a single message:
// 1 header + wrapped text lines + 1 blank separator.
func (ml *MessageList) msgHeight(msg store.Message) int {
	if ml.viewWidth <= 0 {
		return 3
	}
	msgWidth := ml.viewWidth * 3 / 4
	if msgWidth < 10 {
		msgWidth = 10
	}
	h := 1 // header
	for _, part := range strings.Split(msg.Text, "\n") {
		r := []rune(part)
		if len(r) == 0 {
			h++
		} else {
			h += (len(r) + msgWidth - 1) / msgWidth
		}
	}
	return h + 1 // +1 blank separator
}

// renderMessage returns the display lines for a single message.
func (ml *MessageList) renderMessage(msg store.Message) []string {
	if ml.viewWidth <= 0 {
		return []string{""}
	}
	msgWidth := ml.viewWidth * 3 / 4
	if msgWidth < 10 {
		msgWidth = 10
	}
	leftPad := ml.viewWidth - msgWidth

	var blockBase lipgloss.Style
	var tsStyle lipgloss.Style
	var labelStyle lipgloss.Style
	if msg.IsOut {
		blockBase = lipgloss.NewStyle().Width(msgWidth).Background(outBubbleBg)
		tsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(outBubbleBg)
		labelStyle = outNameStyle.Background(outBubbleBg)
	} else {
		blockBase = lipgloss.NewStyle().Width(msgWidth).Background(inBubbleBg)
		tsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(inBubbleBg)
		labelStyle = inNameStyle.Background(inBubbleBg)
	}

	ts := tsStyle.Render(msg.Date.Format("15:04"))

	var lines []string

	// Header: label + time. In group chats show sender name / "You"; in DMs use > / <.
	if msg.IsOut {
		var label string
		if ml.isGroup {
			label = labelStyle.Render("You")
		} else {
			label = labelStyle.Render("<")
		}
		header := ts + " " + label
		lines = append(lines, strings.Repeat(" ", leftPad)+blockBase.Align(lipgloss.Right).Render(header))
	} else {
		var label string
		if ml.isGroup {
			name := msg.SenderName
			if name == "" {
				name = "?"
			}
			label = labelStyle.Render(name)
		} else {
			label = labelStyle.Render(">")
		}
		header := label + " " + ts
		lines = append(lines, blockBase.Render(header))
	}

	// Body: preserve newlines, word-wrap each logical line to msgWidth
	rendered := RenderEntities(msg.Text, msg.Entities)
	var bodyStyle lipgloss.Style
	if msg.IsOut {
		bodyStyle = blockBase.Align(lipgloss.Right)
	} else {
		bodyStyle = blockBase
	}
	for _, part := range strings.Split(rendered, "\n") {
		wrapped := bodyStyle.Render(part)
		for _, wl := range strings.Split(wrapped, "\n") {
			if msg.IsOut {
				lines = append(lines, strings.Repeat(" ", leftPad)+wl)
			} else {
				lines = append(lines, wl)
			}
		}
	}

	// Blank separator between messages (no background)
	lines = append(lines, "")
	return lines
}

func (ml *MessageList) View() string {
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 {
		return ""
	}
	if len(ml.messages) == 0 {
		return strings.Repeat("\n", ml.viewHeight-1)
	}

	var allLines []string
	for i := ml.viewStart; i < len(ml.messages); i++ {
		allLines = append(allLines, ml.renderMessage(ml.messages[i])...)
	}

	// Fewer lines than viewport: pad at top so content anchors to bottom
	if len(allLines) < ml.viewHeight {
		padding := make([]string, ml.viewHeight-len(allLines))
		allLines = append(padding, allLines...)
	}

	// Trim to viewport height
	if len(allLines) > ml.viewHeight {
		allLines = allLines[:ml.viewHeight]
	}

	return strings.Join(allLines, "\n")
}
