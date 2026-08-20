package components

// The active-message cursor is an explicit selection the user steps over bubble
// by bubble (distinct from line/page scrolling). It is tracked by message ID so
// it survives item rebuilds (prepend/edit). The viewport follows the cursor and
// keeps it vertically centered, clamped at the top and natural bottom.

// setCursor writes the mutually exclusive pair. The invariant that at most one
// of cursorMsgID and cursorOutboxRef is set lives here and nowhere else (#193),
// so every move goes through it — by item index via placeCursor, or by message
// id where the item does not exist yet (outboxItems following a delivered send
// into the window, #226).
func (ml *MessageList) setCursor(msgID int, ref string) {
	ml.cursorMsgID, ml.cursorOutboxRef = msgID, ref
}

// placeCursor puts the cursor on one item, whichever kind it is.
func (ml *MessageList) placeCursor(i int) {
	if i < 0 || i >= len(ml.items) {
		ml.setCursor(0, "")
		return
	}
	if ml.items[i].kind == itemOutbox {
		ml.setCursor(0, ml.items[i].entry.Ref)
		return
	}
	ml.setCursor(ml.items[i].msg.ID, "")
}

// selectable reports whether the cursor may rest on an item: messages and
// queued sends, not separators.
func (ml *MessageList) selectable(i int) bool {
	k := ml.items[i].kind
	return k == itemMessage || k == itemOutbox
}

// cursorIndex returns the items index the cursor is on, or -1 when unset or no
// longer present.
func (ml *MessageList) cursorIndex() int {
	if ml.cursorOutboxRef != "" {
		for i := range ml.items {
			if ml.items[i].kind == itemOutbox && ml.items[i].entry.Ref == ml.cursorOutboxRef {
				return i
			}
		}
		return -1
	}
	if ml.cursorMsgID == 0 {
		return -1
	}
	for i := range ml.items {
		if ml.items[i].kind == itemMessage && ml.items[i].msg.ID == ml.cursorMsgID {
			return i
		}
	}
	return -1
}

// setCursorNewest parks the cursor on the newest selectable item — a queued
// send when there is one, since it sits below every message.
func (ml *MessageList) setCursorNewest() {
	for i := len(ml.items) - 1; i >= 0; i-- {
		if ml.selectable(i) {
			ml.placeCursor(i)
			return
		}
	}
	ml.placeCursor(-1)
}

