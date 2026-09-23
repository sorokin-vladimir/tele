package ui

import (
	"image"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// In Kitty mode the placeholder grid must not be advertised as ready before the
// image's virtual placement has actually been transmitted to the terminal.
// Otherwise the placeholders can be painted before the placement exists, which
// renders them mispositioned until a later repaint happens to correct it — the
// intermittent inline-photo gap.
func TestTransmitPhoto_NotReadyBeforePlacementTransmitted(t *testing.T) {
	m := NewRootModel(50, false)
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

// A failed encode/transmit must not advance to marking the image ready: the
// async command returns nil (no message), so no placement is written and the
// store never reports the image ready. Otherwise the cell stays permanently
// blank with the store falsely claiming Ready (#95).
func TestTransmitPhoto_EncodeFailureEmitsNoMessage(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)

	// A degenerate 0x0 image makes the PNG encode inside TransmitSeq fail.
	bad := image.NewRGBA(image.Rect(0, 0, 0, 0))
	cmd := m.transmitPhotoCmd(7, bad)
	require.NotNil(t, cmd)

	require.Nil(t, cmd(), "a failed encode must not emit a message that marks the image ready")
	require.False(t, m.kittyStore.Ready(7, 0), "store must not report a non-transmitted image ready")
}

// A successful encode emits kittyEncodedMsg carrying the placement sequence, so
// the update loop can write it to the terminal and then mark the image ready.
func TestTransmitPhoto_SuccessEmitsEncodedWithSeq(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)

	good := image.NewRGBA(image.Rect(0, 0, 320, 214))
	cmd := m.transmitPhotoCmd(8, good)
	require.NotNil(t, cmd)

	enc, ok := cmd().(kittyEncodedMsg)
	require.True(t, ok, "a successful encode emits kittyEncodedMsg")
	require.Equal(t, int64(8), enc.photoID)
	require.NotEmpty(t, enc.seq)
}

// During a resize the photo width changes many times in quick succession. Each
// change must be debounced so only the final width triggers a reset+retransmit;
// otherwise overlapping async transmits land out of order and leave the Kitty
// placement at a stale size (the photo renders smaller than its grid).
func TestRetransmitTick_StaleGenerationIsIgnored(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 24)
	// Settle the animation loops (chats loaded, a chat open) so the only command
	// under test is the retransmit, not an animation re-arm (issue #147).
	m.chatList.SetWindow(0, 1, []project.ChatRow{{ID: 1}})
	m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
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
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.screen = ScreenMain
	m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	m.chat.SetSize(80, 12) // small viewport: only a couple of photos fit

	const total = 40
	msgs := make([]domain.Message, 0, total)
	for i := 0; i < total; i++ {
		pid := int64(100 + i)
		msgs = append(msgs, domain.Message{
			ID: i + 1, ChatID: 1,
			Media: &domain.MediaRef{Kind: domain.MediaPhoto},
			Photo: &domain.PhotoRef{ID: pid},
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

// Extension derivation moved to the owner in #196; TestExtFromMime in
// internal/core covers it.
