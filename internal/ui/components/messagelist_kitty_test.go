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

// Issue #115: in Kitty mode the image bytes can be known (in ml.images) while
// the placement is still being transmitted (store not Ready). msgHeight must not
// reserve the full footprint then, or positionAtBottom parks the viewport inside
// lines the renderer only draws as a 1-line placeholder, hiding the new tail.
func TestMessageList_Kitty_TailPhoto_NotTransmitted_StaysVisible(t *testing.T) {
	store0 := media.NewKittyStore()
	ml := components.NewMessageList(10, 80)
	ml.SetRenderer(media.NewKittyRenderer(store0))

	ml.SetMessages(kittyTailMsgs())
	ml.SetImage(42, image.NewRGBA(image.Rect(0, 0, 400, 400))) // bytes known, not transmitted

	out := stripANSI(ml.View())
	assert.Len(t, strings.Split(out, "\n"), 10, "viewport must always be exactly viewHeight lines")
	assert.Contains(t, out, "the tail", "new tail message must be visible before the image is transmitted")
	assert.True(t, ml.AtBottom(), "viewport must report at-bottom while the placeholder is shown")
}

// Issue #115: once the placement transmits, the photo grows to its full
// footprint, moving the natural bottom down. The root handler re-anchors using
// AtBottom (captured before) + ScrollToBottom (after). This verifies those
// primitives keep the new tail fully visible across the height growth.
func TestMessageList_Kitty_TailPhoto_TransmitReanchorsToBottom(t *testing.T) {
	store0 := media.NewKittyStore()
	ml := components.NewMessageList(10, 80)
	ml.SetRenderer(media.NewKittyRenderer(store0))

	ml.SetMessages(kittyTailMsgs())
	ml.SetImage(42, image.NewRGBA(image.Rect(0, 0, 400, 400)))

	// Mirror RootModel.kittyTransmittedMsg: capture at-bottom with pre-growth
	// heights, mark transmitted (heights grow), then re-anchor.
	wasAtBottom := ml.AtBottom()
	store0.MarkTransmitted(42, ml.PhotoContentCols())
	if wasAtBottom {
		ml.ScrollToBottom()
	}

	out := stripANSI(ml.View())
	lines := strings.Split(out, "\n")
	assert.Len(t, lines, 10, "viewport must be exactly viewHeight lines")
	assert.Contains(t, out, "the tail", "newest message caption must be visible after the photo expands")
	// The very last viewport line is the tail bubble's bottom border, proving the
	// true tail is reached rather than clipped above the fold.
	assert.Contains(t, lines[len(lines)-1], "╰", "last line must be the tail bubble's bottom border")
}

// Guards the contract that CanRender is exactly "Render(...) != nil"; the height
// calc relies on this equivalence to stay in lock-step with renderMessage.
func TestKittyRenderer_CanRender_MatchesRenderNil(t *testing.T) {
	store0 := media.NewKittyStore()
	r := media.NewKittyRenderer(store0)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	const cols = 40

	assert.False(t, r.CanRender(42, cols))
	assert.Nil(t, r.Render(42, img, cols), "Render must be nil while CanRender is false")

	store0.MarkTransmitted(42, cols)
	assert.True(t, r.CanRender(42, cols))
	assert.NotNil(t, r.Render(42, img, cols), "Render must be non-nil once CanRender is true")
}
