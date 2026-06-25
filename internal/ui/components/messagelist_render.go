package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
)

const indicatorChar = "┃"

// bubbleMetrics holds the finalized geometry and border-row content for a
// message bubble, computed once by measureBubble and consumed by the border and
// content rendering steps.
type bubbleMetrics struct {
	actualW  int // content width (inside the padding)
	innerW   int // actualW + 2 padding columns
	b        lipgloss.Border
	bs       lipgloss.Style
	tsStr    string // timestamp + status, right side of the bottom border
	tsW      int
	reactStr string // reactions, left side of the bottom border
	reactW   int
}

// renderMessage returns the display lines for a single message bubble.
// selected: when true, draws the selection indicator bar beside the bubble.
func (ml *MessageList) renderMessage(msg store.Message, selected bool) []string {
	if ml.viewWidth <= 0 {
		return []string{""}
	}
	if ml.isBareMedia(msg) {
		return ml.renderBareMedia(msg, selected)
	}

	m := ml.measureBubble(msg)
	top, bottom := ml.bubbleBorders(msg, m)
	sideLines := ml.bubbleContentLines(msg, m)

	allLines := make([]string, 0, len(sideLines)+2)
	allLines = append(allLines, top)
	allLines = append(allLines, sideLines...)
	allLines = append(allLines, bottom)

	return ml.alignBubbleLines(allLines, msg.IsOut, selected)
}

// measureBubble computes the finalized bubble geometry and border-row content
// (timestamp, reactions) for a message, widening as needed for the text, media
// placeholder/art, forward and reply blocks, and the sender-name title.
func (ml *MessageList) measureBubble(msg store.Message) bubbleMetrics {
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	// border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}

	borderFg := ml.bubbleBorderFg(msg)
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

	// Ensure photo content width is reflected in bubble sizing. Photos pre-size
	// the bubble even before the image loads; video thumbnails widen it only
	// once the thumbnail is available (the text placeholder is narrow).
	widenToPhotoCols := msg.Photo != nil
	if !widenToPhotoCols {
		if id, ok := ml.PreviewImageID(msg); ok {
			if _, has := ml.cachedImage(id); has {
				widenToPhotoCols = true
			}
		}
	}
	if widenToPhotoCols {
		photoCols := ml.photoContentCols()
		// Once bytes are known, the rendered width may be narrower than the full
		// budget (480px / viewport caps), so size the bubble to the actual image.
		if id, ok := ml.PreviewImageID(msg); ok {
			if img, has := ml.cachedImage(id); has {
				bb := img.Bounds()
				photoCols, _ = ml.mediaBox(msg, bb.Dx(), bb.Dy())
			}
		}
		if photoCols > actualW {
			actualW = photoCols
		}
	}

	// Ensure bubble is wide enough for a media placeholder label.
	// Measured with lipgloss.Width so wide emoji match placeholderLine's padding.
	if msg.Media != nil {
		if w := lipgloss.Width(placeholderFor(msg.Media)); w > actualW {
			actualW = w
		}
	}

	// Optimistic outgoing media bubble: reserve room for the file label and a
	// reasonable progress-bar width.
	if msg.LocalMedia != nil {
		if w := lipgloss.Width(localMediaLabel(msg.LocalMedia)); w > actualW {
			actualW = w
		}
		const minUploadBarW = 24
		if actualW < minUploadBarW {
			actualW = minUploadBarW
		}
		if actualW > maxContentW {
			actualW = maxContentW
		}
	}

	// Ensure bubble is wide enough for the forwarded-message header block.
	if msg.Forward != nil {
		if minW := measureForwardBlock(msg.Forward.From, maxContentW); actualW < minW {
			actualW = minW
		}
	}

	// Ensure bubble is wide enough for the reply preview block.
	if msg.ReplyToMsgID != 0 {
		orig := ml.findMessage(msg.ReplyToMsgID)
		var minW int
		if orig != nil {
			minW = measurePreviewBlock(replyName(orig), firstLine(orig.Text), maxContentW)
		} else {
			w := lipgloss.Width(quoteGlyph + quoteStyle.Render("Original not available"))
			if w > maxContentW {
				w = maxContentW
			}
			minW = w
		}
		if actualW < minW {
			actualW = minW
		}
	}

	// innerW = actualW (content) + 2 (padding 1 each side).
	innerW := actualW + 2

	// Timestamp + optional status indicator in bottom border.
	var statusStr string
	if msg.IsOut {
		if msg.ID > 0 && msg.ID <= ml.outboxReadMaxID {
			statusStr = " " + readStyle.Render("✓✓")
		} else if msg.ID > 0 {
			statusStr = " " + sentStyle.Render("✓")
		}
	}
	editMark := ""
	if msg.EditDate != nil {
		editMark = tsStyle.Render("edited") + " · "
	}
	tsStr := " " + editMark + tsStyle.Render(msg.Date.Format("15:04")) + statusStr + " "
	tsW := lipgloss.Width(tsStr)
	if innerW < tsW {
		innerW = tsW
		actualW = innerW - 2
	}
	reactStr := buildReactStr(msg.Reactions)
	reactW := lipgloss.Width(reactStr)
	if innerW < reactW+tsW+1 {
		innerW = reactW + tsW + 1
		actualW = innerW - 2
	}

	// Ensure bubble is wide enough for the sender name in the top border.
	// rightFill = innerW - titleW - 1 must be >= 0, so innerW >= titleW + 1.
	if !msg.IsOut && ml.isGroup {
		name := msg.SenderName
		if name == "" {
			name = "?"
		}
		titleW := lipgloss.Width(" " + ml.senderNameStyle(msg.SenderID).Render(name) + " ")
		if innerW < titleW+1 {
			innerW = titleW + 1
			actualW = innerW - 2
		}
	}

	return bubbleMetrics{
		actualW:  actualW,
		innerW:   innerW,
		b:        b,
		bs:       bs,
		tsStr:    tsStr,
		tsW:      tsW,
		reactStr: reactStr,
		reactW:   reactW,
	}
}

