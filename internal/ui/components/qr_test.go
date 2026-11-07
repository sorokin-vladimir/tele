package components_test

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"rsc.io/qr"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

const loginURL = "tg://login?token=AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

// module reads back the module at column x and row y of a drawn QR code: each
// cell holds two rows, the upper half and the lower.
func module(t *testing.T, lines []string, x, y int) bool {
	t.Helper()
	cell := []rune(ansi.Strip(lines[y/2]))[x]
	upper, lower := cell == '█' || cell == '▀', cell == '█' || cell == '▄'
	require.Contains(t, []rune(" ▀▄█"), cell, "a cell of the code is one of four glyphs")
	if y%2 == 0 {
		return upper
	}
	return lower
}

// The QR code drawn in half blocks holds every module of the code, with a
// quiet zone of light modules around it, two rows to a line (#330).
func TestQRLines_DrawsEveryModule(t *testing.T) {
	code, err := qr.Encode(loginURL, qr.L)
	require.NoError(t, err)

	lines, err := components.QRLines(loginURL)
	require.NoError(t, err)

	side := code.Size + 2*components.QRQuiet
	require.Len(t, lines, (side+1)/2)
	for _, l := range lines {
		assert.Equal(t, side, ansi.StringWidth(l))
	}
	for y := range side {
		for x := range side {
			want := code.Black(x-components.QRQuiet, y-components.QRQuiet)
			assert.Equal(t, want, module(t, lines, x, y), "module %d,%d", x, y)
		}
	}
}

// A login token's URL fits a code a terminal of 80 by 24 can show beside its
// text.
func TestQRLines_ALoginTokenFitsAnOrdinaryTerminal(t *testing.T) {
	lines, err := components.QRLines(loginURL)
	require.NoError(t, err)

	assert.LessOrEqual(t, ansi.StringWidth(lines[0]), 40)
	assert.LessOrEqual(t, len(lines), 20)
}