// CursorUp moves the active-message cursor one bubble toward older history and
// scrolls the viewport so the cursor stays centered. Returns true when the
// cursor is at the oldest loaded message, so the caller can prefetch history.
func (ml *MessageList) CursorUp() bool {
	idx := ml.cursorIndex()
	if idx < 0 {
		ml.setCursorNewest()
		idx = ml.cursorIndex()
		if idx < 0 {
			return true
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if ml.selectable(i) {
			ml.placeCursor(i)
			ml.revealCursorUp()
			return ml.cursorMsgID != 0 && ml.cursorMsgID == ml.OldestID()
		}
	}
	// Already on the oldest loaded message.
	return true
}

// CursorDown moves the active-message cursor one bubble toward newer messages.
func (ml *MessageList) CursorDown() {
	idx := ml.cursorIndex()
	if idx < 0 {
		ml.setCursorNewest()
		return
	}
	for i := idx + 1; i < len(ml.items); i++ {
		if ml.selectable(i) {
			ml.placeCursor(i)
			ml.revealCursorDown()
			return
		}
	}
}

// cursorTopRow returns the cursor bubble's top row relative to the viewport's
// first visible line (0 = top line). Negative means the cursor is above the
// viewport; >= viewHeight means below it.
func (ml *MessageList) cursorTopRow() int {
	idx := ml.cursorIndex()
	if idx < 0 {
		return 0
	}
	row := -ml.lineOffset
	if idx >= ml.viewStart {
		for i := ml.viewStart; i < idx; i++ {
			row += ml.itemHeight(i)
		}
	} else {
		for i := idx; i < ml.viewStart; i++ {
			row -= ml.itemHeight(i)
		}
	}
	return row
}

// revealCursorUp keeps the cursor at or below the vertical middle after stepping
// to an older message: while the cursor is still in the lower half it simply
// rises within the viewport (no scroll); once it would cross above the middle
// the viewport scrolls up to hold it there. This avoids a teleport/jump when the
// cursor was sitting at the bottom edge (e.g. right after a line scroll).
func (ml *MessageList) revealCursorUp() {
	if ml.viewHeight <= 0 {
		return
	}
	if ml.cursorTopRow() < ml.viewHeight/2 {
		ml.scrollCursorToMiddle()
	}
}

// revealCursorDown keeps the cursor on screen after stepping to a newer message:
// it descends within the viewport until it reaches the bottom, then the viewport
// scrolls down just enough to keep the cursor fully visible.
func (ml *MessageList) revealCursorDown() {
	idx := ml.cursorIndex()
	if idx < 0 || ml.viewHeight <= 0 {
		return
	}
	h := ml.itemHeight(idx)
	for i := 0; i <= ml.viewHeight && ml.cursorTopRow()+h > ml.viewHeight; i++ {
		before := ml.viewStart
		beforeOff := ml.lineOffset
		ml.scrollDownLine()
		if ml.viewStart == before && ml.lineOffset == beforeOff {
			break // hit the natural bottom; can't reveal further
		}
	}
}

// scrollCursorToMiddle positions the viewport so the cursor bubble's top sits at
// the vertical middle, leaving ~half a screen of older content above it. Clamped
// so the viewport never scrolls past the top of history or below the natural
// bottom — near the ends the cursor drifts off-center accordingly.
func (ml *MessageList) scrollCursorToMiddle() {
	idx := ml.cursorIndex()
	if idx < 0 {
		return
	}
	need := ml.viewHeight / 2 // lines of older content to keep above the cursor
	vs, lo := 0, 0
	for j := idx - 1; j >= 0; j-- {
		h := ml.itemHeight(j)
		if h < need {
			need -= h
			continue
		}
		vs, lo, need = j, h-need, 0
		break
	}
	if need > 0 {
		// Not enough older content to center: anchor at the very top.
		vs, lo = 0, 0
	}
	ml.viewStart, ml.lineOffset = ml.clampToBounds(vs, lo)
}

// visibleMessageRange returns the first and last items indices of messages that
// have at least one line within the viewport. ok is false when no message is
// visible (empty list or unsized viewport).
func (ml *MessageList) visibleMessageRange() (first, last int, ok bool) {
	first, last = -1, -1
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.items) && linesUsed < ml.viewHeight; i++ {
		skipped := 0
		if i == ml.viewStart {
			skipped = ml.lineOffset
		}
		visible := ml.itemHeight(i) - skipped
		if visible > 0 && ml.items[i].kind == itemMessage {
			if first == -1 {
				first = i
			}
			last = i
		}
		linesUsed += visible
	}
	return first, last, first != -1
}

// clampCursorToViewport keeps the active-message cursor on screen after a line
// or page scroll: if the cursor message scrolled off an edge, it snaps to the
// nearest still-visible message. The viewport itself is left untouched.
func (ml *MessageList) clampCursorToViewport() {
	idx := ml.cursorIndex()
	if idx < 0 {
		return
	}
	first, last, ok := ml.visibleMessageRange()
	if !ok {
		return
	}
	if idx < first {
		ml.placeCursor(first)
	} else if idx > last {
		ml.placeCursor(last)
	}
}

// clampToBounds keeps a (viewStart, lineOffset) position within the valid range
// [top, naturalBottom].
func (ml *MessageList) clampToBounds(vs, lo int) (int, int) {
	if vs < 0 {
		return 0, 0
	}
	botVs, botLo := ml.positionAtBottom()
	if vs > botVs || (vs == botVs && lo > botLo) {
		return botVs, botLo
	}
	return vs, lo
}
