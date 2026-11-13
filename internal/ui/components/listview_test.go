package components_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// visible is the indexes of the rows a window of maxRows shows.
func visible(l *components.ListView, maxRows int) []int {
	var out []int
	l.Render(maxRows, func(i int, _ bool) string {
		out = append(out, i)
		return ""
	})
	return out
}

// Going down, the cursor walks to the second-to-last row on screen, and only
// then does the list move, a row at a time; at the end of the list it reaches
// the last row.
func TestListView_Render_ScrollsDownFromTheSecondToLastRow(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(10)

	for range 4 {
		l.MoveDown()
	}
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, visible(l, 6), "the cursor on row 4 of 0-5 moves nothing")

	l.MoveDown()
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, visible(l, 6), "past the second-to-last row the list moves by one")

	for range 4 {
		l.MoveDown()
	}
	assert.Equal(t, 9, l.Cursor())
	assert.Equal(t, []int{4, 5, 6, 7, 8, 9}, visible(l, 6), "at the end the cursor reaches the last row")
}

// Going up is the mirror: the cursor walks to the second row on screen before
// the list moves, and reaches the first row at the top of the list.
func TestListView_Render_ScrollsUpFromTheSecondRow(t *testing.T) {
	l := components.NewListView(false)
	l.SetCount(10)
	l.SetCursor(9)
	require.Equal(t, []int{4, 5, 6, 7, 8, 9}, visible(l, 6))

	for range 4 {
		l.MoveUp()
	}
	assert.Equal(t, []int{4, 5, 6, 7, 8, 9}, visible(l, 6), "the cursor on row 5 of 4-9 moves nothing")

	l.MoveUp()
	assert.Equal(t, []int{3, 4, 5, 6, 7, 8}, visible(l, 6), "past the second row the list moves by one")

	for range 4 {
		l.MoveUp()
	}
	assert.Equal(t, 0, l.Cursor())
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5}, visible(l, 6), "at the top the cursor reaches the first row")
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
