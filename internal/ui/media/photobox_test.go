package media_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

// Large terminal, tall portrait: height bounded by the long-side px cap.
func TestPhotoBox_LargeTerminalCapsLongSidePx(t *testing.T) {
	// maxLongSidePx = 480, cellH = 32px -> 480/32 = 15 row ceiling. viewHeight
	// large so it is inert.
	cols, rows := media.PhotoBox(1000, 2000, 60, 100, 480, 16, 32, 2.0)
	require.LessOrEqual(t, rows, 15, "height capped at 480px-equivalent")
	require.LessOrEqual(t, cols, 30, "width capped at 480/16=30")
	require.Greater(t, cols, 0)
}

// Short pane, unknown cell size: height bounded by 2/3 of the viewport.
func TestPhotoBox_ShortPaneCapsToViewportFraction(t *testing.T) {
	cols, rows := media.PhotoBox(100, 300, 60, 12, 480, 0, 0, 2.0)
	require.LessOrEqual(t, rows, 12*2/3, "height capped at 2/3 viewport")
	require.Greater(t, cols, 0)
	require.Greater(t, rows, 0)
}

// Wide landscape on a px-reporting terminal: width bounded by the long-side cap.
func TestPhotoBox_LandscapeCapsWidthPx(t *testing.T) {
	cols, _ := media.PhotoBox(4000, 1000, 60, 100, 480, 16, 32, 2.0)
	require.LessOrEqual(t, cols, 30, "width capped at 480/16=30")
}

// Unknown cell size, image fits: width is the full budget, no px cap applied.
func TestPhotoBox_UnknownCellSizeUsesBudget(t *testing.T) {
	cols, rows := media.PhotoBox(100, 100, 60, 100, 480, 0, 0, 2.0)
	require.Equal(t, 60, cols, "no px cap -> full width budget")
	require.Equal(t, 30, rows, "round(60*1/2)")
}

// A larger maxLongSidePx allows a larger box (config raises the ceiling).
func TestPhotoBox_HigherLongSideAllowsLargerBox(t *testing.T) {
	c480, r480 := media.PhotoBox(1000, 2000, 60, 100, 480, 16, 32, 2.0)
	c600, r600 := media.PhotoBox(1000, 2000, 60, 100, 600, 16, 32, 2.0)
	require.Greater(t, r600, r480, "higher px cap -> taller box")
	require.GreaterOrEqual(t, c600, c480, "higher px cap -> at least as wide")
}

// Non-positive maxLongSidePx falls back to the package default (800).
func TestPhotoBox_NonPositiveUsesDefault(t *testing.T) {
	cZero, rZero := media.PhotoBox(1000, 2000, 60, 100, 0, 16, 32, 2.0)
	c800, r800 := media.PhotoBox(1000, 2000, 60, 100, 800, 16, 32, 2.0)
	require.Equal(t, c800, cZero, "0 falls back to default 800")
	require.Equal(t, r800, rZero, "0 falls back to default 800")
}

// Downscale only: never exceeds the un-capped box.
func TestPhotoBox_DownscaleOnly(t *testing.T) {
	maxCols := 60
	cols, rows := media.PhotoBox(800, 1600, maxCols, 40, 480, 12, 24, 2.0)
	require.LessOrEqual(t, cols, maxCols)
	require.LessOrEqual(t, rows, media.PhotoRows(800, 1600, maxCols, 2.0))
}
