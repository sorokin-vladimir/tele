package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
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
	topTitle = layout.PresentationText(topTitle)
	topSuffix = layout.PresentationText(topSuffix)
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

	// The border goes through a style whether or not it is coloured, so that the
	// canvas reaches the border cells either way. With no canvas and no border
	// colour this renders the argument unchanged, which is what it did before.
	bs := theme.NewStyle()
	if borderFg != nil {
		bs = bs.Foreground(borderFg)
	}
	cb := bs.Render

	// paint frames the title: the spaces around it are the box's own cells, and
	// the title between them is styled separately. Framing and text are split
	// because a caller may hand over a title that is already styled (the help
	// modal does) — wrapping the pair would lose the background after that
	// title's own reset, leaving the trailing space bare.
	paint := func(title string) string {
		return theme.Pad(1) + theme.S().Body.Render(title) + theme.Pad(1)
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
					top = cb(b.TopLeft+b.Top) + paint(topTitle) + cb(b.Top) + theme.Pad(1) + topSuffix + theme.Pad(1) + cb(strings.Repeat(b.Top, remaining)+b.TopRight)
				} else {
					top = cb(b.TopLeft+b.Top) + paint(topTitle) + cb(strings.Repeat(b.Top, fillW-1)+b.TopRight)
				}
			} else {
				top = cb(b.TopLeft+b.Top) + paint(topTitle) + cb(strings.Repeat(b.Top, fillW-1)+b.TopRight)
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
			// The hint arrives styled by the caller; only the spaces framing it
			// are the box's to paint.
			bot = cb(b.BottomLeft+b.Bottom) + theme.Pad(1) + bottomHint + theme.Pad(1) + cb(strings.Repeat(b.Bottom, fillW-1)+b.BottomRight)
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
			// Both labels arrive styled; the box paints only the spaces it puts
			// around them, which is why the widths above are measured on the
			// framed strings but the framing is emitted separately here.
			left := ""
			if bottomHint != "" {
				left = theme.Pad(1) + bottomHint + theme.Pad(1)
			}
			right := theme.Pad(1) + bottomSuffix + theme.Pad(1)
			bot = cb(b.BottomLeft+b.Bottom) + left + cb(strings.Repeat(b.Bottom, fillW)) + right + cb(b.BottomRight)
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
		// The pad goes after the line, which is after whatever reset the line
		// ends in — the only position from which a background survives. This is
		// the interior of every panel, and the largest area of colour on screen.
		l += theme.PadTo(lipgloss.Width(l), innerW)
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
