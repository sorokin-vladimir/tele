package media

import (
	"image"
	"image/color"
	"regexp"
	"strconv"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gradientImage returns a small image whose every pixel is distinct, so a crop
// of it is never identical to the whole.
func gradientImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: 128, A: 255})
		}
	}
	return img
}

func TestInlineMarkerRoundTrip(t *testing.T) {
	ref := InlineRef{ID: 4242, CoverCols: 30, CoverRows: 18, HOff: 3, VOff: 4, WinCols: 12, WinRows: 6, Row: 5}
	line := "prefix " + InlineMarker(ref) + "art"

	got, start, next, ok := ParseInlineMarker(line, 0)
	require.True(t, ok, "marker parses")
	assert.Equal(t, ref, got)
	assert.Equal(t, len("prefix "), start, "start is where the marker begins")
	assert.Equal(t, start+len(InlineMarker(ref)), next, "next points past the marker")
	assert.Equal(t, "art", line[next:], "the art text follows the marker")
}

func TestParseInlineMarkerSkipsLeadingText(t *testing.T) {
	ref := InlineRef{ID: 7, CoverCols: 4, CoverRows: 2, WinCols: 4, WinRows: 2}
	line := "│ bubble │" + InlineMarker(ref) + "▄▄▄"

	got, start, _, ok := ParseInlineMarker(line, 0)
	require.True(t, ok)
	assert.Equal(t, ref, got)
	assert.Equal(t, len("│ bubble │"), start)
}

func TestParseInlineMarkerRejectsMalformedPayload(t *testing.T) {
	line := inlineMarkerOpen + "12/3/4" + inlineMarkerClose + inlineMarkerDone
	_, _, next, ok := ParseInlineMarker(line, 0)
	assert.False(t, ok, "too few fields is not a marker")
	assert.Positive(t, next, "the scan still advances past the bad payload")
}

func TestParseInlineMarkerNoMarker(t *testing.T) {
	_, _, next, ok := ParseInlineMarker("just text", 0)
	assert.False(t, ok)
	assert.Zero(t, next)
}

func TestTruncateInlineLeftAdjustsBadgeCrop(t *testing.T) {
	ref := InlineRef{ID: 1, CoverCols: 20, CoverRows: 8, HOff: 2, WinCols: 12, WinRows: 6}
	line := "  " + InlineMarker(ref) + "abcdefghijkl"
	for _, tc := range []struct{ cut, left, width, offset int }{
		{1, 1, 12, 2}, // Badge ends in the contain-mode padding.
		{6, 0, 8, 6},  // Badge covers padding and four image cells.
	} {
		out := TruncateInlineLeft(line, tc.cut)
		got, start, _, ok := ParseInlineMarker(out, 0)
		require.True(t, ok)
		assert.Equal(t, tc.left, xansi.StringWidth(out[:start]))
		assert.Equal(t, tc.width, got.WinCols)
		assert.Equal(t, tc.offset, got.HOff)
		assert.Equal(t, 14-tc.cut, xansi.StringWidth(out))
	}
	out := TruncateInlineLeft(line, 14)
	_, _, _, ok := ParseInlineMarker(out, 0)
	assert.False(t, ok, "a completely covered row cannot paint an image")
}

func TestParseInlineMarkerTruncatedAndOutOfRange(t *testing.T) {
	for _, from := range []int{-1, 0, 1, 100} {
		_, _, _, ok := ParseInlineMarker("", from)
		assert.False(t, ok)
	}
	truncated := inlineMarkerOpen + "1/4/2/0/0/4/2/0" + inlineMarkerClose
	_, _, next, ok := ParseInlineMarker(truncated, 0)
	assert.False(t, ok)
	assert.LessOrEqual(t, next, len(truncated))
	_, _, _, ok = ParseInlineMarker(truncated, next)
	assert.False(t, ok)
	for _, payload := range []string{
		"1/0/2/0/0/4/2/0", "1/4/2/-1/0/4/2/0",
		"1/4/2/0/0/4/2/2", "1/4/2/2/0/4/2/0",
	} {
		_, _, _, ok := ParseInlineMarker(inlineMarkerOpen+payload+inlineMarkerClose+inlineMarkerDone, 0)
		assert.False(t, ok, payload)
	}
}

// rasterOf returns the "1;1;<w>;<h>" raster attributes SixelSeq writes.
func rasterOf(seq string) (int, int) {
	m := regexp.MustCompile(`"1;1;(\d+);(\d+)`).FindStringSubmatch(seq)
	if m == nil {
		return 0, 0
	}
	w, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	return w, h
}

func TestSixelRegionSeqWindowSize(t *testing.T) {
	img := gradientImage(120, 240)
	ref := InlineRef{ID: 1, CoverCols: 12, CoverRows: 24, WinCols: 12, WinRows: 24}

	full, err := SixelSeq(img, ref.WinCols, ref.WinRows)
	require.NoError(t, err)
	part, err := SixelRegionSeq(img, ref, 6, 6)
	require.NoError(t, err)
	require.NotEmpty(t, part)

	fw, fh := rasterOf(full)
	pw, ph := rasterOf(part)
	require.Positive(t, fw)
	assert.Equal(t, fw, pw, "a crop keeps the width")
	assert.Less(t, ph, fh, "a crop is shorter than the whole")
	assert.NotEqual(t, full, part, "the crop is not the whole image")
}

func TestSixelRegionSeqCropOffsetsDiffer(t *testing.T) {
	img := gradientImage(120, 240)
	ref := InlineRef{ID: 1, CoverCols: 12, CoverRows: 24, WinCols: 12, WinRows: 24}
	top, err := SixelRegionSeq(img, ref, 0, 6)
	require.NoError(t, err)
	bottom, err := SixelRegionSeq(img, ref, 12, 6)
	require.NoError(t, err)
	assert.NotEqual(t, top, bottom, "different rows of the image give different pixels")
}

func TestSixelRegionSeqRejectsEmptyInput(t *testing.T) {
	_, err := SixelRegionSeq(nil, InlineRef{ID: 1, CoverCols: 1, CoverRows: 1, WinCols: 1, WinRows: 1}, 0, 1)
	assert.Error(t, err)
	_, err = SixelRegionSeq(gradientImage(10, 10), InlineRef{ID: 1}, 0, 1)
	assert.Error(t, err)
}
