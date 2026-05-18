package ui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"charm.land/lipgloss/v2"
)

// stampOverlay writes overlayLines into baseLines starting at (top, left).
func stampOverlay(baseLines, overlayLines []string, top, left int) {
	for i, mLine := range overlayLines {
		row := top + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		bLine := baseLines[row]
		if lipgloss.Width(bLine) < left {
			bLine += strings.Repeat(" ", left-lipgloss.Width(bLine))
		}
		mLineW := lipgloss.Width(mLine)
		right := left + mLineW
		prefix := xansi.Truncate(bLine, left, "")
		suffix := ""
		if lipgloss.Width(bLine) > right {
			suffix = xansi.TruncateLeft(bLine, right, "")
		}
		baseLines[row] = prefix + mLine + suffix
	}
}

// overlayCenter stamps overlay string centered over base string (w×h terminal).
func overlayCenter(base, overlay string, w, h int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayH := len(overlayLines)
	overlayW := 0
	for _, l := range overlayLines {
		if ww := lipgloss.Width(l); ww > overlayW {
			overlayW = ww
		}
	}

	top := (h - overlayH) / 2
	left := (w - overlayW) / 2
	if left < 0 {
		left = 0
	}

	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	stampOverlay(baseLines, overlayLines, top, left)

	return strings.Join(baseLines, "\n")
}

// overlayBottomRight stamps overlay string in the bottom-right corner of a
// base string (w×h terminal). bottomOffset shifts the overlay upward by that
// many additional rows (use to clear a bottom status bar or input area).
func overlayBottomRight(base, overlay string, w, h, bottomOffset int) string {
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayH := len(overlayLines)
	overlayW := 0
	for _, l := range overlayLines {
		if ww := lipgloss.Width(l); ww > overlayW {
			overlayW = ww
		}
	}

	top := h - overlayH - 1 - bottomOffset
	if top < 0 {
		top = 0
	}
	left := w - overlayW - 2
	if left < 0 {
		left = 0
	}

	for len(baseLines) < h {
		baseLines = append(baseLines, "")
	}

	stampOverlay(baseLines, overlayLines, top, left)

	return strings.Join(baseLines, "\n")
}
