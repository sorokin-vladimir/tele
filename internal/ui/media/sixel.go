package media

import (
	"errors"
	"image"
	"image/color"
	"sort"
	"strings"

	"github.com/nfnt/resize"

	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// Sixel rendering, for terminals that draw DEC sixel graphics but not the Kitty
// protocol - foot above all, which implements sixel and nothing else.
//
// Unlike a Kitty placement, a sixel is not a cell reference: it is painted at
// the cursor and attached to the cells it covers, and foot erases the covered
// part of an image when those cells are reprinted (sixel_overwrite_at_cursor in
// terminal.c). So a sixel cannot live inside a repainted frame the way a
// placeholder grid does; the caller paints it out of band, after the frame,
// over cells reserved by the frame. Both modals and inline previews use this
// ordering; inline placements must also be invalidated when the viewport moves.

const (
	// sixelMaxColors is the most a sixel image may use. 256 is what every
	// terminal accepts (and foot's own ceiling).
	sixelMaxColors = 256

	// sixelBandRows is fixed by the format: one sixel character carries six
	// vertical pixels, so images are emitted in six-row bands.
	sixelBandRows = 6

	// Fallbacks for a terminal that does not report its cell pixel size (a
	// pipe, a multiplexer without pixel reporting). Only the aspect matters,
	// as the terminal scales the image into the cells it covers anyway.
	sixelFallbackCellW = 10
	sixelFallbackCellH = 20
)

// ErrEmptyImage is returned for a nil or zero-sized image.
var ErrEmptyImage = errors.New("media: empty image")

// SixelSeq encodes img as one sixel sequence sized to fill cols×rows terminal
// cells, with no cursor addressing around it: the caller positions the cursor
// first and restores it afterwards (see PositionSixel).
//
// The image is scaled to the box's real pixel size when the terminal reports
// its cell size, so the graphic lands in exactly the reserved cells instead of
// being stretched by the terminal's own scaling rules.
func SixelSeq(img image.Image, cols, rows int) (string, error) {
	if img == nil {
		return "", ErrEmptyImage
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return "", ErrEmptyImage
	}
	if cols <= 0 || rows <= 0 {
		return "", ErrEmptyImage
	}
	cw, ch := CellPx()
	w, h := sixelPixelSize(cols, rows, cw, ch)
	if w <= 0 || h <= 0 {
		return "", ErrEmptyImage
	}

	// Bilinear, like block art: Lanczos on a multi-megapixel photo costs more
	// than the difference shows at cell resolution.
	scaled := flattenOnBG(resize.Resize(uint(w), uint(h), img, resize.Bilinear))
	rgba := toRGBA(scaled)
	w, h = rgba.Bounds().Dx(), rgba.Bounds().Dy()

	pal, idx := quantizeSixel(rgba, sixelMaxColors)
	if len(pal) == 0 {
		return "", ErrEmptyImage
	}

	var sb strings.Builder
	// P1=0 leaves the pixel aspect to the terminal, P2=1 paints an opaque
	// background (photos have no transparency to preserve), P3=0 is the grid
	// mode every terminal implements.
	sb.WriteString("\x1bP0;1;0q")
	// Raster attributes: aspect 1:1, then the image's pixel size. Terminals do
	// not need them, but they let a receiving program bound the bitmap before
	// decoding, and foot uses them to place the cursor under the image.
	sb.WriteString("\"1;1;")
	sb.WriteString(itoa(w))
	sb.WriteByte(';')
	sb.WriteString(itoa(h))

	for i, c := range pal {
		// Sixel colors are percentages, not 0-255 bytes.
		sb.WriteByte('#')
		sb.WriteString(itoa(i))
		sb.WriteString(";2;")
		sb.WriteString(itoa(int(c.r) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(itoa(int(c.g) * 100 / 255))
		sb.WriteByte(';')
		sb.WriteString(itoa(int(c.b) * 100 / 255))
	}

	appendSixelBands(&sb, idx, w, h)
	sb.WriteString("\x1b\\")
	return sb.String(), nil
}

// PositionSixel wraps an encoded sixel sequence so it is painted with its
// top-left corner at the given 1-based screen row and column, leaving the
// cursor where it found it (DECSC/DECRC). Painting is by definition a cursor
// move and a side effect on the cells below, so the caller must be sure nothing
// will reprint those cells afterwards.
func PositionSixel(row, col int, seq string) string {
	if seq == "" {
		return ""
	}
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	var sb strings.Builder
	sb.WriteString("\x1b7") // save cursor
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(row))
	sb.WriteByte(';')
	sb.WriteString(itoa(col))
	sb.WriteByte('H')
	sb.WriteString(seq)
	sb.WriteString("\x1b8") // restore cursor
	return sb.String()
}

// sixelPixelSize returns the pixel size to encode at so the image fills cols×
// rows cells. A partial final band has unused bits; rounding the raster up to
// six pixels would paint outside the reserved cells into the following row.
func sixelPixelSize(cols, rows int, cellW, cellH float64) (int, int) {
	if cols <= 0 || rows <= 0 {
		return 0, 0
	}
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = sixelFallbackCellW, sixelFallbackCellH
	}
	w := int(float64(cols)*cellW + 0.5)
	h := int(float64(rows)*cellH + 0.5)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if w > maxTransmitPx {
		// Keep the box's aspect while bounding the upload.
		h = int(float64(h)*float64(maxTransmitPx)/float64(w) + 0.5)
		w = maxTransmitPx
	}
	return w, h
}

// appendSixelBands writes the pixel data: one six-row band at a time, one
// run-length encoded pass per color present in that band, colors separated by
// '$' (carriage return within the band) and bands by '-' (next band).
func appendSixelBands(sb *strings.Builder, idx []uint8, w, h int) {
	bits := make([]uint8, w)
	for band := 0; band*sixelBandRows < h; band++ {
		if band > 0 {
			sb.WriteByte('-')
		}
		y0 := band * sixelBandRows

		// Colors used in this band, ascending so the output is reproducible.
		var used []int
		seen := make([]bool, sixelMaxColors)
		for y := y0; y < y0+sixelBandRows && y < h; y++ {
			row := idx[y*w : (y+1)*w]
			for _, v := range row {
				if !seen[v] {
					seen[v] = true
					used = append(used, int(v))
				}
			}
		}
		sort.Ints(used)

		for i, c := range used {
			if i > 0 {
				sb.WriteByte('$')
			}
			sb.WriteByte('#')
			sb.WriteString(itoa(c))
			for x := range bits {
				bits[x] = 0
			}
			for y := y0; y < y0+sixelBandRows && y < h; y++ {
				bit := uint8(1) << uint(y-y0)
				row := idx[y*w : (y+1)*w]
				for x, v := range row {
					if int(v) == c {
						bits[x] |= bit
					}
				}
			}
			appendSixelRun(sb, bits)
		}
	}
}

// appendSixelRun writes one band's bits for one color, run-length encoded: a
// run of four or more becomes "!n<char>", anything shorter is spelled out.
func appendSixelRun(sb *strings.Builder, bits []uint8) {
	for i := 0; i < len(bits); {
		j := i + 1
		for j < len(bits) && bits[j] == bits[i] {
			j++
		}
		n := j - i
		ch := byte('?') + bits[i]
		if n >= 4 {
			sb.WriteByte('!')
			sb.WriteString(itoa(n))
			sb.WriteByte(ch)
		} else {
			for k := 0; k < n; k++ {
				sb.WriteByte(ch)
			}
		}
		i = j
	}
}

// itoa is strconv.Itoa for the small non-negative ints this file writes, kept
// local so the encoder reads as one piece.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// flattenOnBG composites an image with alpha onto the theme's background, so a
// sticker does not arrive with a black box around it on a light theme. Sixel
// has no per-pixel alpha, and P2=1 asks the terminal for an opaque background,
// so every pixel needs a color before quantizing.
func flattenOnBG(img image.Image) image.Image {
	bg := color.Color(color.Black)
	if t := theme.T(); t != nil && !theme.IsNone(t.Background) {
		bg = t.Background
	}
	dst := image.NewRGBA(img.Bounds())
	for y := dst.Bounds().Min.Y; y < dst.Bounds().Max.Y; y++ {
		for x := dst.Bounds().Min.X; x < dst.Bounds().Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0xffff {
				dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff})
				continue
			}
			br, bgc, bb, _ := bg.RGBA()
			mix := func(src, dst uint32) uint8 {
				// Both are 16-bit premultiplied by their alpha; composite in
				// 16-bit then drop to 8.
				v := src + dst*(0xffff-a)/0xffff
				return uint8(v >> 8)
			}
			dst.SetRGBA(x, y, color.RGBA{mix(r, br), mix(g, bgc), mix(b, bb), 0xff})
		}
	}
	return dst
}

