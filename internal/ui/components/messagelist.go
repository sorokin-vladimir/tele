package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var (
	inNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	tsStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
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
// 2 border lines (top with header title + bottom) + wrapped body lines.
func (ml *MessageList) msgHeight(msg store.Message) int {
	if ml.viewWidth <= 0 {
		return 4
	}
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	// border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}
	h := 0
	if msg.Text != "" {
		for _, part := range strings.Split(msg.Text, "\n") {
			r := []rune(part)
			if len(r) == 0 {
				h++
			} else {
				h += (len(r) + maxContentW - 1) / maxContentW
			}
		}
	}
	if h == 0 {
		h = 1 // at least one content line for empty-text messages
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
	// border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}

	var borderFg color.Color
	if msg.IsOut {
		borderFg = lipgloss.Color("25")
	} else {
		borderFg = lipgloss.Color("238")
	}
	b := lipgloss.RoundedBorder()
	bs := lipgloss.NewStyle().Foreground(borderFg)

	// Measure content width from text only.
	actualW := 0
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

	// innerW = actualW (content) + 2 (padding 1 each side).
	innerW := actualW + 2

	// Timestamp in bottom border — ensure it fits.
	tsStr := " " + tsStyle.Render(msg.Date.Format("15:04")) + " "
	tsW := lipgloss.Width(tsStr)
	if innerW < tsW {
		innerW = tsW
		actualW = innerW - 2
	}

	// Top border: sender/indicator left-aligned for incoming; plain for outgoing.
	var top string
	if !msg.IsOut {
		var senderStyled string
		if ml.isGroup {
			name := msg.SenderName
			if name == "" {
				name = "?"
			}
			senderStyled = inNameStyle.Render(name)
		} else {
			senderStyled = inNameStyle.Render(">")
		}
		titleStr := " " + senderStyled + " "
		titleW := lipgloss.Width(titleStr)
		rightFill := innerW - titleW - 1 // 1 fill char on the left
		if rightFill < 0 {
			rightFill = 0
		}
		top = bs.Render(b.TopLeft+b.Top) + titleStr + bs.Render(strings.Repeat(b.Top, rightFill)+b.TopRight)
	} else {
		top = bs.Render(b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight)
	}

	// Bottom border: timestamp right-aligned.
	tsLeftFill := innerW - tsW
	if tsLeftFill < 0 {
		tsLeftFill = 0
	}
	bottom := bs.Render(b.BottomLeft+strings.Repeat(b.Bottom, tsLeftFill)) + tsStr + bs.Render(b.BottomRight)

	// Content lines with word wrapping.
	var sideLines []string
	if msg.Text != "" {
		rendered := RenderEntities(msg.Text, msg.Entities)
		wrapStyle := lipgloss.NewStyle().Width(actualW)
		for _, part := range strings.Split(rendered, "\n") {
			if part == "" {
				sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
				continue
			}
			for _, wl := range strings.Split(wrapStyle.Render(part), "\n") {
				lw := lipgloss.Width(wl)
				if lw < actualW {
					wl += strings.Repeat(" ", actualW-lw)
				}
				sideLines = append(sideLines, bs.Render(b.Left)+" "+wl+" "+bs.Render(b.Right))
			}
		}
	} else {
		sideLines = []string{bs.Render(b.Left) + strings.Repeat(" ", innerW) + bs.Render(b.Right)}
	}

	allLines := make([]string, 0, len(sideLines)+2)
	allLines = append(allLines, top)
	allLines = append(allLines, sideLines...)
	allLines = append(allLines, bottom)

	// Outgoing bubbles are right-aligned; incoming stay at the left margin.
	if msg.IsOut {
		bubbleW := lipgloss.Width(allLines[0])
		leftPad := ml.viewWidth - bubbleW
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)
		for i := range allLines {
			allLines[i] = pad + allLines[i]
		}
	}

	return allLines
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
