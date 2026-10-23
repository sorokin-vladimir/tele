package media

import (
	"image"
	"strconv"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

// Inline sixel support: the message list keeps drawing block art (so its layout
// and its height accounting are untouched) and each art row carries an
// invisible marker naming the image and the geometry it was drawn at. The root
// model reads those markers off the finished frame, which gives it the absolute
// cell of every inline image without threading geometry through the chat, the
// bubbles and the mosaics.
//
// The marker is an empty OSC 8 hyperlink: it occupies no cells, so it changes
// neither the layout nor the measurement of the line it is on, and a terminal
// that does not understand hyperlinks ignores it. It is written as a prefix of
// every art row because that is what makes the column recoverable - the prefix
// width of the marker is the image's first column - and the row number it
// carries is what lets a partially scrolled image be cropped instead of painted
// at the wrong offset.

const (
	// inlineMarkerOpen prefixes an inline-image marker. Everything up to
	// inlineMarkerClose is the payload; the empty URI that follows closes the
	// hyperlink so no cell is associated with it.
	inlineMarkerOpen  = "\x1b]8;;sixel-media/"
	inlineMarkerClose = "\x1b\\"
	// inlineMarkerDone closes the hyperlink the marker just opened. Two writes
	// rather than one because a link's URI cannot be empty.
	inlineMarkerDone = "\x1b]8;;\x1b\\"
)

// InlineRef is the geometry an inline marker describes: which image, how it was
// scaled, and which of its rows this marker is on.
type InlineRef struct {
	ID        int64
	CoverCols int // cells the image was scaled across before windowing
	CoverRows int
	HOff      int // window origin within the cover grid, in cells
	VOff      int
	WinCols   int // cells this window spans
	WinRows   int
	Row       int // row of this line within the window
}

// InlineMarker renders ref as an invisible prefix.
func InlineMarker(ref InlineRef) string {
	var sb strings.Builder
	sb.WriteString(inlineMarkerOpen)
	sb.WriteString(strconv.FormatInt(ref.ID, 10))
	for _, n := range []int{ref.CoverCols, ref.CoverRows, ref.HOff, ref.VOff, ref.WinCols, ref.WinRows, ref.Row} {
		sb.WriteByte('/')
		sb.WriteString(strconv.Itoa(n))
	}
	sb.WriteString(inlineMarkerClose)
	sb.WriteString(inlineMarkerDone)
	return sb.String()
}

// TruncateInlineLeft cuts cells from a marked art row, keeping the marker's
// horizontal crop consistent. Album badges replace the leading cells of the
// first row; retaining the full image width there would paint beyond the tile.
// Unmarked rows retain the ordinary ANSI-aware truncation behavior.
func TruncateInlineLeft(line string, cols int) string {
	if cols <= 0 {
		return line
	}
	ref, start, next, ok := ParseInlineMarker(line, 0)
	if !ok {
		return xansi.TruncateLeft(line, cols, "")
	}
	left := xansi.StringWidth(line[:start])
	drop := max(0, cols-left)
	if drop >= ref.WinCols {
		return xansi.TruncateLeft(line[:start]+line[next:], cols, "")
	}
	ref.HOff += drop
	ref.WinCols -= drop
	return xansi.TruncateLeft(line[:start], cols, "") + InlineMarker(ref) +
		xansi.TruncateLeft(line[next:], drop, "")
}

// ParseInlineMarker finds the next marker in line at or after from, returning
// the ref, the index the marker starts at, the index just past it and whether
// one was found.
func ParseInlineMarker(line string, from int) (ref InlineRef, start, next int, ok bool) {
	if from < 0 || from >= len(line) {
		return InlineRef{}, 0, 0, false
	}
	i := strings.Index(line[from:], inlineMarkerOpen)
	if i < 0 {
		return InlineRef{}, 0, 0, false
	}
	at := from + i
	start = at + len(inlineMarkerOpen)
	end := strings.Index(line[start:], inlineMarkerClose)
	if end < 0 {
		return InlineRef{}, 0, 0, false
	}
	next = start + end + len(inlineMarkerClose)
	if !strings.HasPrefix(line[next:], inlineMarkerDone) {
		return InlineRef{}, at, next, false
	}
	next += len(inlineMarkerDone)
	fields := strings.Split(line[start:start+end], "/")
	if len(fields) != 8 {
		return InlineRef{}, at, next, false
	}
	nums := make([]int, 7)
	for k := 0; k < 7; k++ {
		n, err := strconv.Atoi(fields[k+1])
		if err != nil {
			return InlineRef{}, at, next, false
		}
		nums[k] = n
	}
	id, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return InlineRef{}, at, next, false
	}
	if nums[0] <= 0 || nums[1] <= 0 || nums[2] < 0 || nums[3] < 0 ||
		nums[4] <= 0 || nums[5] <= 0 || nums[4] > nums[0] || nums[5] > nums[1] ||
		nums[2] > nums[0]-nums[4] || nums[3] > nums[1]-nums[5] ||
		nums[6] < 0 || nums[6] >= nums[5] {
		return InlineRef{}, at, next, false
	}
	return InlineRef{
		ID:        id,
		CoverCols: nums[0], CoverRows: nums[1],
		HOff: nums[2], VOff: nums[3],
		WinCols: nums[4], WinRows: nums[5],
		Row: nums[6],
	}, at, next, true
}

