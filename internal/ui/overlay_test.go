package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOverlayBottomRight_PlacesOverlayAtBottomRight(t *testing.T) {
	// 20×6 base; overlay is 3 lines × 5 cols
	base := strings.Repeat(strings.Repeat(".", 20)+"\n", 5) + strings.Repeat(".", 20)
	overlay := "AAAAA\nBBBBB\nCCCCC"
	result := overlayBottomRight(base, overlay, 20, 6, 0)
	lines := strings.Split(result, "\n")

	// overlayH=3, overlayW=5
	// top = 6 - 3 - 1 = 2
	// left = 20 - 5 - 2 = 13
	assert.Equal(t, 6, len(lines), "line count preserved")
	assert.Contains(t, lines[2], "AAAAA", "first overlay row at top=2")
	assert.Contains(t, lines[3], "BBBBB")
	assert.Contains(t, lines[4], "CCCCC")
}

func TestOverlayBottomRight_SingleLine(t *testing.T) {
	base := strings.Repeat(strings.Repeat(" ", 10)+"\n", 4) + strings.Repeat(" ", 10)
	overlay := "XYZ"
	result := overlayBottomRight(base, overlay, 10, 5, 0)
	lines := strings.Split(result, "\n")
	// overlayH=1, top = 5-1-1 = 3; overlayW=3, left = 10-3-2 = 5
	assert.Equal(t, 5, len(lines))
	assert.Contains(t, lines[3], "XYZ")
}
