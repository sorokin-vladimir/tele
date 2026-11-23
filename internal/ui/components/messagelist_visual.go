package components

import "github.com/sorokin-vladimir/tele/internal/domain"

// visualSelection is the Visual mode selection of a message list: a range being
// stretched from anchorID to the cursor, plus the messages marked one by one.
// anchorID 0 means no range is being stretched, so the selection is only the
// marked messages.
type visualSelection struct {
	on       bool
	anchorID int
	marked   map[int]struct{}
}

// StartVisualRange enters Visual mode with a range anchored at the focused
// message. It reports false, without entering the mode, when no message is
// focused.
func (ml *MessageList) StartVisualRange() bool {
	id := ml.cursorMessageID()
	if id == 0 {
		return false
	}
	ml.visual = visualSelection{on: true, anchorID: id, marked: map[int]struct{}{}}
	return true
}

// StartVisualMark enters Visual mode with only the focused message marked. It
// reports false, without entering the mode, when no message is focused.
func (ml *MessageList) StartVisualMark() bool {
	i := ml.cursorMessageIndex()
	if i < 0 {
		return false
	}
	ml.visual = visualSelection{on: true, marked: map[int]struct{}{}}
	ml.markItem(i)
	return true
}

// FixOrStartVisualRange is V inside Visual mode: it marks every message of the
// range being stretched, or starts a new range at the focused message when none
// is.
func (ml *MessageList) FixOrStartVisualRange() {
	if !ml.visual.on {
		return
	}
	if ml.visual.anchorID != 0 {
		ml.fixVisualRange()
		return
	}
	ml.visual.anchorID = ml.cursorMessageID()
}

// FixRangeOrToggleMark is m inside Visual mode: it marks every message of the
// range being stretched, the focused one included, or marks or unmarks the
// focused message when no range is.
func (ml *MessageList) FixRangeOrToggleMark() {
	if !ml.visual.on {
		return
	}
	if ml.visual.anchorID != 0 {
		ml.fixVisualRange()
		return
	}
	i := ml.cursorMessageIndex()
	if i < 0 {
		return
	}
	if ml.itemMarked(i) {
		ml.unmarkItem(i)
		return
	}
	ml.markItem(i)
}

// ClearVisual leaves Visual mode and drops the selection.
func (ml *MessageList) ClearVisual() { ml.visual = visualSelection{} }

// InVisual reports whether the list is in Visual mode.
func (ml *MessageList) InVisual() bool { return ml.visual.on }

// VisualSelection returns the selected messages oldest first, with every
// message of a selected album.
func (ml *MessageList) VisualSelection() []domain.Message {
	var out []domain.Message
	for _, i := range ml.selectedItemIndices() {
		out = append(out, ml.items[i].parts...)
	}
	return out
}

// VisualHasGaps reports whether a message between the oldest and the newest
// selected ones is left out of the selection.
func (ml *MessageList) VisualHasGaps() bool {
	selected := ml.selectedItemIndices()
	if len(selected) == 0 {
		return false
	}
	messagesBetween := 0
	for i := selected[0]; i <= selected[len(selected)-1]; i++ {
		if ml.items[i].kind == itemMessage {
			messagesBetween++
		}
	}
	return messagesBetween != len(selected)
}

// CursorToOldest moves the cursor to the oldest loaded message.
func (ml *MessageList) CursorToOldest() {
	for i := range ml.items {
		if ml.selectable(i) {
			ml.placeCursor(i)
			return
		}
	}
}

func (ml *MessageList) cursorMessageIndex() int {
	i := ml.cursorIndex()
	if i < 0 || ml.items[i].kind != itemMessage {
		return -1
	}
	return i
}

func (ml *MessageList) cursorMessageID() int {
	if i := ml.cursorMessageIndex(); i >= 0 {
		return ml.items[i].msg.ID
	}
	return 0
}

func (ml *MessageList) markItem(i int) {
	for _, p := range ml.items[i].parts {
		ml.visual.marked[p.ID] = struct{}{}
	}
}

func (ml *MessageList) unmarkItem(i int) {
	for _, p := range ml.items[i].parts {
		delete(ml.visual.marked, p.ID)
	}
}

func (ml *MessageList) itemMarked(i int) bool {
	for _, p := range ml.items[i].parts {
		if _, ok := ml.visual.marked[p.ID]; ok {
			return true
		}
	}
	return false
}

func (ml *MessageList) fixVisualRange() {
	if lo, hi, ok := ml.visualRangeBounds(); ok {
		for i := lo; i <= hi; i++ {
			if ml.items[i].kind == itemMessage {
				ml.markItem(i)
			}
		}
	}
	ml.visual.anchorID = 0
}

func (ml *MessageList) visualRangeBounds() (lo, hi int, ok bool) {
	if !ml.visual.on || ml.visual.anchorID == 0 {
		return 0, 0, false
	}
	anchor := ml.itemIndexHolding(ml.visual.anchorID)
	cursor := ml.cursorIndex()
	if anchor < 0 || cursor < 0 {
		return 0, 0, false
	}
	return min(anchor, cursor), max(anchor, cursor), true
}

