package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
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
func (ml *MessageList) renderMessage(msg domain.Message, selected bool) []string {
	return ml.renderBubble(msg, selected, "")
}

// renderBubble draws a message, optionally overriding the delivery indicator in
// the bottom border. The override is how a queued send shows where it has got
// to: it has no message id yet, so the ✓/✓✓ path has nothing to say (#193).
func (ml *MessageList) renderBubble(msg domain.Message, selected bool, statusOverride string) []string {
	if ml.viewWidth <= 0 {
		return []string{""}
	}
	if ml.isBareMedia(msg) {
		return ml.renderBareMedia(msg, selected)
	}

	m := ml.measureBubbleWithStatus(msg, statusOverride)
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
func (ml *MessageList) measureBubble(msg domain.Message) bubbleMetrics {
	return ml.measureBubbleWithStatus(msg, "")
}

func (ml *MessageList) measureBubbleWithStatus(msg domain.Message, statusOverride string) bubbleMetrics {
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
	// A bubble has no fill of its own — its interior is canvas — so the border
	// carries it like any other cell the app draws.
	bs := theme.NewStyle().Foreground(borderFg)

	// Measure content width from text only.
	actualW := 0
	if msg.Text != "" {
		// canvas:ok measurement only — this render is measured and thrown away,
		// so a background would cost work per bubble and reach no cell.
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
			w := lipgloss.Width(quoteGlyph + theme.S().Quote.Render("Original not available"))
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
	statusStr := statusOverride
	if statusStr == "" && msg.IsOut {
		if msg.ID > 0 && msg.ID <= ml.outboxReadMaxID {
			statusStr = theme.Pad(1) + theme.S().TickRead.Render("✓✓")
		} else if msg.ID > 0 {
			statusStr = theme.Pad(1) + theme.S().TickSent.Render("✓")
		}
	}
	editMark := ""
	if msg.EditDate != nil {
		editMark = theme.S().Timestamp.Render("edited") + " · "
	}
	// The spaces framing the stamp sit on the bubble's bottom border, between
	// runs that each end in a reset, so they carry the canvas themselves.
	tsStr := theme.Pad(1) + editMark + theme.S().Timestamp.Render(msg.Date.Format("15:04")) + statusStr + theme.Pad(1)
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
func (ml *MessageList) bubbleBorders(msg domain.Message, m bubbleMetrics) (top, bottom string) {
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
func (ml *MessageList) bubbleContentLines(msg domain.Message, m bubbleMetrics) []string {
	actualW, innerW, b, bs := m.actualW, m.innerW, m.b, m.bs

	// Content lines: forward header (if any), reply quote block (if reply),
	// photo art (if any), then text.
	var sideLines []string

	if msg.Forward != nil {
		sideLines = append(sideLines, renderForwardLines(msg.Forward.From, actualW, bs)...)
		// Separate the forward header from any following content with a blank line.
		if msg.ReplyToMsgID != 0 || msg.Text != "" || msg.Media != nil {
			sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(innerW)+bs.Render(b.Right))
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
			sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(innerW)+bs.Render(b.Right))
		}
	}

	if msg.LocalMedia != nil {
		sideLines = append(sideLines, labelLine(localMediaLabel(msg.LocalMedia), actualW, b, bs))
		sideLines = append(sideLines, labelLine(uploadStatusLine(msg.LocalMedia, actualW), actualW, b, bs))
		if msg.Text != "" {
			sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(innerW)+bs.Render(b.Right))
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
		blankRow := bs.Render(b.Left) + theme.Pad(innerW) + bs.Render(b.Right)
		switch {
		case artLines != nil:
			for _, al := range artLines {
				al += theme.PadTo(lipgloss.Width(al), actualW)
				sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(1)+al+theme.Pad(1)+bs.Render(b.Right))
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
		case msg.Media.Kind == domain.MediaVoice && msg.Document != nil &&
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
		// canvas:ok this style only breaks lines. The text arrives painted run by
		// run from RenderEntities, and giving the wrapper a background would drop
		// it at the first reset inside the very text it is wrapping. Its own
		// padding is taken off below and re-emitted carrying the canvas.
		wrapStyle := lipgloss.NewStyle().Width(actualW)
		for _, part := range strings.Split(rendered, "\n") {
			if part == "" {
				sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(innerW)+bs.Render(b.Right))
				continue
			}
			for _, wl := range strings.Split(wrapStyle.Render(part), "\n") {
				wl = strings.TrimRight(wl, " ")
				wl += theme.PadTo(lipgloss.Width(wl), actualW)
				sideLines = append(sideLines, bs.Render(b.Left)+theme.Pad(1)+wl+theme.Pad(1)+bs.Render(b.Right))
			}
		}
	} else if len(sideLines) == 0 {
		sideLines = []string{bs.Render(b.Left) + theme.Pad(innerW) + bs.Render(b.Right)}
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
		// The margin an outgoing bubble is pushed across by is canvas, not gap.
		//
		// The selection indicator sits in that margin, one cell left of the
		// bubble, and the margin is built per line rather than spliced into
		// afterwards. It used to be spliced: the first leftPad bytes were ASCII
		// spaces, so byte offsets and cell offsets agreed. They do not any more —
		// the pad opens with an escape sequence — and slicing at a cell offset
		// would cut that sequence in half.
		margin := theme.Pad(leftPad)
		indicated := margin
		if leftPad >= 2 {
			indicated = theme.Pad(leftPad-1) + theme.S().Indicator.Render(indicatorChar)
		}
		bar := selected && ml.showIndicator && len(allLines) > 2 && leftPad >= 2
		for i := range allLines {
			// Content lines only: the top and bottom borders keep a clean margin.
			if bar && i > 0 && i < len(allLines)-1 {
				allLines[i] = indicated + allLines[i]
				continue
			}
			allLines[i] = margin + allLines[i]
		}
	} else {
		// Draw indicator bar on every content line to the right of the bubble.
		if selected && ml.showIndicator && len(allLines) > 2 {
			bubbleW := lipgloss.Width(allLines[0])
			available := ml.viewWidth - bubbleW
			if available >= 2 {
				bar := " " + theme.S().Indicator.Render(indicatorChar)
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
func (ml *MessageList) renderBareMedia(msg domain.Message, selected bool) []string {
	id, _ := ml.PreviewImageID(msg)
	img, _ := ml.cachedImage(id)
	bb := img.Bounds()
	cols, rows := ml.mediaBox(msg, bb.Dx(), bb.Dy())
	artLines := ml.renderer.Render(id, img, cols)

	// Timestamp + read status, shown on a plain line under the sticker.
	var statusStr string
	if msg.IsOut {
		if msg.ID > 0 && msg.ID <= ml.outboxReadMaxID {
			statusStr = theme.Pad(1) + theme.S().TickRead.Render("✓✓")
		} else if msg.ID > 0 {
			statusStr = theme.Pad(1) + theme.S().TickSent.Render("✓")
		}
	}
	editMark := ""
	if msg.EditDate != nil {
		editMark = theme.S().Timestamp.Render("edited") + " · "
	}
	tsStr := editMark + theme.S().Timestamp.Render(msg.Date.Format("15:04")) + statusStr
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

	pad := func(s string) string { return s + theme.PadTo(lipgloss.Width(s), blockW) }

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
			lines = append(lines, theme.Pad(blockW))
		}
	}
	if overlay := ml.overlayLabelFor(msg); overlay != "" {
		lines = append(lines, pad(overlay)) // ▶ duration / GIF badge under the thumbnail
	}
	fill := blockW - lipgloss.Width(reactStr) - lipgloss.Width(tsStr)
	if fill < 0 {
		fill = 0
	}
	lines = append(lines, pad(reactStr+theme.Pad(fill)+tsStr))

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
		pad := theme.Pad(leftPad)
		for i := range lines {
			lines[i] = pad + lines[i]
		}
		// leftPad bytes are ASCII spaces, so byte-slicing is safe.
		if selected && ml.showIndicator && leftPad >= 2 {
			bar := " " + theme.S().Indicator.Render(indicatorChar)
			for i := range lines {
				lines[i] = lines[i][:leftPad-2] + bar + lines[i][leftPad:]
			}
		}
		return lines
	}
	if selected && ml.showIndicator {
		if available := ml.viewWidth - blockW; available >= 2 {
			bar := " " + theme.S().Indicator.Render(indicatorChar)
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
	line := theme.S().Separator.Render(strings.Repeat("─", fill)) + " " + label + " " + theme.S().Separator.Render(strings.Repeat("─", rightFill))
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
	line := theme.S().UnreadSeparator.Render(strings.Repeat("─", fill)) + " " + theme.S().UnreadSeparator.Render(label) + " " + theme.S().UnreadSeparator.Render(strings.Repeat("─", rightFill))
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
	if item.kind == itemOutbox {
		return ml.renderBubble(item.msg, selected, outboxStatusGlyph(item.entry))
	}
	if len(item.parts) > 1 {
		return ml.renderGroupBubble(item.parts, selected)
	}
	return ml.renderMessage(item.msg, selected)
}

// renderGroupBubble draws a collapsed album: a mosaic grid when the album grids,
// otherwise the vertical stack. renderMosaic itself falls back to renderGroupStack
// when the plan says not to grid.
func (ml *MessageList) renderGroupBubble(parts []domain.Message, selected bool) []string {
	return ml.renderMosaic(parts, selected)
}

// renderGroupStack draws a Telegram album as a vertical stack: the sender/time
// frame once, then each media part as an informative "[n] <type/context>" badge
// line followed (for parts with a preview) by its scaled art, with a blank line
// between adjacent parts, then the shared caption. Previews are scaled down by
// albumImageRows so the album never spans several screens. It must stay in
// lock-step with groupHeightStack; TestGroupHeightMatchesRender guards that.
func (ml *MessageList) renderGroupStack(parts []domain.Message, selected bool) []string {
	media := groupMediaParts(parts)
	anchor := parts[0]
	caption := albumCaption(parts)

	// Frame width/identity: measure from an anchor bearing the caption so the
	// bubble is at least as wide as the text and the sender name. Clear Media so
	// the frame width is driven by the caption, badges, and previews, not by a
	// single message's placeholder.
	framing := anchor
	framing.Text = caption
	framing.Entities = albumCaptionEntities(parts)
	framing.Media = nil
	framing.Photo = nil
	framing.Document = nil
	m := ml.measureBubble(framing)
	// The caption wraps at its natural width (before badges or an image widen the
	// bubble), so the rendered caption line count matches groupHeight.
	captionW := m.actualW

	// Widen to fit the badge labels (labelLine does not truncate) and the
	// downscaled preview columns of any part whose bytes are known.
	widen := func(cols int) {
		if cols > m.actualW {
			m.actualW = cols
			m.innerW = cols + 2
		}
	}
	budget := ml.albumImageRows(parts)
	for _, gm := range media {
		// +1 for the one-column gap the folded badge adds (badge+" "); without it a
		// badge wider than a narrow preview overflows actualW and tears the border.
		widen(lipgloss.Width(albumBadgeLabel(gm.Index, gm.Msg)) + 1)
		if id, ok := ml.PreviewImageID(gm.Msg); ok {
			if img, has := ml.cachedImage(id); has {
				b := img.Bounds()
				cols, _ := ml.albumPartBox(budget, b.Dx(), b.Dy())
				widen(cols)
			}
		}
	}

	top, bottom := ml.bubbleBorders(framing, m)
	b, bs := m.b, m.bs
	blankRow := bs.Render(b.Left) + theme.Pad(m.innerW) + bs.Render(b.Right)

	lines := make([]string, 0, len(media)*(budget+2)+4)
	lines = append(lines, top)
	for i, gm := range media {
		badge := albumBadgeLabel(gm.Index, gm.Msg)
		if ml.albumPartHasCachedArt(gm.Msg) {
			// Fold the badge onto the image's first row (a Kitty "hole"), with a
			// one-column gap so the picture does not butt against the label text.
			lines = append(lines, ml.groupPartArt(gm.Msg, budget, badge+" ", m)...)
		} else {
			// No image to draw on: keep the badge as a standalone line, and reserve
			// a placeholder box for a photo whose bytes have not arrived yet.
			lines = append(lines, labelLine(badge, m.actualW, b, bs))
			if gm.Msg.Photo != nil {
				for j := 0; j < budget; j++ {
					lines = append(lines, blankRow)
				}
			}
		}
		if i < len(media)-1 {
			lines = append(lines, blankRow) // blank line between adjacent parts
		}
	}
	if caption != "" {
		lines = append(lines, blankRow)
		lines = append(lines, ml.captionLines(caption, albumCaptionEntities(parts), m, captionW)...)
	}
	lines = append(lines, bottom)
	return ml.alignBubbleLines(lines, anchor.IsOut, selected)
}

// groupPartArt renders one album part's downscaled art rows, with the badge
// folded onto the first row: overlayBadgeOnArtRow replaces that row's leading
// columns so the "[n] type" label reads over the top-left of the picture (a Kitty
// hole where no image is drawn). The image is rendered into an albumPartBox (fit
// whole image, never crop). It reserves exactly albumPartRows rows, filling any
// not-yet-transmitted rows with blanks so the height is stable while a Kitty
// placement is still in flight (issue #115). Caller must have verified
// albumPartHasCachedArt.
func (ml *MessageList) groupPartArt(msg domain.Message, budget int, badge string, m bubbleMetrics) []string {
	b, bs := m.b, m.bs
	reserve := ml.albumPartRows(budget, msg)
	id, _ := ml.PreviewImageID(msg)
	img, _ := ml.cachedImage(id)
	cols, _ := ml.albumPartBox(budget, img.Bounds().Dx(), img.Bounds().Dy())
	artLines := ml.renderer.Render(id, img, cols) // nil until the Kitty placement is live

	out := make([]string, 0, reserve)
	for i := 0; i < reserve; i++ {
		al := ""
		if i < len(artLines) {
			al = artLines[i]
		}
		if i == 0 {
			al = overlayBadgeOnArtRow(al, badge, cols)
		}
		al += theme.PadTo(lipgloss.Width(al), m.actualW)
		out = append(out, bs.Render(b.Left)+theme.Pad(1)+al+theme.Pad(1)+bs.Render(b.Right))
	}
	return out
}

// overlayBadgeOnArtRow composites label onto the leading columns of one art row.
// It drops the first labelW display columns of artRow and prepends the label, so
// the label reads over the picture: for a Kitty placeholder row the dropped cells
// leave a hole where the terminal draws no image; for block art the label
// overwrites the blocks. xansi.TruncateLeft is width-aware, so it slices whole
// placeholder cells (rune + diacritics) and re-emits the active foreground for the
// remaining image cells.
func overlayBadgeOnArtRow(artRow, label string, imgCols int) string {
	labelW := lipgloss.Width(label)
	if labelW >= imgCols {
		return label // label spans the whole row; no image cells remain
	}
	return label + xansi.TruncateLeft(artRow, labelW, "")
}

// captionLines wraps the album caption inside the bubble borders, matching the
// text path of bubbleContentLines.
func (ml *MessageList) captionLines(text string, entities []domain.MessageEntity, m bubbleMetrics, wrapW int) []string {
	b, bs := m.b, m.bs
	rendered := RenderEntities(text, entities)
	// canvas:ok breaks lines only; see the text path of bubbleContentLines for
	// why a wrapper of painted text cannot carry the canvas itself.
	wrapStyle := lipgloss.NewStyle().Width(wrapW)
	var out []string
	for _, part := range strings.Split(rendered, "\n") {
		if part == "" {
			out = append(out, bs.Render(b.Left)+theme.Pad(m.innerW)+bs.Render(b.Right))
			continue
		}
		for _, wl := range strings.Split(wrapStyle.Render(part), "\n") {
			wl = strings.TrimRight(wl, " ")
			wl += theme.PadTo(lipgloss.Width(wl), m.actualW)
			out = append(out, bs.Render(b.Left)+theme.Pad(1)+wl+theme.Pad(1)+bs.Render(b.Right))
		}
	}
	return out
}

func (ml *MessageList) View() string {
	ml.selRectOK = false
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 {
		return ""
	}
	if len(ml.items) == 0 {
		return strings.Repeat("\n", ml.viewHeight-1)
	}

	// The top padding below is for a history shorter than the pane, not for an
	// anchor that a mutation left too low. Repair the anchor first so the two
	// cases stay distinguishable (#225).
	ml.reanchorIfUnderfilled()

	selectedID := ml.computeSelectedMsgID()
	// A queued send has no message id, so it has to be recognised by its ref —
	// otherwise the selection indicator is never drawn beside it even though the
	// cursor is there (#193).
	selectedRef := ml.SelectedOutboxRef()

	// Whether the viewport sits where the scroll clamp calls the bottom. It
	// decides both how a too-tall frame is trimmed and, below, whether the loop
	// may stop early at all.
	botIdx, botOff := ml.positionAtBottom()
	atNaturalBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff

	var allLines []string
	reachedEnd := true
	selTopRaw, selHeight, selLeft, selWidth := 0, 0, 0, 0
	for i := ml.viewStart; i < len(ml.items); i++ {
		var selected bool
		switch ml.items[i].kind {
		case itemMessage:
			selected = ml.items[i].msg.ID == selectedID
		case itemOutbox:
			selected = selectedRef != "" && ml.items[i].entry.Ref == selectedRef
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
		// Stopping as soon as the frame is full is what keeps this loop off the
		// whole history - but only away from the bottom. At the bottom the frame
		// has to reach the last item, because that is the item the trim below is
		// asked to keep. Stopping short there left `reachedEnd` false while the
		// clamp insisted the viewport was already at the bottom, and the trim
		// resolved that contradiction by cutting the newest rows away: the last
		// messages became unreachable, with every scroll key a no-op and nothing
		// on screen to say there was more (#231). Rendering on costs the few
		// items between here and the end, which is what positionAtBottom just
		// walked to choose the start.
		if len(allLines) >= ml.viewHeight && !atNaturalBottom {
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
