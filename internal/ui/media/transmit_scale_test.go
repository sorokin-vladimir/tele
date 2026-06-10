package media

import "testing"

func TestTransmitTargetWidth(t *testing.T) {
	tests := []struct {
		name       string
		curW, cols int
		cellW      float64
		want       int
	}{
		// Real cell size reported: size to the box width, upscaling small thumbs.
		{"upscale small thumb to box", 320, 60, 16, 960},
		{"already box width", 960, 60, 16, 0},
		{"downscale large to box", 1280, 60, 16, 960},
		{"cap absurd width", 320, 400, 16, maxTransmitPx},
		// Unknown cell size: only downscale beyond the bandwidth bound, never up.
		{"unknown cell, smaller than bound", 320, 60, 0, 0},
		{"unknown cell, larger than bound", 1000, 60, 0, 720},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := transmitTargetWidth(tt.curW, tt.cols, tt.cellW); got != tt.want {
				t.Errorf("transmitTargetWidth(%d, %d, %.0f) = %d, want %d",
					tt.curW, tt.cols, tt.cellW, got, tt.want)
			}
		})
	}
}
