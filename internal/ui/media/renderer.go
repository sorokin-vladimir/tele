package media

import "image"

// Renderer turns a decoded photo into terminal lines for one chat bubble.
// A nil result means "not renderable yet" (e.g. a Kitty image still being
// transmitted); the caller should fall back to the text placeholder.
type Renderer interface {
	// Render returns one terminal line per row. photoID is a stable key used
	// for caching and (for Kitty) image-id mapping. cols is the target width.
	Render(photoID int64, img image.Image, cols int) []string
	// RenderWindow renders a centered sub-rectangle of an image transmitted at
	// coverCols x coverRows: winRows lines of winCols cells starting at (hOff,
	// vOff). Used to crop a mosaic tile to its cover window. A nil result means
	// "not renderable yet" (Kitty still transmitting), like Render.
	RenderWindow(photoID int64, img image.Image, coverCols, coverRows, hOff, vOff, winCols, winRows int) []string
	// Reset drops any cached output (call on width change).
	Reset()
}

// blockKey keys a rendered result by photo and target width, and for Kitty by
// the image id drawn, which an animated photo changes at every frame (#331).
type blockKey struct {
	photoID int64
	cols    int
	kittyID uint32
}
