package components

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScrollInfo_Thumb_NoOverflow(t *testing.T) {
	_, _, show := ScrollInfo{Total: 5, Visible: 10, Offset: 0}.Thumb(10)
	assert.False(t, show, "content fits, no thumb")

	_, _, show = ScrollInfo{Total: 10, Visible: 10, Offset: 0}.Thumb(10)
	assert.False(t, show, "exact fit, no thumb")
}

func TestScrollInfo_Thumb_TopAndBottom(t *testing.T) {
	// 100 rows of content, 10 visible, track 10 tall.
	top := ScrollInfo{Total: 100, Visible: 10, Offset: 0}
	start, size, show := top.Thumb(10)
	assert.True(t, show)
	assert.Equal(t, 0, start, "at top thumb starts at row 0")
	assert.Equal(t, 1, size, "10/100 of 10 rows = 1")

	bot := ScrollInfo{Total: 100, Visible: 10, Offset: 90}
	start, size, show = bot.Thumb(10)
	assert.True(t, show)
	assert.Equal(t, 9, start+size-1, "at bottom thumb ends at last track row")
}

func TestScrollInfo_Thumb_MinSizeAndClamp(t *testing.T) {
	// Huge content: size rounds to 0 but must clamp to >= 1.
	_, size, show := ScrollInfo{Total: 1000, Visible: 5, Offset: 0}.Thumb(10)
	assert.True(t, show)
	assert.GreaterOrEqual(t, size, 1)

	// Offset beyond max is clamped to bottom.
	start, size, _ := ScrollInfo{Total: 100, Visible: 10, Offset: 9999}.Thumb(10)
	assert.LessOrEqual(t, start+size, 10, "thumb never exceeds track")
	assert.Equal(t, 10, start+size, "clamped offset lands at bottom")
}

func TestScrollInfo_Thumb_ZeroTrack(t *testing.T) {
	_, _, show := ScrollInfo{Total: 100, Visible: 10, Offset: 0}.Thumb(0)
	assert.False(t, show)
}
