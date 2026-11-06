package media

import "testing"

func BenchmarkSixelLargeModal(b *testing.B) {
	SetCellPx(10, 23)
	defer SetCellPx(0, 0)
	img := gradientImage(1024, 768)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq, err := SixelSeq(img, 330, 81)
		if err != nil {
			b.Fatal(err)
		}
		w, h := rasterOf(seq)
		if w != 3300 || h != 1863 {
			b.Fatalf("raster %dx%d", w, h)
		}
		b.ReportMetric(float64(len(seq)), "output-bytes")
	}
}
