//go:build unix

package media

import (
	"os"

	"golang.org/x/sys/unix"
)

// detectCellPx returns the terminal's reported cell pixel width and height, or
// (0,0) when unavailable. Terminals that answer no DSR are covered by this
// probe; foot answers CSI 16 t instead and leaves these fields empty.
func detectCellPx() (float64, float64) {
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err != nil || ws == nil {
			continue
		}
		if ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
			continue
		}
		return float64(ws.Xpixel) / float64(ws.Col), float64(ws.Ypixel) / float64(ws.Row)
	}
	return 0, 0
}
