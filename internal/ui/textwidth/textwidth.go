// Package textwidth centralizes the app's direct text measurement, wrapping and
// slicing in terminal cells. It follows go-runewidth: a grapheme cluster is as
// wide as its first non-zero-width rune, so a base glyph followed by a variation
// selector (✍️, ❤️) counts as the base glyph alone.
//
// That matches the terminal this app runs in, which advances those sequences by
// the base glyph's width. The Unicode emoji-presentation rule (VS16 makes a
// sequence two cells) does not match it, and measuring by that rule left the
// right border one cell short on such rows.
//
// The rule is terminal-dependent, not a property of the text. Terminals that
// widen VS16 sequences will draw these rows one cell wider than measured. The
// real fix is to measure the running terminal (see #235); until then, every
// direct width measurement, wrap and slice goes through this package so the rule
// can change in one place.
package textwidth

import (
	"bytes"
	"io"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// Width returns the cell width of s. ANSI escape sequences are ignored.
func Width(s string) int {
	return xansi.StringWidthWc(s)
}

// Wrap breaks s into lines no wider than limit cells, at word boundaries where
// possible. It returns s unchanged when limit is below 1. Lines are not padded.
//
// The style in effect at a break is re-applied on the next line, as lipgloss
// does. Without that, a painted run that wraps loses its colour from the second
// row on.
func Wrap(s string, limit int) string {
	if limit < 1 {
		return s
	}
	return replayStyles(xansi.WrapWc(s, limit, ""))
}

// Wordwrap breaks s into lines no wider than limit cells without breaking word
// boundaries, using breakpoints in addition to spaces. It returns s unchanged
// when limit is below 1. Lines are not padded.
//
// As with Wrap, the style in effect at a break is re-applied on the next line.
func Wordwrap(s string, limit int, breakpoints string) string {
	if limit < 1 {
		return s
	}
	return replayStyles(xansi.WordwrapWc(s, limit, breakpoints))
}

// replayStyles writes an already-wrapped string through lipgloss's wrap writer,
// which closes the style at each newline and re-applies it on the next row.
func replayStyles(s string) string {
	var buf bytes.Buffer
	w := lipgloss.NewWrapWriter(&buf)
	_, _ = io.WriteString(w, s)
	_ = w.Close()
	return buf.String()
}

// Truncate cuts s to at most limit cells, appending tail when it must cut.
// ANSI escape sequences are ignored when measuring and are not broken.
func Truncate(s string, limit int, tail string) string {
	return xansi.TruncateWc(s, limit, tail)
}

// TruncateLeft removes the first n cells of s, adding prefix when it must cut.
// ANSI escape sequences are ignored when measuring and are not broken.
func TruncateLeft(s string, n int, prefix string) string {
	return xansi.TruncateLeftWc(s, n, prefix)
}

// Cut returns the cells of s in [left, right). ANSI escape sequences are
// ignored when measuring and are not broken.
func Cut(s string, left, right int) string {
	if right <= left {
		return ""
	}
	return TruncateLeft(Truncate(s, right, ""), left, "")
}
