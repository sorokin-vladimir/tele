package ui

import (
	"image"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/store"
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
// change must be debounced so only the final width triggers a reset+retransmit;
// otherwise overlapping async transmits land out of order and leave the Kitty
// placement at a stale size (the photo renders smaller than its grid).
func TestRetransmitTick_StaleGenerationIsIgnored(t *testing.T) {
	m := NewRootModel(nil, nil, 50, false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	// Settle the animation loops (chats loaded, a chat open) so the only command
	// under test is the retransmit, not an animation re-arm (issue #147).
	m.chatList.SetChats([]store.Chat{{ID: 1}})
	m.chat.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	// Seed one live placement so the reset has a placement to delete by id (#94).
	m.kittyStore.IDFor(500)
	m.kittyLive = map[int64]bool{500: true}
	m.retransmitGen = 2

	_, cmd := m.Update(retransmitTickMsg{gen: 1})
	require.Nil(t, cmd, "a superseded (older) debounce tick must not reset/retransmit")

	m2, cmd2 := m.Update(retransmitTickMsg{gen: 2})
	require.NotNil(t, cmd2, "the latest debounce tick must reset placements (delete live)")
	require.False(t, m2.(RootModel).kittyResetPending, "reconcile must consume the reset request")
}

// A heavy chat must not transmit every image at once (that overruns the terminal
// and corrupts placements). reconcile transmits only the on-screen images and
// keeps the live count within the cap.
func TestReconcileKitty_TransmitsOnlyVisible(t *testing.T) {
	m := NewRootModel(nil, nil, 50, false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 12) // small viewport: only a couple of photos fit

	const total = 40
	msgs := make([]store.Message, 0, total)
	for i := 0; i < total; i++ {
		pid := int64(100 + i)
		msgs = append(msgs, store.Message{
			ID: i + 1, ChatID: 1,
			Media: &store.MediaRef{Kind: store.MediaPhoto},
			Photo: &store.PhotoRef{ID: pid},
			Date:  time.Now(),
		})
		m.imageCache.Add(pid, image.NewRGBA(image.Rect(0, 0, 320, 320)))
	}
	m.chat.SetMessages(msgs)
	// Inject the shared cache so the chat's message list reads the same images.
	m.chat.SetKnownImages(m.imageCache)

	visible := m.chat.VisiblePhotoIDs()
	(&m).reconcileKittyCmd()

	require.NotEmpty(t, visible, "some image must be on screen")
	require.Less(t, len(m.kittyLive), total, "must not transmit the whole chat at once")
	require.LessOrEqual(t, len(m.kittyLive), defaultKittyPlacementCap, "live placements must stay within the cap")
	for _, id := range visible {
		require.True(t, m.kittyLive[id], "every visible image must be transmitted")
	}
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
