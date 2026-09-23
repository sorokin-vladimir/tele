package ui

import (
	"image"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/require"
)

// Regression: when a photo above the newest message finishes downloading, its
// bubble grows from a one-line placeholder to the full image footprint. If the
// viewport was pinned to the bottom it must re-anchor so the newest message
// stays visible.
//
// The production path (PhotoReadyMsg) adds the image to the shared cache before
// calling SetImage. Because the chat's message list reads that same cache, the
// bubble had already grown by the time SetImage took its "was I at the bottom?"
// snapshot — so the snapshot measured post-growth heights, decided we were no
// longer at the bottom, and skipped the re-anchor, letting the newest message
// scroll out of view.
func TestRoot_PhotoReady_AtBottom_KeepsNewestVisible(t *testing.T) {
	m := NewRootModel(50, false)
	m.screen = ScreenMain
	m.chat.SetSize(80, 12)
	msgs := []domain.Message{
		{ID: 1, ChatID: 1, Text: "oldest", Date: time.Now()},
		{ID: 2, ChatID: 1, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 42}, Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "newest", Date: time.Now()},
	}
	m.chat.SetMessages(msgs)
	m.chat.SetKnownImages(m.imageCache) // share the cache, as opening a chat does

	// Before the photo loads everything fits and the bottom is visible.
	before := m.chat.ScrollInfo()
	require.GreaterOrEqual(t, before.Offset+before.Visible, before.Total)

	// Photo finishes downloading: this adds to the shared cache and re-anchors.
	big := image.NewRGBA(image.Rect(0, 0, 320, 320))
	m2, _ := m.Update(PhotoReadyMsg{PhotoID: 42, Image: big})
	rm := m2.(RootModel)

	after := rm.chat.ScrollInfo()
	require.Greater(t, after.Total, after.Visible,
		"the photo must have grown the content past the viewport")
	require.GreaterOrEqual(t, after.Offset+after.Visible, after.Total,
		"newest message must stay visible (viewport re-anchored to the new bottom)")
}
