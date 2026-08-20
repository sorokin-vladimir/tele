package components

// ScrollInfo reports the message list's scroll position in rendered lines,
// measured over the currently loaded window. Total grows as older history is
// prepended; the viewport is top-anchored so visible content does not jump.
// The root builds the pane's scrollbar before it renders the pane, so the
// anchor is repaired here too: otherwise the thumb spends a frame describing a
// position the very next View() throws away.
func (ml *MessageList) ScrollInfo() ScrollInfo {
	ml.reanchorIfUnderfilled()
	total := 0
	for i := range ml.items {
		total += ml.itemHeight(i)
	}
	offset := 0
	for i := 0; i < ml.viewStart && i < len(ml.items); i++ {
		offset += ml.itemHeight(i)
	}
	offset += ml.lineOffset
	return ScrollInfo{Total: total, Visible: ml.viewHeight, Offset: offset}
}

// VisiblePhotoIDs returns the inline-image cache keys (PreviewImageID) for the
// messages currently within the viewport, top to bottom. Kitty placements are a
// bounded terminal resource, so the root transmits/keeps only on-screen images.
// The visible range mirrors View's accumulation from viewStart.
func (ml *MessageList) VisiblePhotoIDs() []int64 {
	var ids []int64
	lines := 0
	for i := ml.viewStart; i < len(ml.items) && lines < ml.viewHeight; i++ {
		h := ml.itemHeight(i)
		if i == ml.viewStart {
			h -= ml.lineOffset
		}
		lines += h
		if ml.items[i].kind == itemMessage {
			// Request every album part's preview, not just the anchor, so sibling
			// previews in a collapsed album bubble download too.
			for _, p := range ml.items[i].parts {
				if id, ok := ml.PreviewImageID(p); ok {
					ids = append(ids, id)
				}
			}
		}
	}
	return ids
}

func (ml *MessageList) ScrollToBottom() {
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	ml.setCursorNewest()
}

func (ml *MessageList) ScrollToTop() {
	ml.viewStart = 0
	ml.lineOffset = 0
}

func (ml *MessageList) ScrollDownBy(n int) {
	for i := 0; i < n; i++ {
		ml.ScrollDown()
	}
}

func (ml *MessageList) ScrollUpBy(n int) {
	for i := 0; i < n; i++ {
		ml.ScrollUp()
	}
}

// VisibleReadMaxID returns the highest message ID that is "sufficiently visible" to count
// as read: either more than half its lines are in the viewport, or it fills the entire
// viewport (so more than half is impossible to show at once). Returns 0 if none qualify.
//
// An album renders as one item but is several Telegram messages, so a visible album
// contributes its last part's ID, not the anchor's. Reporting the anchor would leave
// the remaining parts unread forever: the server's read_inbox_max_id would stop at the
// anchor, and the item can never be scrolled past to bump it (issue: album never read).
func (ml *MessageList) VisibleReadMaxID() int {
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 || len(ml.items) == 0 {
		return 0
	}
	maxID := 0
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.items) && linesUsed < ml.viewHeight; i++ {
		h := ml.itemHeight(i)
		if ml.items[i].kind != itemMessage {
			linesUsed += h
			continue
		}
		itemMaxID := ml.items[i].msg.ID
		for _, p := range ml.items[i].parts {
			if p.ID > itemMaxID {
				itemMaxID = p.ID
			}
		}
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
			if itemMaxID > maxID {
				maxID = itemMaxID
			}
		}
		linesUsed += visibleLines
	}
	return maxID
}

// ScrollToFirstUnread positions the viewport at the first message with ID > readMaxID.
// If an itemUnreadSeparator immediately precedes that message it is included at the top
// so the divider is always visible. If the remaining messages don't fill the viewport,
// older messages are pulled in to fill the space (same as positionAtBottom).
// Returns false if all messages are already read (nothing to jump to).
func (ml *MessageList) ScrollToFirstUnread(readMaxID int) bool {
	for i, item := range ml.items {
		if item.kind != itemMessage {
			continue
		}
		if item.msg.ID > readMaxID {
			start := i
			if i > 0 && ml.items[i-1].kind == itemUnreadSeparator {
				start = i - 1
			}
			ml.placeCursor(i)
			ml.viewStart = start
			ml.lineOffset = 0
			lines := 0
			for j := start; j < len(ml.items); j++ {
				lines += ml.itemHeight(j)
			}
			if lines < ml.viewHeight {
				ml.viewStart, ml.lineOffset = ml.positionAtBottom()
			}
			return true
		}
	}
	return false
}

