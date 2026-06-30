package components

// ListView is a dumb vertical list selector. It owns the cursor for a list of
// `count` items and derives the scroll window from the cursor at render time. It
// knows nothing about item content: rows are produced by a caller-supplied
// render function. wrap controls whether cursor movement wraps at the ends.
type ListView struct {
	cursor     int
	count      int
	wrap       bool
	selectable func(i int) bool // nil => every row is selectable
}

// NewListView creates a list selector. wrap enables wrap-around movement.
func NewListView(wrap bool) *ListView {
	return &ListView{wrap: wrap}
}

func (l *ListView) Cursor() int { return l.cursor }

// SetSelectable sets the predicate marking a row navigable. nil restores the
// default (all rows selectable). Set before SetCount so clamping respects it.
func (l *ListView) SetSelectable(fn func(i int) bool) { l.selectable = fn }

func (l *ListView) isSelectable(i int) bool {
	if i < 0 || i >= l.count {
		return false
	}
	if l.selectable == nil {
		return true
	}
	return l.selectable(i)
}

// SetCount records the item count and clamps the cursor to the nearest
// selectable index within [0,count).
func (l *ListView) SetCount(count int) {
	l.count = count
	if count == 0 {
		l.cursor = 0
		return
	}
	if l.cursor >= count {
		l.cursor = count - 1
	}
	if l.cursor < 0 {
		l.cursor = 0
	}
	l.cursor = l.nearestSelectable(l.cursor)
}

// SetCursor places the cursor at i (clamped to [0,count)); if i is not
// selectable the nearest selectable row is chosen.
func (l *ListView) SetCursor(i int) {
	if l.count == 0 {
		l.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= l.count {
		i = l.count - 1
	}
	l.cursor = l.nearestSelectable(i)
}

// nearestSelectable returns the closest selectable index to i (scanning forward
// then backward). Falls back to i when none qualify.
func (l *ListView) nearestSelectable(i int) int {
	if l.isSelectable(i) {
		return i
	}
	for d := 1; d < l.count; d++ {
		if j := i + d; j < l.count && l.isSelectable(j) {
			return j
		}
		if j := i - d; j >= 0 && l.isSelectable(j) {
			return j
		}
	}
	return i
}

// MoveDown/MoveUp advance the cursor to the next/previous selectable row,
// honoring wrap. No-op when there is no other selectable row.
func (l *ListView) MoveDown() { l.move(1) }
func (l *ListView) MoveUp()   { l.move(-1) }

func (l *ListView) move(dir int) {
	if l.count == 0 {
		return
	}
	if l.wrap {
		for d := 1; d < l.count; d++ {
			next := ((l.cursor+dir*d)%l.count + l.count) % l.count
			if l.isSelectable(next) {
				l.cursor = next
				return
			}
		}
		return
	}
	for next := l.cursor + dir; next >= 0 && next < l.count; next += dir {
		if l.isSelectable(next) {
			l.cursor = next
			return
		}
	}
}

// windowOffset returns the index of the first visible row for a window of
// maxRows: the cursor is centered and the window is clamped at the list ends.
func (l *ListView) windowOffset(maxRows int) int {
	if l.count <= maxRows || maxRows <= 0 {
		return 0
	}
	offset := l.cursor - maxRows/2
	if max := l.count - maxRows; offset > max {
		offset = max
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

// Render returns the rows of the visible window (length <= maxRows). When
// count > maxRows the window centers the cursor, clamped at the list ends.
// rowFn renders item i; selected is true when i == cursor.
func (l *ListView) Render(maxRows int, rowFn func(i int, selected bool) string) []string {
	if l.count == 0 || maxRows <= 0 {
		return nil
	}
	offset := l.windowOffset(maxRows)
	rows := make([]string, 0, maxRows)
	for i := offset; i < l.count && i < offset+maxRows; i++ {
		rows = append(rows, rowFn(i, i == l.cursor))
	}
	return rows
}

// visibleRows returns how many rows a window of maxRows actually shows.
func (l *ListView) visibleRows(maxRows int) int {
	if maxRows <= 0 {
		return 0
	}
	if l.count < maxRows {
		return l.count
	}
	return maxRows
}

// ScrollInfo describes the current scroll window for a maxRows viewport, in the
// units RenderBox's scrollbar expects.
func (l *ListView) ScrollInfo(maxRows int) ScrollInfo {
	return ScrollInfo{Total: l.count, Visible: l.visibleRows(maxRows), Offset: l.windowOffset(maxRows)}
}

// Scrollbar builds a *Scrollbar to hand to RenderBox: a track of the visible
// rows starting at trackTop (the box-content row index of the first list row).
// The thumb is hidden automatically when the content fits.
func (l *ListView) Scrollbar(maxRows, trackTop int) *Scrollbar {
	return &Scrollbar{Info: l.ScrollInfo(maxRows), TrackTop: trackTop, TrackLen: l.visibleRows(maxRows)}
}
