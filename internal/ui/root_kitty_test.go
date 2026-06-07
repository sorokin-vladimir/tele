package ui

import (
	"image"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// In Kitty mode the placeholder grid must not be advertised as ready before the
// image's virtual placement has actually been transmitted to the terminal.
// Otherwise the placeholders can be painted before the placement exists, which
// renders them mispositioned until a later repaint happens to correct it — the
// intermittent inline-photo gap.
func TestTransmitPhoto_NotReadyBeforePlacementTransmitted(t *testing.T) {
	m := NewRootModel(nil, nil, 50, false)
	m.imageMode = media.ModeKitty
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	img := image.NewRGBA(image.Rect(0, 0, 320, 214))

	m2, _ := m.Update(PhotoReadyMsg{PhotoID: 7, Image: img})
	rm := m2.(RootModel)
	cols := rm.chat.PhotoContentCols()
	require.False(t, rm.kittyStore.Ready(7, cols),
		"must not be optimistically ready before the placement is transmitted")

	m3, _ := rm.Update(kittyTransmittedMsg{photoID: 7, cols: cols})
	rm3 := m3.(RootModel)
	require.True(t, rm3.kittyStore.Ready(7, cols),
		"ready once the placement has been transmitted")
}

// During a resize the photo width changes many times in quick succession. Each
// change must be debounced so only the final width triggers a retransmit;
// otherwise overlapping async transmits land out of order and leave the Kitty
// placement at a stale size (the photo renders smaller than its grid).
func TestRetransmitTick_StaleGenerationIsIgnored(t *testing.T) {
	m := NewRootModel(nil, nil, 50, false)
	m.imageMode = media.ModeKitty
	m.retransmitGen = 2

	_, cmd := m.Update(retransmitTickMsg{gen: 1})
	require.Nil(t, cmd, "a superseded (older) debounce tick must not retransmit")

	_, cmd2 := m.Update(retransmitTickMsg{gen: 2})
	require.NotNil(t, cmd2, "the latest debounce tick must trigger the retransmit")
}

func TestExtFromMime(t *testing.T) {
	cases := map[string]string{
		"video/quicktime":   ".mov",
		"video/webm":        ".webm",
		"video/x-matroska":  ".mkv",
		"video/mp4":         ".mp4",
		"application/weird": ".mp4", // default container for Telegram video
	}
	for mime, want := range cases {
		require.Equal(t, want, extFromMime(mime), mime)
	}
}
