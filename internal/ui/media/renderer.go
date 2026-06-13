package media

import "image"

// Renderer turns a decoded photo into terminal lines for one chat bubble.
// A nil result means "not renderable yet" (e.g. a Kitty image still being
// transmitted); the caller should fall back to the text placeholder.
type Renderer interface {
	// Render returns one terminal line per row. photoID is a stable key used
	// for caching and (for Kitty) image-id mapping. cols is the target width.
	Render(photoID int64, img image.Image, cols int) []string
	// Reset drops any cached output (call on width change).
	Reset()
}

// blockKey keys a rendered result by photo and target width.
type blockKey struct {
	photoID int64
	cols    int
}
