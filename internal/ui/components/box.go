package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderBox renders a bordered box with an optional top title, top suffix (right of title),
// bottom hint (left of bottom border), and bottom suffix (right of bottom border).
// w and h are outer dimensions (including 1-char border on each side).
// topSuffix is a pre-styled string placed after the title in the top border, separated
// by one border character and spaces on each side. Pass "" to omit.
// bottomHint is rendered verbatim (callers pre-style it, e.g. via OverlayHint).
// bottomSuffix is a pre-styled string right-anchored on the bottom border (symmetric to
// topSuffix). Pass "" to omit; when it does not fit, the bottom border falls back to plain.
// borderFg sets the border foreground color; nil means no color.
func RenderBox(content, topTitle, topSuffix, bottomHint, bottomSuffix string, b lipgloss.Border, borderFg color.Color, w, h int, scrollbar ...*Scrollbar) string {
	innerW := w - 2
	innerH := h - 2

	var sb *Scrollbar
	if len(scrollbar) > 0 {
		sb = scrollbar[0]
	}
	thumbStart, thumbSize, showThumb := 0, 0, false
	if sb != nil {
		thumbStart, thumbSize, showThumb = sb.Info.Thumb(sb.TrackLen)
	}

	cb := func(s string) string { return s }
	if borderFg != nil {
		bs := lipgloss.NewStyle().Foreground(borderFg)
		cb = func(s string) string { return bs.Render(s) }
	}

	var top string
	if topTitle != "" {
		titleStr := " " + topTitle + " "
		titleW := lipgloss.Width(titleStr)
		fillW := innerW - titleW
		if fillW >= 2 {
			if topSuffix != "" {
				suffixW := lipgloss.Width(topSuffix)
				remaining := fillW - suffixW - 4
				if remaining >= 0 {
					top = cb(b.TopLeft+b.Top) + titleStr + cb(b.Top) + " " + topSuffix + " " + cb(strings.Repeat(b.Top, remaining)+b.TopRight)
				} else {
					top = cb(b.TopLeft+b.Top) + titleStr + cb(strings.Repeat(b.Top, fillW-1)+b.TopRight)
				}
			} else {
				top = cb(b.TopLeft+b.Top) + titleStr + cb(strings.Repeat(b.Top, fillW-1)+b.TopRight)
			}
		} else {
			top = cb(b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight)
		}
	} else {
		top = cb(b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight)
	}

	var bot string
	switch {
	case bottomHint == "" && bottomSuffix == "":
		bot = cb(b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight)
	case bottomSuffix == "":
		// Hint only — unchanged existing behavior (left-aligned after one border char).
		hintStr := " " + bottomHint + " "
		hintW := lipgloss.Width(hintStr)
		fillW := innerW - hintW
		if fillW >= 2 {
			bot = cb(b.BottomLeft+b.Bottom) + hintStr + cb(strings.Repeat(b.Bottom, fillW-1)+b.BottomRight)
		} else {
			bot = cb(b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight)
		}
	default:
		// Suffix present (optionally with a left hint): suffix hugs the right corner.
		leftStr := ""
		if bottomHint != "" {
			leftStr = " " + bottomHint + " "
		}
		rightStr := " " + bottomSuffix + " "
		// One leading border char after the corner, then fill, then the labels.
		fillW := innerW - 1 - lipgloss.Width(leftStr) - lipgloss.Width(rightStr)
		if fillW >= 1 {
			bot = cb(b.BottomLeft+b.Bottom) + leftStr + cb(strings.Repeat(b.Bottom, fillW)) + rightStr + cb(b.BottomRight)
		} else {
			bot = cb(b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight)
		}
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
	for ri, l := range lines {
		lw := lipgloss.Width(l)
		if lw < innerW {
			l += strings.Repeat(" ", innerW-lw)
		}
		rightChar := b.Right
		if showThumb && sb != nil && ri >= sb.TrackTop && ri < sb.TrackTop+sb.TrackLen {
			tr := ri - sb.TrackTop
			if tr >= thumbStart && tr < thumbStart+thumbSize {
				rightChar = scrollThumbChar
			}
		}
		result = append(result, cb(b.Left)+l+cb(rightChar))
	}
	result = append(result, bot)
	return strings.Join(result, "\n")
}
