package media

import (
	"bytes"
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/nfnt/resize"
)

// transmitCellPx approximates a cell's pixel width; used only to bound the
// transmitted image size. The terminal still scales the image into c×r cells.
const transmitCellPx = 12

// KittyStore tracks Kitty image ids per photo and their transmission state.
// It is owned by the root model and shared with KittyRenderer.
type KittyStore struct {
	idByPhoto   map[int64]uint32
	colsByPhoto map[int64]int // cols the current placement was transmitted at
	next        uint32
}

// NewKittyStore returns an empty store. Ids start at 1 (Kitty ids are positive).
func NewKittyStore() *KittyStore {
	return &KittyStore{
		idByPhoto:   make(map[int64]uint32),
		colsByPhoto: make(map[int64]int),
		next:        1,
	}
}

// IDFor returns a stable Kitty image id for a photo, assigning one on first use.
// Ids are kept within 24 bits so they fit the placeholder foreground encoding.
func (s *KittyStore) IDFor(photoID int64) uint32 {
	if id, ok := s.idByPhoto[photoID]; ok {
		return id
	}
	id := s.next
	s.next++
	if s.next > 0xFFFFFF {
		s.next = 1 // wrap; a TUI session will not hold 16M distinct photos
	}
	s.idByPhoto[photoID] = id
	return id
}

// Ready reports whether the photo is currently transmitted at the given cols.
func (s *KittyStore) Ready(photoID int64, cols int) bool {
	c, ok := s.colsByPhoto[photoID]
	return ok && c == cols
}

// MarkTransmitted records that the photo's placement exists at cols.
func (s *KittyStore) MarkTransmitted(photoID int64, cols int) {
	s.colsByPhoto[photoID] = cols
}

// Clear marks every image untransmitted (ids stay stable). Call after sending
// DeleteAllSeq so images re-transmit on demand.
func (s *KittyStore) Clear() {
	clear(s.colsByPhoto)
}

// Untransmit marks a single photo's placement as gone (after deleting it from
// the terminal), so it re-transmits on demand. The id mapping is kept stable.
func (s *KittyStore) Untransmit(photoID int64) {
	delete(s.colsByPhoto, photoID)
}

// DeleteAllSeq returns the Kitty sequence that deletes all images and frees
// their data (a=d, d=A), quietly.
func DeleteAllSeq() string {
	opts := &kitty.Options{
		Action:          kitty.Delete,
		Delete:          kitty.DeleteAll,
		DeleteResources: true,
		Quite:           2,
	}
	return ansi.KittyGraphics(nil, opts.Options()...)
}

// DeleteSeq returns the Kitty sequence that deletes a single image by id and
// frees its data (a=d, d=I), quietly. Used to evict off-screen placements so the
// terminal stays under its image-resource limit.
func DeleteSeq(id uint32) string {
	opts := &kitty.Options{
		Action:          kitty.Delete,
		Delete:          kitty.DeleteID,
		DeleteResources: true,
		ID:              int(id),
		Quite:           2,
	}
	return ansi.KittyGraphics(nil, opts.Options()...)
}

// maxTransmitPx caps the transmitted image width so an extreme cell size or
// column count cannot produce an absurdly large upload.
const maxTransmitPx = 2048

// transmitTargetWidth returns the pixel width to transmit an image at so it
// fills cols cells at the terminal's real cell width. Returns 0 when the width
// should be left unchanged (cell size unknown and no bandwidth bound applies).
//
// When the terminal reports its cell pixel size we size the image to the box's
// true pixel width (scaling up small thumbnails too): terminals such as Ghostty
// render Unicode-placeholder images at the image's own resolution rather than
// stretching them to the c×r cell box, so a small thumbnail otherwise shows up
// shrunken in the corner of its reserved space. When the cell size is unknown we
// fall back to a width bound that only downscales (the prior behavior).
func transmitTargetWidth(curW, cols int, cellW float64) int {
	if cellW > 0 {
		target := int(float64(cols)*cellW + 0.5)
		if target > maxTransmitPx {
			target = maxTransmitPx
		}
		if target <= 0 || target == curW {
			return 0
		}
		return target
	}
	// Unknown cell size: bound bandwidth by downscaling only, never upscale.
	maxPx := cols * transmitCellPx
	if maxPx > 0 && curW > maxPx {
		return maxPx
	}
	return 0
}

