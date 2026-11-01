//go:build !unix

package media

// detectCellPx has no portable implementation off unix; callers fall back to
// the default aspect and the sixel fallback cell size.
func detectCellPx() (float64, float64) { return 0, 0 }
