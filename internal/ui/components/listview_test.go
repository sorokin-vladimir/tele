package components_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

func TestListView_SetCount_ClampsCursor(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(10)
	l.SetCursor(7)
	l.SetCount(3) // list shrank
	assert.Equal(t, 2, l.Cursor())
}

func TestListView_MoveDownUp_StopAtEnds_NoWrap(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(3)
	l.MoveUp()
	assert.Equal(t, 0, l.Cursor())
	l.MoveDown()
	l.MoveDown()
	l.MoveDown() // past end
	assert.Equal(t, 2, l.Cursor())
}

func TestListView_MoveWraps_WhenEnabled(t *testing.T) {
	l := components.NewListView(true)
	l.SetCount(3)
	l.MoveUp() // wrap to last
	assert.Equal(t, 2, l.Cursor())
	l.MoveDown() // wrap to first
	assert.Equal(t, 0, l.Cursor())
}

func TestListView_SkipsNonSelectable(t *testing.T) {
	l := components.NewListView(false)
	l.SetSelectable(func(i int) bool { return i != 1 }) // index 1 is a separator
	l.SetCount(3)
	l.MoveDown() // 0 -> skip 1 -> 2
	assert.Equal(t, 2, l.Cursor())
}

func TestListView_SetCount_LandsOnNearestSelectable(t *testing.T) {
	l := components.NewListView(false)
	l.SetSelectable(func(i int) bool { return i == 2 })
	l.SetCount(3)
	assert.Equal(t, 2, l.Cursor()) // 0 not selectable -> nearest is 2
}

func TestListView_Render_AllRowsWhenFits(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(3)
	rows := l.Render(8, func(i int, selected bool) string {
		if selected {
			return "*"
		}
		return "."
	})
	assert.Equal(t, []string{"*", ".", "."}, rows)
}

func TestListView_Render_CentersCursor_WhenScrolling(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(10)
	l.SetCursor(5)
	rows := l.Render(4, func(i int, selected bool) string {
		if selected {
			return "X"
		}
		return "o"
	})
	// maxRows=4, cursor=5 -> offset = 5 - 2 = 3 -> rows for i=3,4,5,6
	assert.Len(t, rows, 4)
	assert.Equal(t, "X", rows[2]) // cursor (i=5) is the 3rd visible row
}

func TestListView_Render_ClampsAtBottom(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(10)
	l.SetCursor(9)
	rows := l.Render(4, func(i int, selected bool) string {
		if selected {
			return "X"
		}
		return "o"
	})
	// offset clamped to 10-4 = 6 -> i=6,7,8,9; cursor=9 is last row
	assert.Len(t, rows, 4)
	assert.Equal(t, "X", rows[3])
}

func TestListView_Render_EmptyWhenNoItems(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(0)
	rows := l.Render(8, func(i int, selected bool) string { return "x" })
	assert.Empty(t, rows)
}

func TestListView_ScrollInfo_TracksWindow(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(20)
	si := l.ScrollInfo(8)
	assert.Equal(t, 20, si.Total)
	assert.Equal(t, 8, si.Visible)
	assert.Equal(t, 0, si.Offset, "cursor at top → offset 0")

	for i := 0; i < 19; i++ {
		l.MoveDown()
	}
	si = l.ScrollInfo(8)
	assert.Equal(t, 12, si.Offset, "cursor at bottom → offset clamps to Total-Visible")
}

func TestListView_Scrollbar_ShowsOnlyWhenOverflowing(t *testing.T) {
	big := components.NewListView(false)
	big.SetCount(20)
	sb := big.Scrollbar(8, 2)
	assert.Equal(t, 2, sb.TrackTop)
	assert.Equal(t, 8, sb.TrackLen)
	_, _, show := sb.Info.Thumb(sb.TrackLen)
	assert.True(t, show, "overflowing list must show a thumb")

	small := components.NewListView(false)
	small.SetCount(5)
	sb2 := small.Scrollbar(8, 0)
	_, _, show2 := sb2.Info.Thumb(sb2.TrackLen)
	assert.False(t, show2, "list that fits must not show a thumb")
}
