package theme

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// The canvas cannot be applied by wrapping a composed view. Any line holding a
// coloured run carries an SGR reset in the middle of it, and everything wrapped
// around that loses the background from the reset onward. So it is baked into
// every run and every padded cell at the point the cells are created, which is
// what this file exists to make unavoidable.
//
// The rule that follows: a container may paint only what it adds itself — its
// border, its padding, its title. Content arrives already painted by whoever
// created it.

// NewStyle is the only style constructor under internal/ui. It returns a style
// carrying the current theme's canvas, so a style is opaque by construction and
// forgetting the background is not a thing that can happen. With the canvas
// unset it is byte-for-byte lipgloss.NewStyle, which is what keeps the built-ins
// rendering exactly as they always have.
//
// A test refuses lipgloss.NewStyle anywhere but here: the discipline this
// replaces is one that would have to be remembered on every style written from
// now on, and a hole in the canvas is visible only to whoever happens to be
// looking at the right screen state.
func NewStyle() lipgloss.Style { return newStyle(*T()) }

// newStyle takes the theme explicitly, for the one caller that cannot read the
// current one: buildStyles runs inside Apply, before the snapshot it belongs to
// has been stored.
func newStyle(t Theme) lipgloss.Style {
	if isNone(t.Background) {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(t.Background)
}

// Pad returns n spaces carrying the canvas. It replaces strings.Repeat(" ", n)
// everywhere the app pads a row out to a width — the interior of every panel,
// the gap a stamped overlay leaves behind it — which together are the largest
// area of colour on screen and are emitted by no style at all.
//
// With the canvas unset the result is exactly strings.Repeat(" ", n).
func Pad(n int) string {
	if n <= 0 {
		return ""
	}
	s := current.Load()
	if s.padPrefix == "" && s.padSuffix == "" {
		// No canvas: hand back the spaces themselves rather than a concatenation
		// of them with two empty strings. Padding runs once per row of every
		// panel and every bubble, so the copy this saves is not nothing.
		return spaces(n)
	}
	return s.padPrefix + spaces(n) + s.padSuffix
}

// spaceRun is sliced rather than repeated for the widths padding actually uses.
// A terminal wider than this is padded the slow way; there is no correctness
// difference, only an allocation.
const spaceRun = "                                                                " +
	"                                                                " +
	"                                                                " +
	"                                                                "

func spaces(n int) string {
	if n <= len(spaceRun) {
		return spaceRun[:n]
	}
	return strings.Repeat(" ", n)
}

// PadTo returns the spaces that carry a line of visible width w out to width, or
// nothing when it already reaches. It is the shape the padding call sites
// actually have, and putting the comparison here keeps a bare Repeat from being
// the obvious way to write it.
func PadTo(w, width int) string { return Pad(width - w) }

// padSGR is the escape pair lipgloss puts around a canvas-coloured run,
// extracted once per Apply. It is derived by rendering rather than assembled by
// hand so that it stays whatever lipgloss emits, including nothing at all when
// the canvas is unset.
func padSGR(t Theme) (prefix, suffix string) {
	const probe = " "
	out := newStyle(t).Render(probe)
	i := strings.Index(out, probe)
	if i < 0 {
		// Unreachable unless lipgloss stops emitting what it was given. Padding
		// without the canvas is a visible hole; padding of the wrong width is a
		// broken layout, so this fails towards the lesser one.
		return "", ""
	}
	return out[:i], out[i+len(probe):]
}