// toRGBA returns img as *image.RGBA, converting when needed.
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			dst.SetRGBA(x-b.Min.X, y-b.Min.Y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// sixelColor is one palette entry.
type sixelColor struct{ r, g, b uint8 }

// paletteEntry is one occupied 5-bit color cell and its weighted sums, used
// while median-cutting.
type paletteEntry struct {
	r5, g5, b5       int
	count            int
	rsum, gsum, bsum int
}

// paletteBox is a set of entries that will become one palette color.
type paletteBox struct {
	entries                            []paletteEntry
	count                              int
	rmin, rmax, gmin, gmax, bmin, bmax int
}

// quantizeSixel maps img to at most maxColors palette colors, returning the
// palette and one index per pixel (row-major).
//
// No dithering, deliberately. Error diffusion makes every pixel disagree with
// its neighbours by a palette step, which destroys the runs sixel's RLE is
// built on: a dithered 380x240 photo encodes to a third of a megabyte, against
// tens of kilobytes without. The whole sequence crosses a socket to the
// terminal on every repaint, so the banding a 256-color palette shows in a
// smooth gradient is the cheaper of the two problems.
//
// The palette is built by median cut over a 5-bit-per-channel histogram: 32768
// possible cells is small enough to sort cheaply and fine enough that a photo's
// colors survive, while exact 24-bit colors would put a map entry per pixel.
func quantizeSixel(img *image.RGBA, maxColors int) ([]sixelColor, []uint8) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || maxColors <= 0 {
		return nil, nil
	}

	hist := make(map[uint16]*paletteEntry, 4096)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			r, g, bl := int(row[x*4]), int(row[x*4+1]), int(row[x*4+2])
			key := uint16(r>>3)<<10 | uint16(g>>3)<<5 | uint16(bl>>3)
			e := hist[key]
			if e == nil {
				e = &paletteEntry{r5: r >> 3, g5: g >> 3, b5: bl >> 3}
				hist[key] = e
			}
			e.count++
			e.rsum += r
			e.gsum += g
			e.bsum += bl
		}
	}

	entries := make([]paletteEntry, 0, len(hist))
	for _, e := range hist {
		entries = append(entries, *e)
	}
	// Map iteration must not choose different median-cut ties after a resize
	// invalidates the encoding cache. Start every split from the same ordering.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		return a.r5<<10|a.g5<<5|a.b5 < b.r5<<10|b.g5<<5|b.b5
	})

	boxes := []*paletteBox{newPaletteBox(entries)}
	for len(boxes) < maxColors {
		// Split the box with the widest channel range, weighted by population,
		// so a photo's dominant gradient gets the palette entries.
		best := -1
		bestScore := 0
		for i, box := range boxes {
			if len(box.entries) < 2 {
				continue
			}
			score := box.widestRange() * box.count
			if score > bestScore {
				bestScore, best = score, i
			}
		}
		if best < 0 {
			break
		}
		a, b2 := boxes[best].split()
		if a == nil || b2 == nil {
			break
		}
		boxes[best] = a
		boxes = append(boxes, b2)
	}

	pal := make([]sixelColor, 0, len(boxes))
	for _, box := range boxes {
		if box.count == 0 {
			continue
		}
		pal = append(pal, sixelColor{
			r: uint8(box.sumR() / box.count),
			g: uint8(box.sumG() / box.count),
			b: uint8(box.sumB() / box.count),
		})
	}
	if len(pal) == 0 {
		return nil, nil
	}

	idx := make([]uint8, w*h)
	cache := make(map[uint32]uint8, 4096)
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w*4]
		for x := 0; x < w; x++ {
			r, g, bl := row[x*4], row[x*4+1], row[x*4+2]
			k := uint32(r>>3)<<10 | uint32(g>>3)<<5 | uint32(bl>>3)
			pi, ok := cache[k]
			if !ok {
				pi = nearestSixel(pal, int(r), int(g), int(bl))
				cache[k] = pi
			}
			idx[y*w+x] = pi
		}
	}
	return pal, idx
}

