package media

import (
	"image"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nfnt/resize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sixelDecoded is what a sixel decoder recovered from a sequence: the raster
// size and one RGB color per pixel, uncovered pixels left black.
type sixelDecoded struct {
	w, h int
	pix  []color.RGBA
}

// decodeSixel is a deliberately small sixel reader used to check what the
// encoder produced. It understands exactly what SixelSeq emits: raster
// attributes, color definitions and RLE-compressed band data.
func decodeSixel(t *testing.T, seq string) sixelDecoded {
	t.Helper()
	start := strings.Index(seq, "q")
	require.GreaterOrEqual(t, start, 0, "sequence must carry a DCS introducer ending in q")
	body := seq[start+1:]
	require.True(t, strings.HasSuffix(body, "\x1b\\"), "sequence must end in ST")
	body = strings.TrimSuffix(body, "\x1b\\")

	var out sixelDecoded
	palette := map[int]color.RGBA{}
	cur := color.RGBA{}
	band, x := 0, 0
	ensure := func() {
		if out.w == 0 || out.h == 0 {
			t.Fatalf("pixel data before raster attributes")
		}
		if len(out.pix) == 0 {
			out.pix = make([]color.RGBA, out.w*out.h)
		}
	}

	for i := 0; i < len(body); {
		c := body[i]
		switch {
		case c == '"':
			// "Pan;Pad;Ph;Pv
			j := i + 1
			for j < len(body) && (body[j] == ';' || isASCIIDigit(body[j])) {
				j++
			}
			params := strings.Split(body[i+1:j], ";")
			require.Len(t, params, 4, "raster attributes must carry four fields")
			var err error
			out.w, err = strconv.Atoi(params[2])
			require.NoError(t, err)
			out.h, err = strconv.Atoi(params[3])
			require.NoError(t, err)
			require.Positive(t, out.w)
			require.Positive(t, out.h)
			i = j
		case c == '#':
			j := i + 1
			for j < len(body) && isASCIIDigit(body[j]) {
				j++
			}
			idx, err := strconv.Atoi(body[i+1 : j])
			require.NoError(t, err)
			if j < len(body) && body[j] == ';' {
				// #idx;2;r;g;b with the three color fields in percent.
				fields := make([]int, 0, 4)
				for k := j; len(fields) < 4 && k < len(body) && body[k] == ';'; {
					k++
					n := k
					for n < len(body) && isASCIIDigit(body[n]) {
						n++
					}
					v, err := strconv.Atoi(body[k:n])
					require.NoError(t, err)
					fields = append(fields, v)
					k = n
					j = k
				}
				require.Len(t, fields, 4, "color definition must carry four fields")
				palette[idx] = color.RGBA{
					uint8(fields[1] * 255 / 100),
					uint8(fields[2] * 255 / 100),
					uint8(fields[3] * 255 / 100),
					0xff,
				}
				i = j
				continue
			}
			cur = palette[idx]
			i = j
		case c == '$':
			x = 0
			i++
		case c == '-':
			band++
			x = 0
			i++
		case c == '!':
			j := i + 1
			for j < len(body) && body[j] >= '0' && body[j] <= '9' {
				j++
			}
			n, err := strconv.Atoi(body[i+1 : j])
			require.NoError(t, err)
			require.Less(t, j, len(body), "RLE count must be followed by a character")
			bits := body[j] - '?'
			ensure()
			for k := 0; k < n; k++ {
				putSixelColumn(&out, cur, x, band, bits)
				x++
			}
			i = j + 1
		case c >= '?' && c <= '~':
			bits := c - '?'
			ensure()
			putSixelColumn(&out, cur, x, band, bits)
			x++
			i++
		default:
			t.Fatalf("unexpected byte %q at %d in sixel data", c, i)
		}
	}
	return out
}

func putSixelColumn(out *sixelDecoded, c color.RGBA, x, band int, bits byte) {
	for k := 0; k < 6; k++ {
		if bits&(1<<uint(k)) == 0 {
			continue
		}
		y := band*6 + k
		if x < 0 || x >= out.w || y < 0 || y >= out.h {
			continue
		}
		out.pix[y*out.w+x] = c
	}
}

// testPhoto builds an opaque image with a horizontal gradient plus a bright
// corner block, so a quantizer has both a smooth ramp and a hard edge to keep.
func testPhoto(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := uint8(x * 255 / w)
			g := uint8(y * 255 / h)
			b := uint8((x + y) * 255 / (w + h))
			if x < w/4 && y < h/4 {
				r, g, b = 250, 250, 40
			}
			img.SetRGBA(x, y, color.RGBA{r, g, b, 0xff})
		}
	}
	return img
}

