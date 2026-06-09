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

// TransmitSeq encodes a transmit-and-virtual-place sequence for an image scaled
// into cols×rows cells. The image is downscaled to bound bandwidth.
func TransmitSeq(id uint32, img image.Image, cols, rows int) (string, error) {
	maxPx := uint(cols * transmitCellPx)
	if b := img.Bounds(); maxPx > 0 && uint(b.Dx()) > maxPx {
		img = resize.Resize(maxPx, 0, img, resize.Bilinear) // 0 height preserves aspect
	}
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
	store      *KittyStore
	cache      map[blockKey][]string
	cellAspect float64
}

// NewKittyRenderer returns a renderer backed by the given store. The cell aspect
// is detected from the terminal so real-pixel placements match the image.
func NewKittyRenderer(store *KittyStore) *KittyRenderer {
	return &KittyRenderer{
		store:      store,
		cache:      make(map[blockKey][]string),
		cellAspect: CellAspect(),
	}
}

// SetCellAspect overrides the cell aspect ratio (height/width). Mainly for tests
// and for refreshing after a terminal font/zoom change.
func (r *KittyRenderer) SetCellAspect(a float64) {
	r.cellAspect = a
	clear(r.cache)
}

// Footprint returns the row height for a real-pixel placement at the terminal's
// cell aspect ratio.
func (r *KittyRenderer) Footprint(imgW, imgH, cols int) int {
	return kittyTermLines(imgW, imgH, cols, r.cellAspect)
}

// CanRender reports whether the image is transmitted at cols, i.e. whether
// Render will emit the placeholder grid instead of nil. It mirrors Render's
// readiness gate so the layout height calc reserves the footprint only once the
// placement actually exists (issue #115).
func (r *KittyRenderer) CanRender(photoID int64, cols int) bool {
	return r.store.Ready(photoID, cols)
}

// Render returns placeholder lines, or nil if the image is not yet transmitted
// at this width (caller shows the text placeholder meanwhile).
func (r *KittyRenderer) Render(photoID int64, img image.Image, cols int) []string {
	if !r.store.Ready(photoID, cols) {
		return nil
	}
	k := blockKey{photoID: photoID, cols: cols}
	if v, ok := r.cache[k]; ok {
		return v
	}
	b := img.Bounds()
	rows := r.Footprint(b.Dx(), b.Dy(), cols)
	v := placeholderLines(r.store.IDFor(photoID), cols, rows)
	r.cache[k] = v
	return v
}

// Reset clears the placeholder cache (call on width change).
func (r *KittyRenderer) Reset() {
	clear(r.cache)
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
