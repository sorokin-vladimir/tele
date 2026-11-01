package media

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPaletteBoundsStartAtActualEntries(t *testing.T) {
	// Red has a high offset but no spread; green is the axis to split.
	entries := []paletteEntry{{r5: 29, g5: 2, b5: 10, count: 1}, {r5: 30, g5: 20, b5: 12, count: 2}}
	box := newPaletteBox(entries)
	require.Equal(t, 29, box.rmin)
	require.Equal(t, 2, box.gmin)
	require.Equal(t, 10, box.bmin)
	require.Equal(t, 18, box.widestRange())
	left, right := box.split()
	require.Equal(t, 2, left.entries[0].g5)
	require.Equal(t, 20, right.entries[0].g5)
}
