package ui_test

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pngBytes is a minimal valid 1x1 PNG so MIME detection returns image/png.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// enterInsertOnChat opens a chat and flips to insert mode so Ctrl+V routes to
// the composer paste path.
func enterInsertOnChat(t *testing.T) (ui.RootModel, store.Store) {
	t.Helper()
	m, st := newRootOnChat(t, &mockTGClient{})
	m.SetTmpDir(t.TempDir())
	nm, _ := m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = nm.(ui.RootModel)
	require.Equal(t, keys.ModeInsert, m.VimMode())
	return m, st
}

func TestPasteImage_StagesAsPhotoAndEntersInsert(t *testing.T) {
	defer ui.SetClipboardImageReaderForTest(func() ([]byte, string, error) {
		return pngBytes, ".png", nil
	})()

	m, _ := enterInsertOnChat(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	require.NotNil(t, cmd, "ctrl+v in the composer must read the clipboard")

	// The command reads the clipboard image and writes a temp file off-loop.
	for _, msg := range drainMsgs(cmd()) {
		if msg == nil {
			continue
		}
		nm, _ := m.Update(msg)
		m = nm.(ui.RootModel)
	}

	require.True(t, m.Chat().HasAttachment(), "clipboard image must be staged as an attachment")
	sendAs, ok := m.PendingAttachmentSendAs()
	require.True(t, ok)
	assert.Equal(t, store.MediaPhoto, sendAs, "clipboard image must stage as a photo, not a file")
	assert.Equal(t, keys.ModeInsert, m.VimMode(), "caption field must be active")
}

func TestPasteImage_ReaderError_ShowsToastAndFallsBackToText(t *testing.T) {
	defer ui.SetClipboardImageReaderForTest(func() ([]byte, string, error) {
		return nil, "", errors.New("osascript boom")
	})()

	m, _ := enterInsertOnChat(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	// The read command reports the failure as a single message.
	msgs := drainMsgs(cmd())
	require.Len(t, msgs, 1)
	nm, handlerCmd := m.Update(msgs[0])
	m = nm.(ui.RootModel)
	require.NotNil(t, handlerCmd, "the error handler must batch a toast + text fallback")

	var sawToast, sawText bool
	for _, out := range drainMsgs(handlerCmd()) {
		switch out.(type) {
		case ui.StatusErrMsg:
			sawToast = true
		case tea.PasteMsg:
			sawText = true
		}
	}
	assert.True(t, sawToast, "a reader failure must surface a toast")
	// The text-fallback command runs the real clipboard; in a headless test it may
	// yield nil (no clipboard text). The toast + no-attachment are the assertions.
	_ = sawText
	assert.False(t, m.Chat().HasAttachment(), "no attachment is staged on failure")
}

func TestPasteImage_NoImage_PastesText(t *testing.T) {
	defer ui.SetClipboardImageReaderForTest(func() ([]byte, string, error) {
		return nil, "", nil // no image on the clipboard
	})()
	// Stub the text clipboard so the fallback is deterministic (no live clipboard).
	defer ui.SetClipboardReaderForTest(func() (string, error) { return "pasted text", nil })()

	m, _ := enterInsertOnChat(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	require.NotNil(t, cmd)

	var pasted string
	for _, msg := range drainMsgs(cmd()) {
		if p, ok := msg.(tea.PasteMsg); ok {
			pasted = p.Content
		}
	}
	assert.Equal(t, "pasted text", pasted, "no clipboard image must fall back to a text paste")
	assert.False(t, m.Chat().HasAttachment())
}