func (ml *MessageList) selectedItemIndices() []int {
	if !ml.visual.on {
		return nil
	}
	lo, hi, ranged := ml.visualRangeBounds()
	var out []int
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		if (ranged && i >= lo && i <= hi) || ml.itemMarked(i) {
			out = append(out, i)
		}
	}
	return out
}

func (ml *MessageList) selectedPartIDs() map[int]struct{} {
	indices := ml.selectedItemIndices()
	if len(indices) == 0 {
		return nil
	}
	ids := make(map[int]struct{})
	for _, i := range indices {
		for _, p := range ml.items[i].parts {
			ids[p.ID] = struct{}{}
		}
	}
	return ids
}

func (ml *MessageList) itemIndexHolding(id int) int {
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		for _, p := range ml.items[i].parts {
			if p.ID == id {
				return i
			}
		}
	}
	return -1
}

func (ml *MessageList) keepVisualAfterRemoval(before []listItem) {
	if !ml.visual.on {
		return
	}
	oldCursor := indexOfMessage(before, ml.cursorMsgID)
	oldAnchor := indexOfMessage(before, ml.visual.anchorID)
	if oldCursor >= 0 && ml.cursorIndex() < 0 {
		toward := oldCursor + 1
		if oldAnchor >= 0 && oldAnchor != oldCursor {
			toward = oldAnchor
		}
		ml.setCursor(ml.survivorIDNear(before, oldCursor, toward), "")
	}
	if ml.visual.anchorID != 0 && ml.itemIndexHolding(ml.visual.anchorID) < 0 {
		toward := oldCursor
		if toward < 0 || toward == oldAnchor {
			toward = oldAnchor + 1
		}
		ml.visual.anchorID = ml.survivorIDNear(before, oldAnchor, toward)
	}
}

func (ml *MessageList) reloadKeepingVisual(msgs []domain.Message) bool {
	if !ml.visual.on || len(msgs) == 0 || msgs[0].ChatID != ml.firstMessageChatID() {
		return false
	}
	rowOnScreen := ml.messageRowsOnScreen()
	before := ml.items
	ml.items = ml.buildItems(msgs)
	ml.invalidateHeights()
	ml.keepVisualAfterRemoval(before)
	if ml.cursorIndex() < 0 {
		return false
	}
	ml.viewStart, ml.lineOffset = ml.clampToBounds(ml.viewStart, ml.lineOffset)
	ml.putFirstSurvivorOnItsRow(rowOnScreen)
	return true
}

func (ml *MessageList) putFirstSurvivorOnItsRow(rowOnScreen map[int]int) bool {
	for i, it := range ml.items {
		if row, ok := rowOnScreen[it.msg.ID]; ok && it.kind == itemMessage {
			ml.putItemOnRow(i, row)
			return true
		}
	}
	return false
}

func (ml *MessageList) firstMessageChatID() int64 {
	for _, it := range ml.items {
		if it.kind == itemMessage {
			return it.msg.ChatID
		}
	}
	return 0
}

func (ml *MessageList) messageRowsOnScreen() map[int]int {
	rows := map[int]int{}
	row := -ml.lineOffset
	for i := ml.viewStart; i < len(ml.items) && row < ml.viewHeight; i++ {
		if ml.items[i].kind == itemMessage {
			rows[ml.items[i].msg.ID] = row
		}
		row += ml.itemHeight(i)
	}
	return rows
}

func (ml *MessageList) putItemOnRow(i, row int) {
	viewStart, lineOffset := i, 0
	if row < 0 {
		lineOffset = -row
	}
	for row > 0 && viewStart > 0 {
		viewStart--
		h := ml.itemHeight(viewStart)
		if h > row {
			lineOffset = h - row
			break
		}
		row -= h
	}
	ml.viewStart, ml.lineOffset = ml.clampToBounds(viewStart, lineOffset)
}

func (ml *MessageList) survivorIDNear(before []listItem, from, toward int) int {
	if from < 0 {
		return 0
	}
	step := 1
	if toward < from {
		step = -1
	}
	for _, dir := range []int{step, -step} {
		for i := from; i >= 0 && i < len(before); i += dir {
			if before[i].kind != itemMessage {
				continue
			}
			for _, p := range before[i].parts {
				if j := ml.itemIndexHolding(p.ID); j >= 0 {
					return ml.items[j].msg.ID
				}
			}
		}
	}
	return 0
}

func indexOfMessage(items []listItem, id int) int {
	if id == 0 {
		return -1
	}
	for i := range items {
		if items[i].kind == itemMessage && items[i].msg.ID == id {
			return i
		}
	}
	return -1
}

func (ml *MessageList) bareMediaSelected(i int) bool {
	it := ml.items[i]
	if it.kind != itemMessage || len(it.parts) != 1 || !ml.isBareMedia(it.msg) {
		return false
	}
	_, ok := ml.selectedWhileDrawing[it.msg.ID]
	return ok
}
