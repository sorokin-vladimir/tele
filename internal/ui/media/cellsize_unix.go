//go:build unix

package media

import (
	"os"

	"golang.org/x/sys/unix"
)

// detectCellAspect reads the terminal cell pixel size via TIOCGWINSZ and returns
// the cell height:width ratio. Returns 0 when no controlling TTY reports pixel
// dimensions (e.g. under a pipe or a terminal that omits xpixel/ypixel).
func detectCellAspect() float64 {
	for _, f := range []*os.File{os.Stdout, os.Stdin, os.Stderr} {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err != nil || ws == nil {
			continue
		}
		if ws.Xpixel == 0 || ws.Ypixel == 0 || ws.Col == 0 || ws.Row == 0 {
			continue
		}
		cellW := float64(ws.Xpixel) / float64(ws.Col)
		cellH := float64(ws.Ypixel) / float64(ws.Row)
		if cellW > 0 && cellH > 0 {
			return cellH / cellW
		}
	}
	return 0
}

// detectCellPx returns the terminal's reported cell pixel width and height, or
// (0,0) when unavailable.
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