// nearestSixel returns the palette index closest to the given color.
func nearestSixel(pal []sixelColor, r, g, b int) uint8 {
	best := 0
	bestDist := int64(1) << 62
	for i, c := range pal {
		dr := int64(int(c.r) - r)
		dg := int64(int(c.g) - g)
		db := int64(int(c.b) - b)
		d := dr*dr + dg*dg + db*db
		if d < bestDist {
			bestDist, best = d, i
			if d == 0 {
				break
			}
		}
	}
	return uint8(best)
}

func newPaletteBox(entries []paletteEntry) *paletteBox {
	box := &paletteBox{entries: entries}
	for _, e := range entries {
		box.count += e.count
		if e.r5 < box.rmin {
			box.rmin = e.r5
		}
		if e.r5 > box.rmax {
			box.rmax = e.r5
		}
		if e.g5 < box.gmin {
			box.gmin = e.g5
		}
		if e.g5 > box.gmax {
			box.gmax = e.g5
		}
		if e.b5 < box.bmin {
			box.bmin = e.b5
		}
		if e.b5 > box.bmax {
			box.bmax = e.b5
		}
	}
	return box
}

func (b *paletteBox) widestRange() int {
	widest := b.rmax - b.rmin
	if d := b.gmax - b.gmin; d > widest {
		widest = d
	}
	if d := b.bmax - b.bmin; d > widest {
		widest = d
	}
	return widest
}

