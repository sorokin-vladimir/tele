//go:build !unix

package media

// detectCellAspect has no portable implementation off unix; callers fall back
// to defaultCellAspect.
func detectCellAspect() float64 { return 0 }

// detectCellPx has no portable implementation off unix.
func detectCellPx() (float64, float64) { return 0, 0 }
