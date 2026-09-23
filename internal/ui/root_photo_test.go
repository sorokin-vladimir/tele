package ui

import (
	"image"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func solidImage(w, h int) image.Image {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func newSizedModel(t *testing.T) RootModel {
	t.Helper()
	m := NewRootModel(50, false)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return m2.(RootModel)
}

func TestOpenPhotoModal_UsesFullWhenCached(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.fullImageCache.Add(42, solidImage(800, 600))
	ref := domain.PhotoRef{ID: 42, FullThumbSize: "x"}
	m2, cmd := m.openPhotoModal(ref, 100, "Alice", time.Now())
	assert.NotNil(t, m2.photoViewer, "modal opens")
	assert.True(t, m2.photoViewer.full, "uses the full-quality image")
	assert.Equal(t, int64(42), m2.photoViewer.photoID)
	assert.Positive(t, m2.photoViewer.cols)
	assert.Positive(t, m2.photoViewer.rows)
	assert.Nil(t, cmd, "no download when full quality is already cached")
}

func TestOpenPhotoModal_PreviewThenDownloadsFull(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.imageCache.Add(7, solidImage(400, 300)) // inline preview only
	ref := domain.PhotoRef{ID: 7, FullThumbSize: "x"}
	m2, cmd := m.openPhotoModal(ref, 100, "Alice", time.Now())
	assert.NotNil(t, m2.photoViewer)
	assert.False(t, m2.photoViewer.full, "preview is not full quality")
	assert.NotNil(t, cmd, "a full-quality download is dispatched")
}

func TestOpenPhotoModal_SpinnerWhenNothingCached(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	ref := domain.PhotoRef{ID: 9, FullThumbSize: "x"}
	m2, _ := m.openPhotoModal(ref, 100, "Alice", time.Now())
	assert.NotNil(t, m2.photoViewer)
	assert.Nil(t, m2.photoViewer.img, "no image yet -> spinner")
}

func TestOpenPhotoModal_BuildsDateLabel(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.fullImageCache.Add(3, solidImage(100, 100))
	when := time.Date(time.Now().Year(), time.January, 2, 9, 41, 0, 0, time.Local)
	ref := domain.PhotoRef{ID: 3, FullThumbSize: "x"}
	m2, _ := m.openPhotoModal(ref, 100, "Bob", when)
	assert.Equal(t, "January 2 09:41", m2.photoViewer.timeLabel)
}

func TestClosePhotoModal_Clears(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{photoID: 1}
	m, _ = m.closePhotoModal()
	assert.Nil(t, m.photoViewer, "closing clears the overlay")
}

func TestClosePhotoModal_DeletesKittyPlacement(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeKitty
	m.photoViewer = &photoViewer{photoID: 42}
	// Simulate an open modal that transmitted its image under the stable key.
	m.imageCache.Add(photoPlayerKey, solidImage(100, 80))
	id := m.kittyStore.IDFor(photoPlayerKey)

	m2, cmd := m.closePhotoModal()
	assert.Nil(t, m2.photoViewer)
	require.NotNil(t, cmd, "closing must delete the modal's Kitty placement")
	raw, ok := cmd().(tea.RawMsg)
	require.True(t, ok)
	assert.Equal(t, media.DeleteSeq(id), raw.Msg.(string), "delete the reused placement by id (#175)")
}

func TestClosePhotoModal_NoDeleteWhenNoImageTransmitted(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeKitty
	m.photoViewer = &photoViewer{photoID: 42} // spinner only; nothing transmitted
	_, cmd := m.closePhotoModal()
	assert.Nil(t, cmd, "no placement was transmitted, so nothing to delete")
}

func TestPhotoFooterHints(t *testing.T) {
	h := photoFooterHints(false)
	assert.Contains(t, h, "external", "hints mention the external-open action")
	assert.Contains(t, h, "close", "hints mention close")
	assert.NotContains(t, h, "browse", "no browse hint for a lone photo")

	album := photoFooterHints(true)
	assert.Contains(t, album, "browse", "album photo shows the left/right browse hint")
}

func TestPhotoViewerView_RendersOverBase(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.photoViewer = &photoViewer{photoID: 3, title: "Alice", timeLabel: "Today 09:41", img: solidImage(200, 150)}
	b := m.photoViewer.img.Bounds()
	m.photoViewer.cols, m.photoViewer.rows = m.modalImageBox(b.Dx(), b.Dy(), 0)
	out := m.photoViewerView("background")
	assert.Contains(t, out, "Alice", "sender shows on the top border")
	assert.Contains(t, out, "Today 09:41", "date/time shows on the bottom border")
}

func TestHandlePhotoModalKey_EscCloses(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{photoID: 1}
	m2, _ := m.handlePhotoModalKey("esc")
	assert.Nil(t, m2.photoViewer, "esc closes the modal")
}

func TestHandlePhotoModalKey_QCloses(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{photoID: 1}
	m2, _ := m.handlePhotoModalKey("q")
	assert.Nil(t, m2.photoViewer, "q closes the modal")
}

func TestHandlePhotoModalKey_ExternalKeepsOpen(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	// No image is cached on purpose: openPhotoExternal then skips the goroutine
	// that would launch the real OS viewer, keeping the test hermetic. We only
	// assert the modal stays open on O.
	m.photoViewer = &photoViewer{photoID: 1}
	m2, _ := m.handlePhotoModalKey("O")
	assert.NotNil(t, m2.photoViewer, "O opens externally but keeps the modal open")
}

func TestHandlePhotoModalKey_OtherIsNoop(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{photoID: 1}
	m2, cmd := m.handlePhotoModalKey("x")
	assert.NotNil(t, m2.photoViewer, "unrelated keys do nothing")
	assert.Nil(t, cmd)
}

func TestHandleFullPhotoReady_SwapsOpenPhoto(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.photoViewer = &photoViewer{photoID: 5, img: solidImage(100, 80), full: false}
	m2, _ := m.handleFullPhotoReady(FullPhotoReadyMsg{PhotoID: 5, Image: solidImage(1600, 1200)})
	assert.True(t, m2.photoViewer.full, "the open photo swaps to full quality")
	assert.Positive(t, m2.photoViewer.cols)
	assert.Positive(t, m2.photoViewer.rows)
}

func TestHandleFullPhotoReady_IgnoresMismatch(t *testing.T) {
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.photoViewer = &photoViewer{photoID: 5, full: false}
	m2, _ := m.handleFullPhotoReady(FullPhotoReadyMsg{PhotoID: 999, Image: solidImage(10, 10)})
	assert.False(t, m2.photoViewer.full, "a different photo does not touch the modal")
}

func TestUpdatePhotoSpinner_AdvancesWhileLoading(t *testing.T) {
	m := newSizedModel(t)
	m.photoViewer = &photoViewer{photoID: 1, img: nil, spinnerIdx: 0}
	m.updatePhotoSpinner()
	assert.Equal(t, 1, m.photoViewer.spinnerIdx, "spinner advances while no image is shown")
	m.photoViewer.img = solidImage(4, 3)
	m.updatePhotoSpinner()
	assert.Equal(t, 1, m.photoViewer.spinnerIdx, "spinner stops once an image is shown")
}

func TestModalPagingWithinAlbum(t *testing.T) {
	album := []components.GroupMediaRef{
		{Index: 1, Kind: domain.MediaPhoto, Photo: &domain.PhotoRef{ID: 11}, MsgID: 1},
		{Index: 2, Kind: domain.MediaPhoto, Photo: &domain.PhotoRef{ID: 22}, MsgID: 2},
	}
	m := newSizedModel(t)
	m.imageMode = media.ModeBlocks
	m.imageCache.Add(11, solidImage(400, 300))
	m.imageCache.Add(22, solidImage(400, 300))

	m, _ = m.openPhotoModalAlbum(*album[0].Photo, album[0].MsgID, "", time.Time{}, album, 0)
	require.NotNil(t, m.photoViewer)
	assert.Equal(t, 0, m.photoViewer.albumIdx)

	m, _ = m.pageModal(1)
	require.NotNil(t, m.photoViewer)
	assert.Equal(t, 1, m.photoViewer.albumIdx)
	assert.Equal(t, int64(22), m.photoViewer.photoID)

	// Paging past the end is a no-op (stays on the last part).
	m, _ = m.pageModal(1)
	require.NotNil(t, m.photoViewer)
	assert.Equal(t, 1, m.photoViewer.albumIdx)

	// Paging back returns to the first part.
	m, _ = m.pageModal(-1)
	require.NotNil(t, m.photoViewer)
	assert.Equal(t, 0, m.photoViewer.albumIdx)
	assert.Equal(t, int64(11), m.photoViewer.photoID)
}
