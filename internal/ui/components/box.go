package components

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// RenderBox renders a bordered box with an optional top title and bottom hint.
// w and h are outer dimensions (including 1-char border on each side).
// Empty title or hint results in a plain border on that side.
// bottomHint is rendered in a dim color.
func RenderBox(content, topTitle, bottomHint string, b lipgloss.Border, w, h int) string {
	innerW := w - 2
	innerH := h - 2

	var top string
	if topTitle != "" {
		titleStr := " " + topTitle + " "
		titleW := lipgloss.Width(titleStr)
		fillW := innerW - titleW
		if fillW >= 2 {
			top = b.TopLeft + b.Top + titleStr + strings.Repeat(b.Top, fillW-1) + b.TopRight
		} else {
			top = b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight
		}
	} else {
		top = b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight
	}

	var bot string
	if bottomHint != "" {
		hintStr := " " + bottomHint + " "
		hintW := lipgloss.Width(hintStr)
		fillW := innerW - hintW
		if fillW >= 2 {
			bot = b.BottomLeft + b.Bottom + hintStyle.Render(hintStr) + strings.Repeat(b.Bottom, fillW-1) + b.BottomRight
		} else {
			bot = b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight
		}
	} else {
		bot = b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight
	}

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	result := make([]string, 0, innerH+2)
	result = append(result, top)
	for _, l := range lines {
		lw := lipgloss.Width(l)
		if lw < innerW {
			l += strings.Repeat(" ", innerW-lw)
		}
		result = append(result, b.Left+l+b.Right)
	}
	result = append(result, bot)
	return strings.Join(result, "\n")
}