// bubbleBorders builds the top and bottom border rows of a message bubble. The
// top border carries the sender name (incoming group messages); the bottom
// border carries reactions on the left and the timestamp on the right.
func (ml *MessageList) bubbleBorders(msg store.Message, m bubbleMetrics) (top, bottom string) {
	b, bs := m.b, m.bs

	// Top border: sender/indicator left-aligned for incoming; plain for outgoing.
	if !msg.IsOut {
		var senderStyled string
		if ml.isGroup {
			name := msg.SenderName
			if name == "" {
				name = "?"
			}
			senderStyled = ml.senderNameStyle(msg.SenderID).Render(name)
		}
		var titleStr string
		if senderStyled != "" {
			titleStr = " " + senderStyled + " "
		}
		titleW := lipgloss.Width(titleStr)
		rightFill := m.innerW - titleW - 1 // 1 fill char on the left
		if rightFill < 0 {
			rightFill = 0
		}
		top = bs.Render(b.TopLeft+b.Top) + titleStr + bs.Render(strings.Repeat(b.Top, rightFill)+b.TopRight)
	} else {
		top = bs.Render(b.TopLeft + strings.Repeat(b.Top, m.innerW) + b.TopRight)
	}

	// Bottom border: reactions left, timestamp right.
	fillW := m.innerW - m.reactW - m.tsW
	if fillW < 0 {
		fillW = 0
	}
	bottom = bs.Render(b.BottomLeft) + m.reactStr + bs.Render(strings.Repeat(b.Bottom, fillW)) + m.tsStr + bs.Render(b.BottomRight)
	return top, bottom
}

