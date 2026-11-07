package media

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/nfnt/resize"
)

// transmitCellPx approximates a cell's pixel width; used only to bound the
// transmitted image size. The terminal still scales the image into c×r cells.
const transmitCellPx = 12

// maxKittyID is the largest image id that fits the placeholder cell's 24-bit
// foreground encoding without spending a diacritic on a non-zero top byte.
const maxKittyID = 0xFFFFFF

// KittyStore tracks Kitty image ids per photo and their transmission state.
// It is owned by the root model and shared with KittyRenderer.
type KittyStore struct {
	idByPhoto   map[int64]uint32
	colsByPhoto map[int64]int // cols the current placement was transmitted at
	// backByPhoto is the second id of an animated photo: its next frame is
	// transmitted there, off screen, and Swap puts it on screen (#331).
	backByPhoto map[int64]uint32
	// backCols is the width a frame was written behind at, for photos that
	// have one waiting to be swapped in.
	backCols map[int64]int
	next     uint32
}

// NewKittyStore returns an empty store. Numbering starts at a random point in
// the 24-bit id space rather than at 1: the terminal keeps the placements of a
// process that has exited, and iTerm2 resolves a placeholder to the first
// placement it holds under that id, so a run numbering from 1 draws the photos
// of the previous run in the same tab (#259). A random start also keeps us out
// of the way of other programs sharing the terminal.
func NewKittyStore() *KittyStore {
	return &KittyStore{
		idByPhoto:   make(map[int64]uint32),
		colsByPhoto: make(map[int64]int),
		backByPhoto: make(map[int64]uint32),
		backCols:    make(map[int64]int),
		next:        1 + rand.Uint32N(maxKittyID),
	}
}

// IDFor returns a stable Kitty image id for a photo, assigning one on first use.
// Ids are kept within 24 bits so they fit the placeholder foreground encoding.
// It is the id the photo is drawn under; an animated photo's changes at Swap.
func (s *KittyStore) IDFor(photoID int64) uint32 {
	if id, ok := s.idByPhoto[photoID]; ok {
		return id
	}
	id := s.newID()
	s.idByPhoto[photoID] = id
	return id
}

// BackIDFor returns the id an animated photo's next frame is transmitted
// under: never the one it is drawn under, so the frame on screen stays there
// while the next one loads (#331).
func (s *KittyStore) BackIDFor(photoID int64) uint32 {
	if id, ok := s.backByPhoto[photoID]; ok {
		return id
	}
	s.IDFor(photoID)
	id := s.newID()
	s.backByPhoto[photoID] = id
	return id
}

// MarkBackTransmitted records that an animated photo's next frame is written
// under its back id, at cols.
func (s *KittyStore) MarkBackTransmitted(photoID int64, cols int) {
	s.backCols[photoID] = cols
}

// BackReady reports whether an animated photo has a frame written behind,
// waiting for Swap.
func (s *KittyStore) BackReady(photoID int64) bool {
	_, ok := s.backCols[photoID]
	return ok
}

// Swap draws an animated photo under its back id from now on, with the frame
// written there, and leaves the id it was drawn under for its next frame. With
// no frame written behind it does nothing: it would draw an empty id.
func (s *KittyStore) Swap(photoID int64) {
	cols, ok := s.backCols[photoID]
	if !ok {
		return
	}
	back := s.BackIDFor(photoID)
	s.backByPhoto[photoID] = s.idByPhoto[photoID]
	s.idByPhoto[photoID] = back
	s.colsByPhoto[photoID] = cols
	delete(s.backCols, photoID)
}

func (s *KittyStore) newID() uint32 {
	id := s.next
	s.next++
	if s.next > maxKittyID {
		s.next = 1 // wrap; a TUI session will not hold 16M distinct photos
	}
	return id
}

// Ready reports whether the photo is currently transmitted at the given cols.
func (s *KittyStore) Ready(photoID int64, cols int) bool {
	c, ok := s.colsByPhoto[photoID]
	return ok && c == cols
}

// Placed reports whether the photo has a placement on the terminal, at whatever
// width it went out at. It is what a caller asks when it has to delete a
// placement whose size it no longer knows.
func (s *KittyStore) Placed(photoID int64) bool {
	_, ok := s.colsByPhoto[photoID]
	return ok
}

// MarkTransmitted records that the photo's placement exists at cols.
func (s *KittyStore) MarkTransmitted(photoID int64, cols int) {
	s.colsByPhoto[photoID] = cols
}

// Clear marks every image untransmitted (ids stay stable). Call after deleting
// the live placements (DeleteLiveSeq) so images re-transmit on demand.
func (s *KittyStore) Clear() {
	clear(s.colsByPhoto)
	clear(s.backCols)
}

