package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var (
	inBubbleBg  = compat.AdaptiveColor{Dark: lipgloss.Color("237"), Light: lipgloss.Color("252")}
	outBubbleBg = compat.AdaptiveColor{Dark: lipgloss.Color("17"), Light: lipgloss.Color("153")}

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
// 2 border lines + 1 header + wrapped body lines + 1 blank separator.
func (ml *MessageList) msgHeight(msg store.Message) int {
	if ml.viewWidth <= 0 {
		return 5
	}
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	// Width() in lipgloss v2 is the total outer width; border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}
	h := 1 // header
	for _, part := range strings.Split(msg.Text, "\n") {
		r := []rune(part)
		if len(r) == 0 {
			h++
		} else {
			h += (len(r) + maxContentW - 1) / maxContentW
		}
	}
	return h + 2 // +2 border lines (top+bottom)
}

// renderMessage returns the display lines for a single message bubble.
func (ml *MessageList) renderMessage(msg store.Message) []string {
	if ml.viewWidth <= 0 {
		return []string{""}
	}
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	// Width() in lipgloss v2 is the total outer width; border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}

	var bg color.Color
	var borderFg color.Color
	if msg.IsOut {
		bg = outBubbleBg
		borderFg = lipgloss.Color("25")
	} else {
		bg = inBubbleBg
		borderFg = lipgloss.Color("238")
	}

	tsStyled := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(bg).
		Render(msg.Date.Format("15:04"))

	// Header: for outgoing messages the header is right-aligned inside the bubble;
	// for incoming it is left-aligned.
	var headerStyled string
	if msg.IsOut {
		var label string
		if ml.isGroup {
			label = outNameStyle.Background(bg).Render("You")
		} else {
			label = outNameStyle.Background(bg).Render("<")
		}
		headerStyled = label + "  " + tsStyled
	} else {
		var name string
		if ml.isGroup {
			name = msg.SenderName
			if name == "" {
				name = "?"
			}
		} else {
			name = ">"
		}
		label := inNameStyle.Background(bg).Render(name)
		headerStyled = label + "  " + tsStyled
	}

	// Measure the natural content width so the bubble hugs its content.
	actualW := lipgloss.Width(headerStyled)
	if msg.Text != "" {
		measureStyle := lipgloss.NewStyle().Width(maxContentW)
		for _, part := range strings.Split(msg.Text, "\n") {
			if part == "" {
				continue
			}
			for _, wl := range strings.Split(measureStyle.Render(part), "\n") {
				if w := lipgloss.Width(strings.TrimRight(wl, " ")); w > actualW {
					actualW = w
				}
			}
		}
		if actualW > maxContentW {
			actualW = maxContentW
		}
	}
	if actualW < 1 {
		actualW = 1
	}

	// Right-align the header line for outgoing messages by padding it on the left.
	if msg.IsOut {
		if pad := actualW - lipgloss.Width(headerStyled); pad > 0 {
			headerStyled = strings.Repeat(" ", pad) + headerStyled
		}
	}

	// Body with entity styling.
	rendered := RenderEntities(msg.Text, msg.Entities)
	var contentParts []string
	contentParts = append(contentParts, headerStyled)
	if msg.Text != "" {
		contentParts = append(contentParts, strings.Split(rendered, "\n")...)
	}
	content := strings.Join(contentParts, "\n")

	// actualW+4 = total outer width: content + padding(1+1) + border(1+1).
	bubble := lipgloss.NewStyle().
		Width(actualW+4).
		Background(bg).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderFg).
		Padding(0, 1).
		Render(content)

	lines := strings.Split(bubble, "\n")

	// Outgoing bubbles are right-aligned; incoming stay at the left margin.
	if msg.IsOut {
		bubbleW := lipgloss.Width(lines[0])
		leftPad := ml.viewWidth - bubbleW
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)
		for i := range lines {
			lines[i] = pad + lines[i]
		}
	}

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