// split cuts the box at the weighted median of its widest channel.
func (b *paletteBox) split() (*paletteBox, *paletteBox) {
	if len(b.entries) < 2 {
		return nil, nil
	}
	channel := 0 // 0=r, 1=g, 2=b
	width := b.rmax - b.rmin
	if d := b.gmax - b.gmin; d > width {
		width, channel = d, 1
	}
	if d := b.bmax - b.bmin; d > width {
		channel = 2
	}
	value := func(e paletteEntry) int {
		switch channel {
		case 0:
			return e.r5
		case 1:
			return e.g5
		}
		return e.b5
	}
	sort.Slice(b.entries, func(i, j int) bool { return value(b.entries[i]) < value(b.entries[j]) })
	half := b.count / 2
	seen := 0
	cut := 0
	for i, e := range b.entries {
		seen += e.count
		if seen >= half {
			cut = i + 1
			break
		}
	}
	if cut <= 0 {
		cut = 1
	}
	if cut >= len(b.entries) {
		cut = len(b.entries) - 1
	}
	return newPaletteBox(b.entries[:cut]), newPaletteBox(b.entries[cut:])
}

func (b *paletteBox) sumR() int {
	s := 0
	for _, e := range b.entries {
		s += e.rsum
	}
	return s
}

func (b *paletteBox) sumG() int {
	s := 0
	for _, e := range b.entries {
		s += e.gsum
	}
	return s
}

func (b *paletteBox) sumB() int {
	s := 0
	for _, e := range b.entries {
		s += e.bsum
	}
	return s
}
