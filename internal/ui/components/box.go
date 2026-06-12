package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

var hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// RenderBox renders a bordered box with an optional top title, top suffix (right of title),
// and bottom hint. w and h are outer dimensions (including 1-char border on each side).
// topSuffix is a pre-styled string placed after the title in the top border, separated
// by one border character and spaces on each side. Pass "" to omit.
// bottomHint is rendered in a dim color.
// borderFg sets the border foreground color; nil means no color.
func RenderBox(content, topTitle, topSuffix, bottomHint string, b lipgloss.Border, borderFg color.Color, w, h int, scrollbar ...*Scrollbar) string {
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
	if bottomHint != "" {
		hintStr := " " + bottomHint + " "
		hintW := lipgloss.Width(hintStr)
		fillW := innerW - hintW
		if fillW >= 2 {
			bot = cb(b.BottomLeft+b.Bottom) + hintStyle.Render(hintStr) + cb(strings.Repeat(b.Bottom, fillW-1)+b.BottomRight)
		} else {
			bot = cb(b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight)
		}
	} else {
		bot = cb(b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight)
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
