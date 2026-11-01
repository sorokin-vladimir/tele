package media

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSetCellPx checks the override a terminal's CSI 16 t answer installs. It
// is the only source of cell pixels on foot, which leaves the pixel fields of
// TIOCGWINSZ empty.
func TestSetCellPx(t *testing.T) {
	t.Cleanup(func() { SetCellPx(0, 0) })

	assert.True(t, SetCellPx(9, 21), "a report from the terminal is a change")
	w, h := CellPx()
	assert.Equal(t, 9.0, w)
	assert.Equal(t, 21.0, h)
	assert.InDelta(t, 21.0/9.0, CellAspect(), 1e-9,
		"the aspect follows the reported cell, not the 2:1 guess")

	assert.False(t, SetCellPx(9, 21), "the same size again is not a change")

	assert.True(t, SetCellPx(0, 0), "a reply of zeros drops the report")
	w, h = CellPx()
	want := defaultCellAspect
	if w > 0 && h > 0 {
		// `go test` run from a terminal leaves the ioctl probe something real
		// to find; the point is that the guess only stands when it finds
		// nothing.
		want = h / w
	}
	assert.InDelta(t, want, CellAspect(), 1e-9,
		"with no report the ioctl probe (or the guess) is back in charge")
	assert.False(t, SetCellPx(0, 0), "nothing to drop is not a change")
}