// bubbleContentLines builds the interior rows of a message bubble: the forward
// header (if any), the reply quote block (if a reply), media art or its
// placeholder (if any), then the wrapped message text.
func (ml *MessageList) bubbleContentLines(msg store.Message, m bubbleMetrics) []string {
	actualW, innerW, b, bs := m.actualW, m.innerW, m.b, m.bs

	// Content lines: forward header (if any), reply quote block (if reply),
	// photo art (if any), then text.
	var sideLines []string

	if msg.Forward != nil {
		sideLines = append(sideLines, renderForwardLines(msg.Forward.From, actualW, bs)...)
		// Separate the forward header from any following content with a blank line.
		if msg.ReplyToMsgID != 0 || msg.Text != "" || msg.Media != nil {
			sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
		}
	}

	if msg.ReplyToMsgID != 0 {
		orig := ml.findMessage(msg.ReplyToMsgID)
		var origSenderID int64
		var name, snippet string
		if orig != nil {
			origSenderID = orig.SenderID
			name = replyName(orig)
			snippet = firstLine(orig.Text)
		}
		sideLines = append(sideLines, ml.renderPreviewLines(origSenderID, name, snippet, actualW, bs)...)
		if msg.Text != "" || msg.Media != nil {
			sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
		}
	}

	if msg.LocalMedia != nil {
		sideLines = append(sideLines, labelLine(localMediaLabel(msg.LocalMedia), actualW, b, bs))
		sideLines = append(sideLines, labelLine(uploadStatusLine(msg.LocalMedia, actualW), actualW, b, bs))
		if msg.Text != "" {
			sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
		}
	}

	if msg.Media != nil {
		var artLines []string
		hasBytes, footprint := false, 0
		if id, ok := ml.PreviewImageID(msg); ok {
			if img, has := ml.cachedImage(id); has {
				hasBytes = true
				bb := img.Bounds()
				cols, rows := ml.mediaBox(msg, bb.Dx(), bb.Dy())
				footprint = rows
				artLines = ml.renderer.Render(id, img, cols)
			}
		}
		blankRow := bs.Render(b.Left) + strings.Repeat(" ", innerW) + bs.Render(b.Right)
		switch {
		case artLines != nil:
			for _, al := range artLines {
				lw := lipgloss.Width(al)
				if lw < actualW {
					al += strings.Repeat(" ", actualW-lw)
				}
				sideLines = append(sideLines, bs.Render(b.Left)+" "+al+" "+bs.Render(b.Right))
			}
			if overlay := ml.overlayLabelFor(msg); overlay != "" {
				sideLines = append(sideLines, labelLine(overlay, actualW, b, bs))
			}
		case hasBytes:
			// Bytes are known but the Kitty placement is not transmitted yet. Fill
			// the full reserved footprint with a placeholder box (label on the first
			// row) so the rendered height matches msgHeight — the image swaps in at
			// the same size with no scroll jump or hidden tail (issue #115).
			for i := 0; i < footprint; i++ {
				if i == 0 {
					sideLines = append(sideLines, placeholderLine(msg.Media, actualW, b, bs))
				} else {
					sideLines = append(sideLines, blankRow)
				}
			}
			if overlay := ml.overlayLabelFor(msg); overlay != "" {
				sideLines = append(sideLines, labelLine(overlay, actualW, b, bs))
			}
		case msg.Media.Kind == store.MediaVoice && msg.Document != nil &&
			msg.Document.ID == ml.playingVoiceID:
			// Voice currently playing: waveform with playhead + live position.
			label := voicePlayingLabel(msg.Media, ml.voiceProgress, ml.voicePosition)
			sideLines = append(sideLines, labelLine(label, actualW, b, bs))
		default:
			sideLines = append(sideLines, placeholderLine(msg.Media, actualW, b, bs))
		}
		if msg.Text != "" {
			sideLines = append(sideLines, blankRow)
		}
	}

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
	} else if len(sideLines) == 0 {
		sideLines = []string{bs.Render(b.Left) + strings.Repeat(" ", innerW) + bs.Render(b.Right)}
	}

	return sideLines
}

// alignBubbleLines right-aligns outgoing bubbles (incoming stay at the left
// margin) and draws the selection indicator bar beside the bubble on every
// content line.
func (ml *MessageList) alignBubbleLines(allLines []string, isOut, selected bool) []string {
	// Outgoing bubbles are right-aligned; incoming stay at the left margin.
	if isOut {
		bubbleW := lipgloss.Width(allLines[0])
		leftPad := ml.viewWidth - bubbleW
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)
		for i := range allLines {
			allLines[i] = pad + allLines[i]
		}
		// Draw indicator bar on every content line (all except top and bottom border).
		// First leftPad bytes are ASCII spaces, so byte-slicing is safe.
		if selected && ml.showIndicator && len(allLines) > 2 && leftPad >= 2 {
			bar := " " + indicatorStyle.Render(indicatorChar)
			for i := 1; i < len(allLines)-1; i++ {
				allLines[i] = allLines[i][:leftPad-2] + bar + allLines[i][leftPad:]
			}
		}
	} else {
		// Draw indicator bar on every content line to the right of the bubble.
		if selected && ml.showIndicator && len(allLines) > 2 {
			bubbleW := lipgloss.Width(allLines[0])
			available := ml.viewWidth - bubbleW
			if available >= 2 {
				bar := " " + indicatorStyle.Render(indicatorChar)
				for i := 1; i < len(allLines)-1; i++ {
					allLines[i] = allLines[i] + bar
				}
			}
		}
	}

	return allLines
}