// DeleteLiveSeq returns the concatenated delete-by-id sequences (d=I, freeing
// each placement and its data) for the given photo ids that have an assigned
// image id. Unlike a blanket d=A this is unambiguous for the virtual (U=1)
// placements used for Unicode placeholders; see #94. Photo ids without an
// assigned image id are skipped.
func (s *KittyStore) DeleteLiveSeq(photoIDs []int64) string {
	var b strings.Builder
	for _, pid := range photoIDs {
		if id, ok := s.idByPhoto[pid]; ok {
			b.WriteString(DeleteSeq(id))
		}
		if id, ok := s.backByPhoto[pid]; ok {
			b.WriteString(DeleteSeq(id))
		}
	}
	return b.String()
}

// Untransmit marks a single photo's placement as gone (after deleting it from
// the terminal), so it re-transmits on demand. The id mapping is kept stable.
// A frame written behind is gone with it.
func (s *KittyStore) Untransmit(photoID int64) {
	delete(s.colsByPhoto, photoID)
	delete(s.backCols, photoID)
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
	if err := kitty.EncodeGraphics(&buf, img, transmitOptions(id, cols, rows)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// TransmitFrameSeq is TransmitSeq for a frame of an animation. Its PNG is
// encoded for speed rather than size: at the default level a large frame took
// longer to encode than a frame lasts, about three times as long as this,
// for a fifth less data (#331). The kitty package offers no level, so the
// chunks are written here, the way it writes them.
func TransmitFrameSeq(id uint32, img image.Image, cols, rows int) (string, error) {
	img = scaleForTransmit(img, cols)
	var data bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&data, img); err != nil {
		return "", err
	}
	o := transmitOptions(id, cols, rows)
	payload := base64.StdEncoding.EncodeToString(data.Bytes())
	var b strings.Builder
	for start := 0; start < len(payload) || start == 0; start += kitty.MaxChunkSize {
		end := min(start+kitty.MaxChunkSize, len(payload))
		first, last := start == 0, end == len(payload)
		var keys []string
		if first {
			keys = o.Options()
		} else {
			keys = []string{fmt.Sprintf("q=%d", o.Quite)}
		}
		switch {
		case first && last: // one chunk carries no m=
		case last:
			keys = append(keys, "m=0")
		default:
			keys = append(keys, "m=1")
		}
		b.WriteString(ansi.KittyGraphics([]byte(payload[start:end]), keys...))
		if last {
			break
		}
	}
	return b.String(), nil
}

// transmitOptions transmits an image as PNG under id and places it virtually
// over cols×rows cells, quietly.
func transmitOptions(id uint32, cols, rows int) *kitty.Options {
	return &kitty.Options{
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
	id := r.store.IDFor(photoID)
	k := blockKey{photoID: photoID, cols: cols, kittyID: id}
	return r.cache.get(k, func() []string {
		b := img.Bounds()
		rows := PhotoRows(b.Dx(), b.Dy(), cols, CellAspect())
		return placeholderLines(id, cols, rows)
	})
}

// RenderWindow returns the placeholder cells for a centered sub-rectangle of the
// image transmitted at coverCols wide, or nil until it is transmitted at that
// width. img/coverRows are unused (the placement already holds the pixels); they
// keep the signature uniform with the block renderer.
func (r *KittyRenderer) RenderWindow(photoID int64, _ image.Image, coverCols, _, hOff, vOff, winCols, winRows int) []string {
	if !r.store.Ready(photoID, coverCols) {
		return nil
	}
	return PlaceholderWindow(r.store.IDFor(photoID), hOff, vOff, winCols, winRows)
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
	return PlaceholderWindow(id, 0, 0, cols, rows)
}

// PlaceholderWindow builds winRows lines of winCols Kitty Unicode placeholder
// cells that reference the sub-rectangle [vOff, vOff+winRows) x [hOff,
// hOff+winCols) of the image transmitted under id. Each cell carries explicit
// (row, col) diacritics, so the terminal draws exactly that slice of the
// placement — the same mechanism the message list uses for partial-scroll
// slicing, here used to crop a mosaic tile to a centered window. hOff/vOff of 0
// with winCols/winRows equal to the placement size yields the whole image.
//
// A third diacritic spells out the image id's most significant byte. The spec
// lets it be omitted while the id fits in 24 bits (ours always does), but
// iTerm2 reads an absent one as -1 and shifts it into the id as 0xff000000,
// then finds no placement under that number and draws nothing (#259). Sending
// it costs one combining mark per cell and terminals that default it to zero
// read the same id either way.
func PlaceholderWindow(id uint32, hOff, vOff, winCols, winRows int) []string {
	fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm",
		byte((id>>16)&0xff), byte((id>>8)&0xff), byte(id&0xff))
	msb := kitty.Diacritic(int(id >> 24))
	lines := make([]string, winRows)
	for r := 0; r < winRows; r++ {
		var sb strings.Builder
		sb.WriteString(fg)
		rd := kitty.Diacritic(vOff + r)
		for c := 0; c < winCols; c++ {
			sb.WriteRune(kitty.Placeholder)
			sb.WriteRune(rd)
			sb.WriteRune(kitty.Diacritic(hOff + c))
			sb.WriteRune(msb)
		}
		sb.WriteString("\x1b[0m")
		lines[r] = sb.String()
	}
	return lines
}