func TestSixelSeqStructure(t *testing.T) {
	seq, err := SixelSeq(testPhoto(120, 90), 20, 10)
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(seq, "\x1bP0;1;0q"), "DCS with an opaque background")
	assert.True(t, strings.HasSuffix(seq, "\x1b\\"), "must end with ST")

	// Every palette definition is the only place "#i;2;r;g;b" appears; the
	// "#i" in the pixel data only selects an entry already defined.
	defs := len(regexp.MustCompile(`#[0-9]+;2;[0-9]+;[0-9]+;[0-9]+`).FindAllString(seq, -1))
	assert.Positive(t, defs)
	assert.LessOrEqual(t, defs, sixelMaxColors, "never more colors than a terminal accepts")
}

func TestSixelSeqRoundTrip(t *testing.T) {
	const cols, rows = 24, 12
	src := testPhoto(96, 48)
	seq, err := SixelSeq(src, cols, rows)
	require.NoError(t, err)

	got := decodeSixel(t, seq)
	cw, ch := CellPx()
	wantW, wantH := sixelPixelSize(cols, rows, cw, ch)
	assert.Equal(t, wantW, got.w)
	assert.Equal(t, wantH, got.h)
	require.NotEmpty(t, got.pix)

	// The encoder scales with bilinear the same way, so a decoded pixel should
	// land within quantization noise of the reference.
	ref := resize.Resize(uint(wantW), uint(wantH), src, resize.Bilinear)
	var total, n int
	for y := 0; y < got.h; y++ {
		for x := 0; x < got.w; x++ {
			g := got.pix[y*got.w+x]
			r, gg, b, _ := ref.At(x, y).RGBA()
			// Dithering trades per-pixel accuracy for average accuracy; a single
			// pixel may be off by a whole palette step, which is why the check
			// is a mean over the image.
			total += absInt(int(g.R) - int(r>>8))
			total += absInt(int(g.G) - int(gg>>8))
			total += absInt(int(g.B) - int(b>>8))
			n += 3
		}
	}
	mean := total / n
	assert.Less(t, mean, 12, "mean channel error after quantization should stay small")

	// Distinct colors prove the palette was actually used, not collapsed to a
	// single entry.
	seen := map[color.RGBA]bool{}
	for _, p := range got.pix {
		seen[p] = true
	}
	assert.Greater(t, len(seen), 8, "a gradient should quantize to many palette colors")
}

func TestSixelSeqRejectsEmpty(t *testing.T) {
	_, err := SixelSeq(nil, 10, 10)
	assert.ErrorIs(t, err, ErrEmptyImage)

	_, err = SixelSeq(image.NewRGBA(image.Rect(0, 0, 0, 0)), 10, 10)
	assert.ErrorIs(t, err, ErrEmptyImage)

	_, err = SixelSeq(testPhoto(10, 10), 0, 0)
	assert.ErrorIs(t, err, ErrEmptyImage)
}

func TestSixelPixelSize(t *testing.T) {
	w, h := sixelPixelSize(80, 40, 10, 20)
	assert.Equal(t, 800, w)
	assert.Equal(t, 800, h, "never overflow the reserved cell footprint")

	// Unknown cell size falls back to the default aspect.
	w, h = sixelPixelSize(10, 5, 0, 0)
	assert.Equal(t, 10*sixelFallbackCellW, w)
	assert.Equal(t, 100, h)

	// The width cap scales the height down with it.
	w, h = sixelPixelSize(400, 100, 40, 40)
	assert.Equal(t, maxTransmitPx, w)
	assert.Less(t, h, 4000)
	assert.Positive(t, h)
}

func TestSixelPartialBandKeepsExactHeight(t *testing.T) {
	t.Cleanup(func() { SetCellPx(0, 0) })
	SetCellPx(10, 23)
	seq, err := SixelSeq(gradientImage(20, 20), 2, 1)
	require.NoError(t, err)
	decoded := decodeSixel(t, seq)
	assert.Equal(t, 23, decoded.h)
	for x := 0; x < decoded.w; x++ {
		assert.Equal(t, uint8(255), decoded.pix[22*decoded.w+x].A,
			"the partial final band must still paint the last row")
	}
}

func TestPositionSixel(t *testing.T) {
	seq := PositionSixel(5, 9, "X")
	assert.Equal(t, "\x1b7\x1b[5;9HX\x1b8", seq, "save, move, paint, restore")

	assert.Empty(t, PositionSixel(1, 1, ""), "nothing to paint, nothing to wrap")
	assert.Equal(t, "\x1b7\x1b[1;1HX\x1b8", PositionSixel(0, 0, "X"), "coordinates are clamped to the screen")
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }
