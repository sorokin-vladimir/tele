package ui_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gotd/td/tgerr"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// downloadCompletionText runs a command (possibly batched) and returns the
// fileDownloadDoneMsg text it produces, if any.
func downloadCompletionText(cmd tea.Cmd) (string, bool) {
	if cmd == nil {
		return "", false
	}
	for _, msg := range drainMsgs(cmd()) {
		if text, _, ok := ui.FileDownloadDoneTextForTest(msg); ok {
			return text, true
		}
	}
	return "", false
}

func TestDownloadKey_OnPhoto_SavesFullQualityJpg(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	mc := &mockTGClient{
		downloadPhotoFileFunc: func(_ store.PhotoRef, dst io.Writer) error {
			_, err := io.WriteString(dst, "jpeg")
			return err
		},
	}
	m, st := newRootOnChat(t, mc)

	photo := store.Message{ID: 10, ChatID: 1, Date: time.Now(),
		Media: &store.MediaRef{Kind: store.MediaPhoto},
		Photo: &store.PhotoRef{ID: 321, FullThumbSize: "y"}}
	st.AppendMessage(photo)
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View() // lay out the message list so the photo becomes the selection

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "pressing s on a photo must start a download")
	assert.Contains(t, text, filepath.Join(dir, "photo_321.jpg"))
}

func TestDownloadKey_OnVideo_SavesSynthesizedName(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	mc := &mockTGClient{
		downloadDocFileFunc: func(dst io.Writer) error {
			_, err := io.WriteString(dst, "mp4")
			return err
		},
	}
	m, st := newRootOnChat(t, mc)

	video := store.Message{ID: 11, ChatID: 1, Date: time.Now(),
		Media:    &store.MediaRef{Kind: store.MediaVideo},
		Document: &store.DocumentRef{ID: 654, MimeType: "video/mp4"}}
	st.AppendMessage(video)
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "pressing s on a video must start a download")
	assert.Contains(t, text, filepath.Join(dir, "video_654.mp4"))
}

// The context-menu Download request must be routed (it was previously dropped in
// the root update switch).
func TestDownloadFileRequest_RoutedForPhoto(t *testing.T) {
	dir := t.TempDir()
	defer ui.SetDownloadsDirForTest(dir)()

	mc := &mockTGClient{
		downloadPhotoFileFunc: func(_ store.PhotoRef, dst io.Writer) error {
			_, err := io.WriteString(dst, "jpeg")
			return err
		},
	}
	m, st := newRootOnChat(t, mc)

	photo := store.Message{ID: 12, ChatID: 1, Date: time.Now(),
		Media: &store.MediaRef{Kind: store.MediaPhoto},
		Photo: &store.PhotoRef{ID: 999}}
	st.AppendMessage(photo)
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(components.DownloadFileRequest{})
	text, ok := downloadCompletionText(cmd)
	require.True(t, ok, "DownloadFileRequest must be routed to a download")
	assert.Contains(t, text, filepath.Join(dir, "photo_999.jpg"))
}

// openDocumentCmd must stream the document straight to a temp file (never
// buffering it) and hand that path to the OS launcher.
func TestOpenDocumentCmd_StreamsToTempFile(t *testing.T) {
	tmpDir := t.TempDir()
	const body = "the entire document body"

	client := &mockTGClient{
		downloadDocFileFunc: func(dst io.Writer) error {
			_, err := io.WriteString(dst, body)
			return err
		},
	}

	var opened string
	restore := ui.SetOpenPathForTest(func(name string) { opened = name })
	defer restore()

	ref := store.DocumentRef{ID: 7, FileName: "clip.mp4"}
	msg := ui.OpenDocumentCmdForTest(client, store.Peer{ID: 1}, 99, ref, tmpDir)()

	errText, ok := ui.DocumentOpenErrTextForTest(msg)
	require.True(t, ok, "completion must be a documentOpenDoneMsg")
	assert.Empty(t, errText, "successful open reports no error")
	require.NotEmpty(t, opened, "OS launcher must be invoked with the temp path")
	assert.Equal(t, tmpDir, filepath.Dir(opened), "temp file lives in tmpDir")
	assert.Equal(t, ".mp4", filepath.Ext(opened))

	data, err := os.ReadFile(opened)
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

// On FILE_REFERENCE_EXPIRED the partial first attempt must be discarded: the
// file is truncated before the retry so the result is exactly the retry's data.
func TestOpenDocumentCmd_TruncatesOnRetry(t *testing.T) {
	tmpDir := t.TempDir()
	const fresh = "fresh"

	calls := 0
	client := &mockTGClient{
		downloadDocFileFunc: func(dst io.Writer) error {
			calls++
			if calls == 1 {
				// Simulate a partial write before the reference expires.
				_, _ = io.WriteString(dst, "stale-partial-bytes")
				return &tgerr.Error{Code: 400, Type: "FILE_REFERENCE_EXPIRED"}
			}
			_, err := io.WriteString(dst, fresh)
			return err
		},
		refreshFunc: func(int) (store.Message, error) {
			return store.Message{Document: &store.DocumentRef{ID: 7, FileName: "clip.mp4"}}, nil
		},
	}

	var opened string
	restore := ui.SetOpenPathForTest(func(name string) { opened = name })
	defer restore()

	ref := store.DocumentRef{ID: 7, FileName: "clip.mp4"}
	msg := ui.OpenDocumentCmdForTest(client, store.Peer{ID: 1}, 99, ref, tmpDir)()

	assert.Equal(t, 2, calls, "must retry once after refresh")
	errText, ok := ui.DocumentOpenErrTextForTest(msg)
	require.True(t, ok, "completion must be a documentOpenDoneMsg")
	assert.Empty(t, errText, "retry succeeds, so no error reported")
	require.NotEmpty(t, opened)

	data, err := os.ReadFile(opened)
	require.NoError(t, err)
	assert.Equal(t, fresh, string(data), "stale partial bytes must be truncated away")
}

// A download failure must report the error text on the completion message so the
// root handler can surface it (and clear the indicator).
func TestOpenDocumentCmd_FailureReportsErrText(t *testing.T) {
	tmpDir := t.TempDir()
	client := &mockTGClient{
		downloadDocFileFunc: func(io.Writer) error {
			return assert.AnError
		},
	}

	restore := ui.SetOpenPathForTest(func(string) {})
	defer restore()

	ref := store.DocumentRef{ID: 7, FileName: "clip.mp4"}
	msg := ui.OpenDocumentCmdForTest(client, store.Peer{ID: 1}, 99, ref, tmpDir)()

	errText, ok := ui.DocumentOpenErrTextForTest(msg)
	require.True(t, ok, "completion must be a documentOpenDoneMsg")
	assert.NotEmpty(t, errText, "failed open must report an error")
}

func TestDownloadFileCmd_SavesToDir(t *testing.T) {
	dir := t.TempDir()
	const body = "the file body"
	client := &mockTGClient{
		downloadDocFileFunc: func(dst io.Writer) error {
			_, err := io.WriteString(dst, body)
			return err
		},
	}

	ref := store.DocumentRef{ID: 7, FileName: "report.pdf"}
	msg := ui.DownloadFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	text, sev, ok := ui.FileDownloadDoneTextForTest(msg)
	require.True(t, ok, "completion must be a fileDownloadDoneMsg")
	assert.Equal(t, components.SeverityInfo, sev)
	assert.Contains(t, text, filepath.Join(dir, "report.pdf"))

	data, err := os.ReadFile(filepath.Join(dir, "report.pdf"))
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

func TestDownloadFileCmd_CollisionGetsSuffix(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("old"), 0644))

	client := &mockTGClient{
		downloadDocFileFunc: func(dst io.Writer) error {
			_, err := io.WriteString(dst, "new")
			return err
		},
	}
	ref := store.DocumentRef{ID: 7, FileName: "report.pdf"}
	msg := ui.DownloadFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	text, _, ok := ui.FileDownloadDoneTextForTest(msg)
	require.True(t, ok)
	assert.Contains(t, text, "report (1).pdf")
}

func TestDownloadPhotoFileCmd_SavesJpg(t *testing.T) {
	dir := t.TempDir()
	const body = "\xff\xd8\xff raw jpeg bytes"
	client := &mockTGClient{
		downloadPhotoFileFunc: func(_ store.PhotoRef, dst io.Writer) error {
			_, err := io.WriteString(dst, body)
			return err
		},
	}

	ref := store.PhotoRef{ID: 42}
	msg := ui.DownloadPhotoFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	text, sev, ok := ui.FileDownloadDoneTextForTest(msg)
	require.True(t, ok, "completion must be a fileDownloadDoneMsg")
	assert.Equal(t, components.SeverityInfo, sev)
	assert.Contains(t, text, filepath.Join(dir, "photo_42.jpg"))

	data, err := os.ReadFile(filepath.Join(dir, "photo_42.jpg"))
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

// The saved photo must be the full-quality size: the command requests
// ref.FullThumbSize, not the small inline ThumbSize.
func TestDownloadPhotoFileCmd_UsesFullQuality(t *testing.T) {
	dir := t.TempDir()
	var gotRef store.PhotoRef
	client := &mockTGClient{
		downloadPhotoFileFunc: func(ref store.PhotoRef, dst io.Writer) error {
			gotRef = ref
			_, err := io.WriteString(dst, "x")
			return err
		},
	}

	ref := store.PhotoRef{ID: 42, ThumbSize: "m", FullThumbSize: "y"}
	_ = ui.DownloadPhotoFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	assert.Equal(t, "y", gotRef.ThumbSize, "must download the full-quality size")
}

func TestDownloadPhotoFileCmd_FailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	client := &mockTGClient{
		downloadPhotoFileFunc: func(_ store.PhotoRef, _ io.Writer) error { return assert.AnError },
	}
	ref := store.PhotoRef{ID: 42}
	msg := ui.DownloadPhotoFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	text, sev, ok := ui.FileDownloadDoneTextForTest(msg)
	require.True(t, ok)
	assert.Equal(t, components.SeverityWarning, sev)
	assert.NotEmpty(t, text)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "partial download must be removed")
}

func TestDownloadFileCmd_FailureLeavesNoFile(t *testing.T) {
	dir := t.TempDir()
	client := &mockTGClient{
		downloadDocFileFunc: func(io.Writer) error { return assert.AnError },
	}
	ref := store.DocumentRef{ID: 7, FileName: "report.pdf"}
	msg := ui.DownloadFileCmdForTest(client, store.Peer{ID: 1}, 99, ref, dir)()

	text, sev, ok := ui.FileDownloadDoneTextForTest(msg)
	require.True(t, ok)
	assert.Equal(t, components.SeverityWarning, sev)
	assert.NotEmpty(t, text)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "partial download must be removed")
}