// scaleForTransmit resizes img to fill cols cells at the terminal's real cell
// width, preserving aspect ratio. See transmitTargetWidth for the rationale.
func scaleForTransmit(img image.Image, cols int) image.Image {
	cw, _ := CellPx()
	target := transmitTargetWidth(img.Bounds().Dx(), cols, cw)
	if target == 0 {
		return img
	}
	return resize.Resize(uint(target), 0, img, resize.Bilinear) // 0 height preserves aspect
}

// TransmitSeq encodes a transmit-and-virtual-place sequence for an image scaled
// into cols×rows cells. The image is resized to the reserved box's real pixel
// size so it fills the placement regardless of terminal upscaling behavior.
func TransmitSeq(id uint32, img image.Image, cols, rows int) (string, error) {
	img = scaleForTransmit(img, cols)
	var buf bytes.Buffer
	opts := &kitty.Options{
		Action: kitty.TransmitAndPut,
		ID:     int(id),
		// PNG is self-describing: the terminal reads dimensions from the data,
		// so no s=/v= keys are needed (raw RGBA without them is undecodable).
		Format:           kitty.PNG,
		Transmission:     kitty.Direct,
		Chunk:            true,
		VirtualPlacement: true,
		Columns:          cols,
		Rows:             rows,
		Quite:            2,
	}
	if err := kitty.EncodeGraphics(&buf, img, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// KittyRenderer emits Unicode-placeholder grids that reference images already
// transmitted to the terminal via KittyStore. Output is cached per (photo, cols).
type KittyRenderer struct {
	store *KittyStore
	cache *renderCache
}

// NewKittyRenderer returns a renderer backed by the given store.
func NewKittyRenderer(store *KittyStore) *KittyRenderer {
	return &KittyRenderer{
		store: store,
		cache: newRenderCache(),
	}
}

// Render returns placeholder lines, or nil if the image is not yet transmitted
// at this width (caller shows the text placeholder meanwhile).
func (r *KittyRenderer) Render(photoID int64, img image.Image, cols int) []string {
	if !r.store.Ready(photoID, cols) {
		return nil
	}
	k := blockKey{photoID: photoID, cols: cols}
	return r.cache.get(k, func() []string {
		b := img.Bounds()
		rows := PhotoRows(b.Dx(), b.Dy(), cols, CellAspect())
		return placeholderLines(r.store.IDFor(photoID), cols, rows)
	})
}

// Reset clears the placeholder cache (call on width change).
func (r *KittyRenderer) Reset() {
	r.cache.reset()
}

// PlaceholderLines exposes the Kitty Unicode-placeholder grid for a transmitted
// image id, for callers (e.g. a fullscreen video overlay) that compose their own
// layout rather than going through KittyRenderer.
func PlaceholderLines(id uint32, cols, rows int) []string {
	return placeholderLines(id, cols, rows)
}

// placeholderLines builds the rows×cols grid of Kitty Unicode placeholder cells.
// Each cell is the placeholder rune followed by its row and column diacritics;
// the image id is carried in the 24-bit foreground color. Every cell carries
// explicit (row, col) so the message list's line slicing shows the correct
// vertical slice when a photo is partially scrolled.
func placeholderLines(id uint32, cols, rows int) []string {
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm",
		byte((id>>16)&0xff), byte((id>>8)&0xff), byte(id&0xff))
	lines := make([]string, rows)
	for row := 0; row < rows; row++ {
		var sb strings.Builder
		sb.WriteString(fg)
		rd := kitty.Diacritic(row)
		for col := 0; col < cols; col++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(rd)
			sb.WriteRune(kitty.Diacritic(col))
		}
		sb.WriteString("\x1b[0m")
		lines[row] = sb.String()
	}
	return lines
}
