package media

import (
	"sync"
	"sync/atomic"
)

// defaultCellAspect is the cell height:width ratio assumed when the terminal
// does not report its pixel size. Half-block art is built around this 2:1
// approximation; it is also a safe fallback for the Kitty path.
const defaultCellAspect = 2.0

var (
	cellPxOnce sync.Once
	cellPxW    float64
	cellPxH    float64

	// cellPxReported is the cell size the terminal answered a CSI 16 t query
	// with. It wins over the ioctl probe below because that probe is blind on
	// terminals which leave the pixel fields of TIOCGWINSZ empty - foot is one,
	// and foot is the terminal sixel exists for here.
	cellPxReported atomic.Pointer[cellPxPair]
)

// cellPxPair is the reported cell size. A pointer, so the atomic swap is one
// word and readers never see half an update.
type cellPxPair struct{ w, h float64 }

// SetCellPx records the cell pixel size reported by the terminal (a CSI 16 t
// reply, surfaced by bubbletea as a CellSizeEvent) and returns whether it
// changed anything. A non-positive size means the terminal answered nothing:
// the earlier report is dropped and the ioctl probe becomes the source again.
func SetCellPx(w, h float64) bool {
	if w <= 0 || h <= 0 {
		if cellPxReported.Load() == nil {
			return false
		}
		cellPxReported.Store(nil)
		return true
	}
	if prev := cellPxReported.Load(); prev != nil && prev.w == w && prev.h == h {
		return false
	}
	cellPxReported.Store(&cellPxPair{w: w, h: h})
	return true
}

// CellPx returns the terminal's real cell pixel width and height: the size it
// reported, or else one detected from the controlling TTY, or (0,0) when
// neither is available. Used to transmit images at the reserved box's true
// pixel size so terminals that do not upscale Unicode-placeholder images still
// fill the box, and to size a sixel to the cells it covers.
func CellPx() (float64, float64) {
	if p := cellPxReported.Load(); p != nil {
		return p.w, p.h
	}
	cellPxOnce.Do(func() {
		cellPxW, cellPxH = detectCellPx()
	})
	return cellPxW, cellPxH
}

// CellAspect returns the terminal's real cell aspect ratio (height/width),
// falling back to defaultCellAspect when the terminal does not report pixel
// dimensions. The aspect follows the same source as CellPx, so a terminal that
// only answers the query gets the true ratio rather than the 2:1 guess.
func CellAspect() float64 {
	if w, h := CellPx(); w > 0 && h > 0 {
		return h / w
	}
	return defaultCellAspect
}