// renderBareMedia draws a sticker or round video note without the message
// bubble: the image art, a sender-name line above it in groups, an optional
// play/duration overlay (video notes), and a plain timestamp line below
// (reactions left, time + read status right). Caller must have verified
// isBareMedia.
func (ml *MessageList) renderBareMedia(msg store.Message, selected bool) []string {
	id, _ := ml.PreviewImageID(msg)
	img, _ := ml.cachedImage(id)
	bb := img.Bounds()
	cols, rows := ml.mediaBox(msg, bb.Dx(), bb.Dy())
	artLines := ml.renderer.Render(id, img, cols)

	// Timestamp + read status, shown on a plain line under the sticker.
	var statusStr string
	if msg.IsOut {
		if msg.ID > 0 && msg.ID <= ml.outboxReadMaxID {
			statusStr = " " + readStyle.Render("✓✓")
		} else if msg.ID > 0 {
			statusStr = " " + sentStyle.Render("✓")
		}
	}
	editMark := ""
	if msg.EditDate != nil {
		editMark = tsStyle.Render("edited") + " · "
	}
	tsStr := editMark + tsStyle.Render(msg.Date.Format("15:04")) + statusStr
	reactStr := strings.TrimSpace(buildReactStr(msg.Reactions))

	// Block width: widest of the art, the meta line, and (in groups) the name.
	blockW := cols
	metaW := lipgloss.Width(tsStr)
	if reactStr != "" {
		metaW += lipgloss.Width(reactStr) + 1
	}
	if metaW > blockW {
		blockW = metaW
	}
	nameStr := ""
	if !msg.IsOut && ml.isGroup {
		name := msg.SenderName
		if name == "" {
			name = "?"
		}
		nameStr = ml.senderNameStyle(msg.SenderID).Render(name)
		if w := lipgloss.Width(nameStr); w > blockW {
			blockW = w
		}
	}

	pad := func(s string) string {
		if w := lipgloss.Width(s); w < blockW {
			return s + strings.Repeat(" ", blockW-w)
		}
		return s
	}

	lines := make([]string, 0, rows+2)
	if nameStr != "" {
		lines = append(lines, pad(nameStr))
	}
	if artLines != nil {
		for _, al := range artLines {
			lines = append(lines, pad(al))
		}
	} else {
		// Placement not transmitted yet: reserve the art rows so the height
		// matches msgHeight; the image swaps in on the next render.
		for i := 0; i < rows; i++ {
			lines = append(lines, strings.Repeat(" ", blockW))
		}
	}
	if overlay := ml.overlayLabelFor(msg); overlay != "" {
		lines = append(lines, pad(overlay)) // ▶ duration / GIF badge under the thumbnail
	}
	fill := blockW - lipgloss.Width(reactStr) - lipgloss.Width(tsStr)
	if fill < 0 {
		fill = 0
	}
	lines = append(lines, pad(reactStr+strings.Repeat(" ", fill)+tsStr))

	return ml.alignBareLines(lines, blockW, selected, msg.IsOut)
}

// alignBareLines right-aligns outgoing borderless media blocks (left margin for
// incoming) and draws the selection indicator bar beside the block, mirroring
// the bubble path but without borders.
func (ml *MessageList) alignBareLines(lines []string, blockW int, selected, isOut bool) []string {
	if isOut {
		leftPad := ml.viewWidth - blockW
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)
		for i := range lines {
			lines[i] = pad + lines[i]
		}
		// leftPad bytes are ASCII spaces, so byte-slicing is safe.
		if selected && ml.showIndicator && leftPad >= 2 {
			bar := " " + indicatorStyle.Render(indicatorChar)
			for i := range lines {
				lines[i] = lines[i][:leftPad-2] + bar + lines[i][leftPad:]
			}
		}
		return lines
	}
	if selected && ml.showIndicator {
		if available := ml.viewWidth - blockW; available >= 2 {
			bar := " " + indicatorStyle.Render(indicatorChar)
			for i := range lines {
				lines[i] = lines[i] + bar
			}
		}
	}
	return lines
}