// ScrollUp moves the viewport one line toward older messages, dragging the
// active-message cursor along if it would otherwise scroll off screen.
func (ml *MessageList) ScrollUp() {
	ml.scrollUpLine()
	ml.clampCursorToViewport()
}

// scrollUpLine pans the viewport one line toward older messages.
// When crossing a message boundary, small messages (h <= viewHeight) are entered at
// lineOffset=h-2 so at least content+bottom are visible (never bottom-border-only).
// Large messages are entered at their bottom portion (lineOffset=h-viewHeight).
func (ml *MessageList) scrollUpLine() {
	if ml.lineOffset > 0 {
		ml.lineOffset--
		return
	}
	if ml.viewStart > 0 {
		ml.viewStart--
		h := ml.itemHeight(ml.viewStart)
		if h > ml.viewHeight {
			ml.lineOffset = h - ml.viewHeight
		} else {
			// Enter showing content+bottom border; lineOffset=h-1 (bottom-only) is skipped.
			ml.lineOffset = h - 2
		}
	}
}

// ScrollDown moves the viewport one line toward newer messages, dragging the
// active-message cursor along if it would otherwise scroll off screen.
func (ml *MessageList) ScrollDown() {
	ml.scrollDownLine()
	ml.clampCursorToViewport()
}

// scrollDownLine pans the viewport one line toward newer messages.
// Scrolls line-by-line but skips lineOffset=h-1 (bottom-border-only frame).
// The at-bottom check (positionAtBottom) is the primary stop condition.
func (ml *MessageList) scrollDownLine() {
	botIdx, botOff := ml.positionAtBottom()
	if ml.viewStart > botIdx || (ml.viewStart == botIdx && ml.lineOffset >= botOff) {
		return
	}
	h := ml.itemHeight(ml.viewStart)
	if ml.lineOffset+1 < h-1 {
		ml.lineOffset++
		return
	}
	if ml.viewStart+1 < len(ml.items) {
		ml.viewStart++
		ml.lineOffset = 0
	}
}

// positionAtBottom returns (msgIdx, lineOffset) for the viewport bottom position.
// lineOffset > 0 means the first visible message is shown from its bottom portion,
// filling the space that would otherwise be empty above the last full messages.
func (ml *MessageList) positionAtBottom() (int, int) {
	lineCount := 0
	for i := len(ml.items) - 1; i >= 0; i-- {
		h := ml.itemHeight(i)
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

// reanchorIfUnderfilled restores the invariant that a frame running to the last
// item never leaves blank rows at the top while older content sits above the
// viewport. View pads such a frame from the top, which is only right when the
// whole history is shorter than the pane. Every mutation that shrinks the
// content below the anchor breaks that: older messages prepended above
// viewStart (PrependMessages deliberately keeps the visual position), a
// collapsing composer growing viewHeight through SetSize, an image dropped from
// the cache turning a tall bubble back into a placeholder. The padding then
// freezes a blank band that only a scroll key clears, because scrollUpLine is
// the first thing to recompute the position (#225).
//
// Re-anchoring at the bottom fills those rows with history that is already
// loaded. The cursor is left alone: the bottom position keeps the same last
// line and only extends the frame upward, so whatever was on screen still is.
// Callers are the reads that depend on the anchor; it is idempotent and stops
// as soon as the viewport is proven full, so calling it per frame is cheap.
func (ml *MessageList) reanchorIfUnderfilled() {
	if ml.viewHeight <= 0 || len(ml.items) == 0 {
		return
	}
	if ml.viewStart <= 0 && ml.lineOffset <= 0 {
		return // nothing above the viewport: a short history pads legitimately
	}
	lines := -ml.lineOffset
	for i := ml.viewStart; i < len(ml.items); i++ {
		lines += ml.itemHeight(i)
		if lines >= ml.viewHeight {
			return
		}
	}
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
}

func (ml *MessageList) ScrollToMessage(id int) bool {
	for i, item := range ml.items {
		if item.kind != itemMessage || item.msg.ID != id {
			continue
		}
		ml.placeCursor(i)
		ml.viewStart = i
		ml.lineOffset = 0
		lines := 0
		for j := i; j < len(ml.items); j++ {
			lines += ml.itemHeight(j)
		}
		if lines < ml.viewHeight {
			ml.viewStart, ml.lineOffset = ml.positionAtBottom()
		}
		return true
	}
	return false
}
