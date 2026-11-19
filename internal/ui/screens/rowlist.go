package screens

import (
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// rowList is a window onto a list the core owns: the chat list, or a forum's
// topic list. The core owns order and filtering and hands over a slice around
// what is on screen; cursor indexes the whole list, so it stays meaningful when
// the window moves under it. What a row is and how it is drawn belongs to the
// list that embeds this one (#275).
type rowList[R any, K comparable] struct {
	rows   []R
	offset int // index of rows[0] in the whole list
	total  int // length of the whole list
	cursor int // index into the whole list, not into rows
	// reqOffset/reqLimit are the last window asked for, so a repeated request
	// for the same window is silent. Zero limit means nothing asked for yet.
	reqOffset int
	reqLimit  int
	// activeKey is the open row, held by key rather than index so it survives a
	// window that no longer contains it.
	activeKey K
	height    int
	focused   bool

	// highlightKey is the row currently flashed because something arrived in
	// it; highlightStep counts down to 0 (none). Tracked by key so the highlight
	// survives a window reset.
	highlightKey  K
	highlightStep int

	// keyOf names a row.
	keyOf func(R) K
}

// SetWindow replaces the window with the contents of a Reset delta. The cursor
// follows the row it was on, which is what makes a Reset — emitted on every
// reorder — non-disruptive.
//
// When the cursor sits outside the window being replaced, there is no row to
// follow and the numeric position is kept instead. That case is not exotic: a
// jump to the end of the list moves the cursor first and only then asks for the
// window around it, so the arriving window is precisely the one the cursor is
// no longer inside.
func (m *rowList[R, K]) SetWindow(offset, total int, rows []R) {
	var zero K
	cursorKey := zero
	if r, ok := m.rowAt(m.cursor); ok {
		cursorKey = m.keyOf(r)
	}

	m.rows = rows
	m.offset = offset
	m.total = total

	if cursorKey != zero {
		for i, r := range rows {
			if m.keyOf(r) == cursorKey {
				m.cursor = offset + i
				break
			}
		}
	}
	m.clampCursor()
}

// SetRow replaces one row in place, from a Row delta. The row is found by key: a
// Row is only emitted while the window's order is unchanged, so the key is
// unambiguous and no index travels on the wire.
func (m *rowList[R, K]) SetRow(row R) {
	k := m.keyOf(row)
	for i := range m.rows {
		if m.keyOf(m.rows[i]) == k {
			m.rows[i] = row
			return
		}
	}
}

// rowAt returns the row at a whole-list index, translating through the window.
// ok is false when the index falls outside the window the core has sent.
func (m *rowList[R, K]) rowAt(i int) (R, bool) {
	j := i - m.offset
	if j < 0 || j >= len(m.rows) {
		var zero R
		return zero, false
	}
	return m.rows[j], true
}

// viewStart is the whole-list index of the first visible row. It is the single
// definition of the scroll position: View, ScrollInfo, CursorViewportRow and
// the viewport-row lookups all derive from it, because four copies of this
// arithmetic is how a click lands on the wrong row.
func (m *rowList[R, K]) viewStart() int {
	if m.height <= 0 {
		return 0
	}
	start := m.cursor - m.height + 1
	if start < 0 {
		start = 0
	}
	if max := m.total - m.height; start > max {
		if max < 0 {
			max = 0
		}
		start = max
	}
	return start
}

func (m *rowList[R, K]) clampCursor() {
	if m.total == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.total {
		m.cursor = m.total - 1
	}
}

// WindowRequest reports the window the model wants for its current scroll
// position: the visible rows plus one screen of overscan either side, so
// ordinary scrolling never waits on a round trip. changed is false when this is
// the window it last asked for, which keeps a quiet list quiet.
//
// It compares against the last request rather than against the window it holds,
// because at the end of the list the core legitimately returns fewer rows than
// asked for and a held-window comparison would ask again forever.
func (m *rowList[R, K]) WindowRequest() (offset, limit int, changed bool) {
	h := m.height
	if h <= 0 {
		h = 1
	}
	offset = m.viewStart() - h
	if offset < 0 {
		offset = 0
	}
	limit = 3 * h
	if offset == m.reqOffset && limit == m.reqLimit {
		return offset, limit, false
	}
	m.reqOffset, m.reqLimit = offset, limit
	return offset, limit, true
}

// highlight starts a fade highlight on the row with the given key.
func (m *rowList[R, K]) highlight(key K) {
	m.highlightKey = key
	m.highlightStep = components.HighlightInitialStep
}

// StepHighlight advances the row highlight fade by one step. Returns true while
// still active; clears the highlight and returns false at 0. No-op (false) when
// no highlight is active.
func (m *rowList[R, K]) StepHighlight() bool {
	if m.highlightStep <= 0 {
		return false
	}
	m.highlightStep--
	if m.highlightStep <= 0 {
		var zero K
		m.highlightKey = zero
		return false
	}
	return true
}

// HighlightStep returns the current row fade step (0 when none).
func (m *rowList[R, K]) HighlightStep() int { return m.highlightStep }

func (m *rowList[R, K]) Cursor() int { return m.cursor }

// Rows returns the window's rows. For assertions and for the mouse mapping;
// callers must not assume it is the whole list.
func (m *rowList[R, K]) Rows() []R { return m.rows }

// Total is the length of the whole list, which the window is a slice of.
func (m *rowList[R, K]) Total() int { return m.total }

// ActiveIdx is the whole-list index of the open row, or -1 when it is outside
// the window (or nothing is open).
func (m *rowList[R, K]) ActiveIdx() int {
	var zero K
	if m.activeKey == zero {
		return -1
	}
	for i, r := range m.rows {
		if m.keyOf(r) == m.activeKey {
			return m.offset + i
		}
	}
	return -1
}

// SetActive marks a row as the open one. It does not touch the cursor: the
// window is replaced on every reorder, and dragging the cursor back to the open
// row each time would pin it there for as long as it is open.
func (m *rowList[R, K]) SetActiveKey(key K) { m.activeKey = key }

// SetActiveByKey marks a row as the open one and moves the cursor onto it. For
// the moment it is opened, where the cursor should follow.
func (m *rowList[R, K]) SetActiveByKey(key K) {
	m.activeKey = key
	m.SetCursorByKey(key)
}

func (m *rowList[R, K]) SetCursorByKey(key K) {
	for i, r := range m.rows {
		if m.keyOf(r) == key {
			m.cursor = m.offset + i
			return
		}
	}
}

func (m *rowList[R, K]) hasKey(key K) bool {
	for _, r := range m.rows {
		if m.keyOf(r) == key {
			return true
		}
	}
	return false
}

func (m *rowList[R, K]) Focused() bool     { return m.focused }
func (m *rowList[R, K]) SetFocused(f bool) { m.focused = f }

// Height returns the pane's content height in rows.
func (m *rowList[R, K]) Height() int { return m.height }

// ScrollInfo reports the scroll position for the pane scrollbar. It reflects
// the whole list, not the window: the scrollbar is the user's sense of how much
// there is.
func (m *rowList[R, K]) ScrollInfo() components.ScrollInfo {
	if m.height <= 0 {
		return components.ScrollInfo{Total: m.total, Visible: m.total, Offset: 0}
	}
	return components.ScrollInfo{Total: m.total, Visible: m.height, Offset: m.viewStart()}
}

// CursorViewportRow returns the cursor's row index within the visible viewport.
func (m *rowList[R, K]) CursorViewportRow() int {
	if m.height <= 0 {
		return m.cursor
	}
	return m.cursor - m.viewStart()
}

// SetCursor moves the selection cursor to a whole-list index, clamped. It is a
// no-op when the list is empty.
func (m *rowList[R, K]) SetCursor(i int) {
	if m.total == 0 {
		return
	}
	m.cursor = i
	m.clampCursor()
}

// rowAtViewportRow maps a content row (0-based, within the visible viewport) to
// a row. ok is false when the viewport row holds none: past the end of the
// list, or inside a window the core has not sent yet.
func (m *rowList[R, K]) rowAtViewportRow(row int) (R, bool) {
	var zero R
	if row < 0 || m.total == 0 {
		return zero, false
	}
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	if row >= visible {
		return zero, false
	}
	return m.rowAt(m.viewStart() + row)
}

// indexAtViewportRow maps a viewport row to a whole-list index, for callers
// that move the cursor rather than read the row.
func (m *rowList[R, K]) indexAtViewportRow(row int) (int, bool) {
	if row < 0 || m.total == 0 {
		return 0, false
	}
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	if row >= visible {
		return 0, false
	}
	idx := m.viewStart() + row
	if idx >= m.total {
		return 0, false
	}
	return idx, true
}

// move applies a navigation action to the cursor and reports whether it was
// one. Confirming is the embedding list's own business.
func (m *rowList[R, K]) move(action keys.Action) bool {
	switch action {
	case keys.ActionDown:
		if m.cursor < m.total-1 {
			m.cursor++
		}
	case keys.ActionUp:
		if m.cursor > 0 {
			m.cursor--
		}
	case keys.ActionGoTop:
		m.cursor = 0
	case keys.ActionGoBottom:
		if m.total > 0 {
			m.cursor = m.total - 1
		}
	case keys.ActionScrollHalfDown:
		m.cursor += m.halfStep()
		m.clampCursor()
	case keys.ActionScrollHalfUp:
		m.cursor -= m.halfStep()
		m.clampCursor()
	default:
		return false
	}
	return true
}

func (m *rowList[R, K]) halfStep() int {
	step := m.height * 2 / 3
	if step < 1 {
		step = 1
	}
	return step
}

// visibleRange is the whole-list index range drawn in the viewport.
func (m *rowList[R, K]) visibleRange() (start, end int) {
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	start = m.viewStart()
	end = min(start+visible, m.total)
	return start, end
}