func (ml *MessageList) renderSeparator(label string) []string {
	labelW := lipgloss.Width(label)
	fill := (ml.viewWidth - labelW - 2) / 2
	if fill < 0 {
		fill = 0
	}
	rightFill := ml.viewWidth - fill - 1 - labelW - 1
	if rightFill < 0 {
		rightFill = 0
	}
	line := sepStyle.Render(strings.Repeat("─", fill)) + " " + label + " " + sepStyle.Render(strings.Repeat("─", rightFill))
	return []string{"", line, ""}
}

func (ml *MessageList) renderUnreadSeparator() []string {
	const label = "New Messages"
	labelW := lipgloss.Width(label)
	fill := (ml.viewWidth - labelW - 2) / 2
	if fill < 0 {
		fill = 0
	}
	rightFill := ml.viewWidth - fill - 1 - labelW - 1
	if rightFill < 0 {
		rightFill = 0
	}
	line := unreadSepStyle.Render(strings.Repeat("─", fill)) + " " + unreadSepStyle.Render(label) + " " + unreadSepStyle.Render(strings.Repeat("─", rightFill))
	return []string{"", line, ""}
}

func (ml *MessageList) renderItem(i int, selected bool) []string {
	item := ml.items[i]
	if item.kind == itemDateSeparator {
		return ml.renderSeparator(item.label)
	}
	if item.kind == itemUnreadSeparator {
		return ml.renderUnreadSeparator()
	}
	return ml.renderMessage(item.msg, selected)
}

func (ml *MessageList) View() string {
	ml.selRectOK = false
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 {
		return ""
	}
	if len(ml.items) == 0 {
		return strings.Repeat("\n", ml.viewHeight-1)
	}

	selectedID := ml.computeSelectedMsgID()

	var allLines []string
	reachedEnd := true
	selTopRaw, selHeight, selLeft, selWidth := 0, 0, 0, 0
	for i := ml.viewStart; i < len(ml.items); i++ {
		var selected bool
		if ml.items[i].kind == itemMessage {
			selected = ml.items[i].msg.ID == selectedID
		}
		itemLines := ml.renderItem(i, selected)

		// Measure alignment from the top border line (index 0); it never carries
		// the selection indicator, and every line of a bubble shares the same
		// left padding, so this yields the bubble's left/width reliably.
		var selFirstFull string
		if selected && len(itemLines) > 0 {
			selFirstFull = itemLines[0]
		}

		if i == ml.viewStart && ml.lineOffset > 0 {
			if ml.lineOffset < len(itemLines) {
				itemLines = itemLines[ml.lineOffset:]
			} else {
				itemLines = nil
			}
		}

		if selected {
			selTopRaw = len(allLines)
			selHeight = len(itemLines)
			trimmed := strings.TrimLeft(selFirstFull, " ")
			selLeft = lipgloss.Width(selFirstFull) - lipgloss.Width(trimmed)
			selWidth = lipgloss.Width(trimmed)
			ml.selRectOK = true
		}

		allLines = append(allLines, itemLines...)
		if len(allLines) >= ml.viewHeight {
			reachedEnd = (i == len(ml.items)-1)
			break
		}
	}

	// delta tracks how the pad/trim step below shifts every line's index, so the
	// captured selTopRaw can be mapped to its final viewport row.
	delta := 0

	// Pad to viewHeight.
	// If we rendered all the way to the last message, anchor content to the bottom
	// (chat-like: newest messages visible). Otherwise we're in the middle of history,
	// so anchor to the top so the jump target is immediately visible.
	if len(allLines) < ml.viewHeight {
		padding := make([]string, ml.viewHeight-len(allLines))
		if reachedEnd {
			allLines = append(padding, allLines...)
			delta = len(padding)
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
			cut := len(allLines) - ml.viewHeight
			allLines = allLines[cut:]
			delta = -cut
		} else {
			allLines = allLines[:ml.viewHeight]
		}
	}

	if ml.selRectOK {
		ml.selRect = Rect{Top: selTopRaw + delta, Left: selLeft, Height: selHeight, Width: selWidth}
	}

	return strings.Join(allLines, "\n")
}
