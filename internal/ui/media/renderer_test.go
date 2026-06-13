package media_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

func sampleImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	return img
}

func TestBlockRenderer_ImplementsRenderer(t *testing.T) {
	var _ media.Renderer = media.NewBlockRenderer()
}

func TestBlockRenderer_RenderMatchesRenderBlockArt(t *testing.T) {
	img := sampleImage(8, 8)
	r := media.NewBlockRenderer()
	got := r.Render(42, img, 6)
	want := media.RenderBlockArt(img, 6)
	require.Equal(t, want, got)
}

func TestBlockRenderer_CacheReturnsSameSlice(t *testing.T) {
	img := sampleImage(8, 8)
	r := media.NewBlockRenderer()
	first := r.Render(42, img, 6)
	second := r.Render(42, img, 6)
	// Same backing array => cache hit (no re-render).
	require.Equal(t, &first[0], &second[0])
}

func TestBlockRenderer_ResetClearsCache(t *testing.T) {
	img := sampleImage(8, 8)
	r := media.NewBlockRenderer()
	first := r.Render(42, img, 6)
	r.Reset()
	second := r.Render(42, img, 6)
	require.NotSame(t, &first[0], &second[0])
}

func TestPhotoRows_CapsTallImages(t *testing.T) {
	// A very tall, narrow image would otherwise need >297 rows, overflowing the
	// Kitty diacritic table. The cap keeps it bounded and addressable.
	rows := media.PhotoRows(10, 100000, 60, 2.0)
	require.LessOrEqual(t, rows, 256)
	require.Greater(t, rows, 0)
}

func TestPhotoRows_TallerCellsReserveFewerRows(t *testing.T) {
	// Taller cells (larger height/width) reserve FEWER rows for the same image,
	// otherwise the picture is shorter than its reserved box (gap / "smaller").
	base := media.PhotoRows(40, 30, 60, 2.0)
	taller := media.PhotoRows(40, 30, 60, 2.5)
	require.Less(t, taller, base, "taller cells must reserve fewer rows")
}

func TestKittyRenderer_TallImageStaysWithinDiacritics(t *testing.T) {
	store := media.NewKittyStore()
	r := media.NewKittyRenderer(store)
	img := sampleImage(10, 100000) // aspect ~1:10000
	cols := 60
	store.MarkTransmitted(1, cols)
	lines := r.Render(1, img, cols)
	require.Len(t, lines, media.PhotoRows(10, 100000, cols, media.CellAspect()))
	require.LessOrEqual(t, len(lines), 256, "rows must stay within the diacritic table")
}

func TestRenderers_FootprintParity(t *testing.T) {
	store := media.NewKittyStore()
	kr := media.NewKittyRenderer(store)
	br := media.NewBlockRenderer()
	dims := [][2]int{{20, 10}, {10, 20}, {16, 16}, {33, 50}}
	cols := 12
	for i, d := range dims {
		// Distinct ids per image: the cache keys on (photoID, cols) and assumes
		// a photoID identifies one specific image.
		kID := int64(1000 + i)
		bID := int64(2000 + i)
		img := sampleImage(d[0], d[1])
		store.MarkTransmitted(kID, cols)
		k := kr.Render(kID, img, cols)
		b := br.Render(bID, img, cols)
		want := media.PhotoRows(d[0], d[1], cols, media.CellAspect())
		require.Len(t, k, want, "kitty footprint")
		require.Len(t, b, want, "block footprint")
	}
}
