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
	viewStart  int // index of first (possibly partial) visible message
	lineOffset int // lines of messages[viewStart] to skip from the top
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
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
}

func (ml *MessageList) Count() int        { return len(ml.messages) }
func (ml *MessageList) ViewStart() int    { return ml.viewStart }
func (ml *MessageList) LineOffset() int   { return ml.lineOffset }
func (ml *MessageList) AtTop() bool       { return ml.viewStart == 0 && ml.lineOffset == 0 }
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

// VisibleReadMaxID returns the highest message ID that is "sufficiently visible" to count
// as read: either more than half its lines are in the viewport, or it fills the entire
// viewport (so more than half is impossible to show at once). Returns 0 if none qualify.
func (ml *MessageList) VisibleReadMaxID() int {
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 || len(ml.messages) == 0 {
		return 0
	}
	maxID := 0
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.messages) && linesUsed < ml.viewHeight; i++ {
		msg := ml.messages[i]
		h := ml.msgHeight(msg)
		skipped := 0
		if i == ml.viewStart {
			skipped = ml.lineOffset
		}
		visibleLines := h - skipped
		remaining := ml.viewHeight - linesUsed
		if visibleLines > remaining {
			visibleLines = remaining
		}
		if visibleLines > 0 && (visibleLines*2 > h || h >= ml.viewHeight) {
			if msg.ID > maxID {
				maxID = msg.ID
			}
		}
		linesUsed += visibleLines
	}
	return maxID
}

// ScrollToFirstUnread positions the viewport at the first message with ID > readMaxID.
// If the remaining messages don't fill the viewport, older messages are pulled in to
// fill the space (same as positionAtBottom), keeping the first unread visible.
// Returns false if all messages are already read (nothing to jump to).
func (ml *MessageList) ScrollToFirstUnread(readMaxID int) bool {
	for i, msg := range ml.messages {
		if msg.ID > readMaxID {
			ml.viewStart = i
			ml.lineOffset = 0
			lines := 0
			for j := i; j < len(ml.messages); j++ {
				lines += ml.msgHeight(ml.messages[j])
			}
			if lines < ml.viewHeight {
				ml.viewStart, ml.lineOffset = ml.positionAtBottom()
			}
			return true
		}
	}
	return false
}

// ScrollUp moves the viewport one line toward older messages.
// When crossing a message boundary, small messages (h <= viewHeight) are entered at
// lineOffset=h-2 so at least content+bottom are visible (never bottom-border-only).
// Large messages are entered at their bottom portion (lineOffset=h-viewHeight).
func (ml *MessageList) ScrollUp() {
	if ml.lineOffset > 0 {
		ml.lineOffset--
		return
	}
	if ml.viewStart > 0 {
		ml.viewStart--
		h := ml.msgHeight(ml.messages[ml.viewStart])
		if h > ml.viewHeight {
			ml.lineOffset = h - ml.viewHeight
		} else {
			// Enter showing content+bottom border; lineOffset=h-1 (bottom-only) is skipped.
			ml.lineOffset = h - 2
		}
	}
}

// ScrollDown moves the viewport one line toward newer messages.
// Scrolls line-by-line but skips lineOffset=h-1 (bottom-border-only frame).
// The at-bottom check (positionAtBottom) is the primary stop condition.
func (ml *MessageList) ScrollDown() {
	botIdx, botOff := ml.positionAtBottom()
	if ml.viewStart > botIdx || (ml.viewStart == botIdx && ml.lineOffset >= botOff) {
		return
	}
	h := ml.msgHeight(ml.messages[ml.viewStart])
	if ml.lineOffset+1 < h-1 {
		ml.lineOffset++
		return
	}
	if ml.viewStart+1 < len(ml.messages) {
		ml.viewStart++
		ml.lineOffset = 0
	}
}

// positionAtBottom returns (msgIdx, lineOffset) for the viewport bottom position.
// lineOffset > 0 means the first visible message is shown from its bottom portion,
// filling the space that would otherwise be empty above the last full messages.
func (ml *MessageList) positionAtBottom() (int, int) {
	lineCount := 0
	for i := len(ml.messages) - 1; i >= 0; i-- {
		h := ml.msgHeight(ml.messages[i])
		if lineCount+h >= ml.viewHeight {
			// Adding this message meets or exceeds the viewport.
			// Show it from the offset that makes total lines == viewHeight.
			overflow := lineCount + h - ml.viewHeight
			return i, overflow
		}
		lineCount += h
	}
	return 0, 0
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
		}
		var titleStr string
		if senderStyled != "" {
			titleStr = " " + senderStyled + " "
		}
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
	reachedEnd := true
	for i := ml.viewStart; i < len(ml.messages); i++ {
		msgLines := ml.renderMessage(ml.messages[i])
		if i == ml.viewStart && ml.lineOffset > 0 {
			if ml.lineOffset < len(msgLines) {
				msgLines = msgLines[ml.lineOffset:]
			} else {
				msgLines = nil
			}
		}
		allLines = append(allLines, msgLines...)
		if len(allLines) >= ml.viewHeight {
			reachedEnd = (i == len(ml.messages)-1)
			break
		}
	}

	// Pad to viewHeight.
	// If we rendered all the way to the last message, anchor content to the bottom
	// (chat-like: newest messages visible). Otherwise we're in the middle of history,
	// so anchor to the top so the jump target is immediately visible.
	if len(allLines) < ml.viewHeight {
		padding := make([]string, ml.viewHeight-len(allLines))
		if reachedEnd {
			allLines = append(padding, allLines...)
		} else {
			allLines = append(allLines, padding...)
		}
	}

	// Trim to viewport height.
	// At the natural bottom of the chat, trim from the top so the newest content
	// stays visible. When scrolling through history, trim from the bottom so the
	// current scroll position is preserved.
	if len(allLines) > ml.viewHeight {
		botIdx, botOff := ml.positionAtBottom()
		atNaturalBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
		if reachedEnd && atNaturalBottom {
			allLines = allLines[len(allLines)-ml.viewHeight:]
		} else {
			allLines = allLines[:ml.viewHeight]
		}
	}

	return strings.Join(allLines, "\n")
}
