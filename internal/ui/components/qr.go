package components

import (
	"strings"

	"rsc.io/qr"

	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// QRQuiet is the light margin around a drawn QR code, in modules. The
// standard asks for four; two is what scanners read in practice, and on an
// 80 by 24 terminal the four would not leave room for the text beside it.
const QRQuiet = 2

// QRLines draws text as a QR code in half blocks, two rows of modules to a
// line, with a quiet zone around it. The code is the smallest that holds the
// text at the lowest level of error correction, which keeps it small enough
// for a terminal; a screen is not a crumpled label.
func QRLines(text string) ([]string, error) {
	code, err := qr.Encode(text, qr.L)
	if err != nil {
		return nil, err
	}
	side := code.Size + 2*QRQuiet
	dark := func(x, y int) bool { return code.Black(x-QRQuiet, y-QRQuiet) }
	lines := make([]string, 0, (side+1)/2)
	style := theme.QRStyle()
	var b strings.Builder
	for y := 0; y < side; y += 2 {
		b.Reset()
		for x := range side {
			upper, lower := dark(x, y), y+1 < side && dark(x, y+1)
			switch {
			case upper && lower:
				b.WriteRune('█')
			case upper:
				b.WriteRune('▀')
			case lower:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		lines = append(lines, style.Render(b.String()))
	}
	return lines, nil
}
