package ui

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func solidFrame(w, h int) image.Image { return image.NewNRGBA(image.Rect(0, 0, w, h)) }

func TestHandleGifFramesReady_CachesAndStarts(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.gifActiveID = 77 // doc 77 is the pending selection
	frames := []image.Image{solidFrame(4, 4), solidFrame(4, 4)}

	nm, _ := m.handleGifFramesReady(gifFramesReadyMsg{docID: 77, frames: frames})
	assert.Len(t, nm.gifFrames[77], 2, "frames must be cached by doc id")
}

func TestHandleGifTick_AdvancesAndWraps(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.gifFrames[77] = []image.Image{solidFrame(4, 4), solidFrame(4, 4)}
	m.gifActiveID = 77
	m.gifIdx = 0
	m.gifGen = 5

	nm, _ := m.handleGifTick(gifTickMsg{gen: 5})
	assert.Equal(t, 1, nm.gifIdx, "tick advances the frame index")

	nm.gifIdx = 1
	nm2, _ := nm.handleGifTick(gifTickMsg{gen: 5})
	assert.Equal(t, 0, nm2.gifIdx, "index wraps to 0 at the end")
}

func TestEnsureGifAnimForSelection_NoopWhenAlreadyActive(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.chat.SetMessages([]domain.Message{{
		ID:       1,
		Media:    &domain.MediaRef{Kind: domain.MediaGIF},
		Document: &domain.DocumentRef{ID: 55, ThumbSize: "m"},
	}})
	m.gifActiveID = 55 // already animating/downloading this gif

	nm, cmd := m.ensureGifAnimForSelection()
	assert.Equal(t, int64(55), nm.gifActiveID, "must not restart the active gif")
	assert.Nil(t, cmd, "no-op when the selected gif is already active")
}

func TestEnsureGifAnimForSelection_NoopForNonGif(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.chat.SetMessages([]domain.Message{{
		ID:    1,
		Media: &domain.MediaRef{Kind: domain.MediaPhoto},
		Photo: &domain.PhotoRef{ID: 9},
	}})

	nm, cmd := m.ensureGifAnimForSelection()
	assert.Equal(t, int64(0), nm.gifActiveID)
	assert.Nil(t, cmd, "no-op when the selection is not a gif")
}

func TestOpenChat_ClearsGifFrames(t *testing.T) {
	m := NewRootModel(50, false)
	m.gifFrames[77] = []image.Image{solidFrame(4, 4), solidFrame(4, 4)}

	// Switching chats must drop decoded frames so the memory is released and
	// does not accumulate across chats.
	nm, _ := m.Update(screens.OpenChatMsg{ChatID: 9})
	rm := nm.(RootModel)
	assert.Empty(t, rm.gifFrames, "switching chats must clear the gif frame cache")
}

func TestDecodeGifCmd_RemovesTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.mp4")
	require.NoError(t, os.WriteFile(path, []byte("not really a video"), 0600))

	// Run the decode command; ffmpeg will reject the bogus file, but the temp
	// file must be removed regardless so downloaded GIFs don't pile up on disk.
	_ = decodeGifCmd(context.Background(), 1, path, 16, 16)()

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "temp file must be removed after decode")
}

func TestStopGifAnim_ResetsAndBumpsGen(t *testing.T) {
	m := NewRootModel(50, false)
	m.gifFrames[77] = []image.Image{solidFrame(4, 4), solidFrame(4, 4)}
	m.gifActiveID = 77
	m.gifIdx = 3
	genBefore := m.gifGen

	m.stopGifAnim()
	assert.Equal(t, int64(0), m.gifActiveID, "stop clears the active id")
	assert.Equal(t, 0, m.gifIdx, "stop resets the index")
	assert.Greater(t, m.gifGen, genBefore, "stop bumps the generation")
}

func TestHandleGifTick_StaleGenIgnored(t *testing.T) {
	m := NewRootModel(50, false)
	m.imageMode = media.ModeKitty
	m.gifFrames[77] = []image.Image{solidFrame(4, 4), solidFrame(4, 4)}
	m.gifActiveID = 77
	m.gifIdx = 0
	m.gifGen = 6

	nm, cmd := m.handleGifTick(gifTickMsg{gen: 5}) // stale
	assert.Equal(t, 0, nm.gifIdx, "stale tick must not advance")
	assert.Nil(t, cmd, "stale tick must not re-arm")
}
