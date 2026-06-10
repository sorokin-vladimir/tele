package components_test

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

func kittyTailMsgs() []store.Message {
	return []store.Message{
		{ID: 1, ChatID: 1, Text: "msg 1", Date: time.Now()},
		{ID: 2, ChatID: 1, Text: "msg 2", Date: time.Now()},
		{ID: 3, ChatID: 1, Media: &store.MediaRef{Kind: store.MediaPhoto}, Photo: &store.PhotoRef{ID: 42}, Text: "the tail", Date: time.Now()},
	}
}

// photoBubbleHeight returns the rendered line count of the tail photo bubble by
// rendering a tall viewport (no trimming) and counting the bordered rows.
func photoBubbleHeight(ml *components.MessageList) int {
	ml.SetSize(80, 200)
	defer ml.SetSize(80, 200)
	lines := strings.Split(stripANSI(ml.View()), "\n")
	n := 0
	for _, l := range lines {
		if strings.Contains(l, "│") || strings.Contains(l, "╭") || strings.Contains(l, "╰") {
			n++
		}
	}
	return n
}

// Issue #115: in Kitty mode the image bytes can be known (in ml.images) while
// the placement is still being transmitted (store not Ready). The bubble reserves
// the image's full footprint immediately and draws a full-height placeholder box,
// so the new tail message is fully visible (not clipped) even before transmit.
func TestMessageList_Kitty_TailPhoto_NotTransmitted_StaysVisible(t *testing.T) {
	store0 := media.NewKittyStore()
	ml := components.NewMessageList(10, 80)
	ml.SetRenderer(media.NewKittyRenderer(store0))

	ml.SetMessages(kittyTailMsgs())
	ml.SetImage(42, image.NewRGBA(image.Rect(0, 0, 400, 400))) // bytes known, not transmitted

	out := stripANSI(ml.View())
	assert.Len(t, strings.Split(out, "\n"), 10, "viewport must always be exactly viewHeight lines")
	assert.Contains(t, out, "the tail", "new tail caption must be visible before the image is transmitted")
}

// Part A invariant: the bubble height is identical before and after the Kitty
// placement transmits — the image swaps into the already-reserved full-height
// placeholder box, so there is no scroll jump and no re-anchor is needed.
func TestMessageList_Kitty_TailPhoto_HeightStableAcrossTransmit(t *testing.T) {
	store0 := media.NewKittyStore()
	ml := components.NewMessageList(10, 80)
	ml.SetRenderer(media.NewKittyRenderer(store0))

	ml.SetMessages(kittyTailMsgs())
	ml.SetImage(42, image.NewRGBA(image.Rect(0, 0, 400, 400)))

	before := photoBubbleHeight(ml)
	store0.MarkTransmitted(42, ml.PhotoContentCols())
	after := photoBubbleHeight(ml)

	assert.Equal(t, before, after, "photo bubble height must not change when the placement transmits")
	assert.Greater(t, before, 3, "bytes-known photo must reserve its full footprint, not a 1-line placeholder")
}