// SixelRenderer is the inline renderer for sixel mode: the same block art every
// terminal gets, tagged with a marker per row so the root model can paint the
// real image over it after the frame.
type SixelRenderer struct {
	block *BlockRenderer
}

// NewSixelRenderer returns an inline renderer that draws block art and marks it.
func NewSixelRenderer() *SixelRenderer {
	return &SixelRenderer{block: NewBlockRenderer()}
}

// Render draws the image as block art at cols cells wide and marks every row.
func (r *SixelRenderer) Render(photoID int64, img image.Image, cols int) []string {
	art := r.block.Render(photoID, img, cols)
	if len(art) == 0 {
		return art
	}
	ref := InlineRef{ID: photoID, CoverCols: cols, CoverRows: len(art), WinCols: cols, WinRows: len(art)}
	marked := make([]string, len(art))
	for i, line := range art {
		ref.Row = i
		marked[i] = InlineMarker(ref) + line
	}
	return marked
}

// RenderWindow draws a window of a cover-scaled image and marks it with the
// window's geometry, so a mosaic tile can be painted as the crop it is.
func (r *SixelRenderer) RenderWindow(photoID int64, img image.Image, coverCols, coverRows, hOff, vOff, winCols, winRows int) []string {
	art := r.block.RenderWindow(photoID, img, coverCols, coverRows, hOff, vOff, winCols, winRows)
	if len(art) == 0 {
		return art
	}
	ref := InlineRef{
		ID: photoID, CoverCols: coverCols, CoverRows: coverRows,
		HOff: hOff, VOff: vOff,
		WinCols: winCols, WinRows: winRows,
	}
	marked := make([]string, len(art))
	for i, line := range art {
		ref.Row = i
		marked[i] = InlineMarker(ref) + line
	}
	return marked
}

// Reset drops the block-art cache (the caller resizes the list).
func (r *SixelRenderer) Reset() { r.block.Reset() }

// SixelRegionSeq encodes the part of an inline image that is on screen: the
// window ref describes, starting skipTop rows down, visible rows tall. The crop
// happens in source pixels so the painted pixels line up with the rows they
// replace rather than being squeezed into the visible ones.
func SixelRegionSeq(img image.Image, ref InlineRef, skipTop, visible int) (string, error) {
	if img == nil || ref.CoverCols <= 0 || ref.CoverRows <= 0 || ref.WinCols <= 0 || visible <= 0 {
		return "", ErrEmptyImage
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return "", ErrEmptyImage
	}
	sx := b.Min.X + ref.HOff*w/ref.CoverCols
	sw := ref.WinCols * w / ref.CoverCols
	sy := b.Min.Y + (ref.VOff+skipTop)*h/ref.CoverRows
	sh := visible * h / ref.CoverRows
	// The scaling above truncates; keep at least one pixel in each direction and
	// stay inside the image.
	if sw < 1 {
		sw = 1
	}
	if sh < 1 {
		sh = 1
	}
	if sx+sw > b.Max.X {
		sw = b.Max.X - sx
	}
	if sy+sh > b.Max.Y {
		sh = b.Max.Y - sy
	}
	crop := image.Rect(sx, sy, sx+sw, sy+sh)
	return SixelSeq(subImage(img, crop), ref.WinCols, visible)
}

// subImage returns the crop of img when the concrete type supports it.
func subImage(img image.Image, r image.Rectangle) image.Image {
	if si, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return si.SubImage(r)
	}
	return img
}
