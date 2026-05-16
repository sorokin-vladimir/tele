package ui

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"charm.land/lipgloss/v2"
)

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

	for i, mLine := range overlayLines {
		row := top + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		bLine := baseLines[row]
		bLineW := lipgloss.Width(bLine)
		if bLineW < left {
			bLine += strings.Repeat(" ", left-bLineW)
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

	return strings.Join(baseLines, "\n")
}
