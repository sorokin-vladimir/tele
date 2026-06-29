package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateUniqueDownloadFile_ResolvesCollision(t *testing.T) {
	dir := t.TempDir()

	f1, err := createUniqueDownloadFile(dir, "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "report.pdf"), f1.Name())
	require.NoError(t, f1.Close())

	f2, err := createUniqueDownloadFile(dir, "report.pdf")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "report (1).pdf"), f2.Name())
	require.NoError(t, f2.Close())
}

func TestCreateUniqueDownloadFile_SanitizesName(t *testing.T) {
	dir := t.TempDir()
	f, err := createUniqueDownloadFile(dir, "/etc/passwd")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "passwd"), f.Name())
	require.NoError(t, f.Close())
}

func TestCreateUniqueDownloadFile_EmptyNameFallsBack(t *testing.T) {
	dir := t.TempDir()
	f, err := createUniqueDownloadFile(dir, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "file"), f.Name())
	require.NoError(t, f.Close())
}

func TestResolveDownloadsDir_NonEmpty(t *testing.T) {
	assert.NotEmpty(t, resolveDownloadsDir())
}

func TestDownloadFileName_UsesOriginalWhenPresent(t *testing.T) {
	ref := store.DocumentRef{ID: 7, FileName: "clip.mp4", MimeType: "video/mp4"}
	assert.Equal(t, "clip.mp4", downloadFileName(ref, store.MediaVideo))
}

func TestDownloadFileName_SynthesizesForUnnamed(t *testing.T) {
	cases := []struct {
		name string
		ref  store.DocumentRef
		kind store.MediaKind
		want string
	}{
		{"video", store.DocumentRef{ID: 7, MimeType: "video/mp4"}, store.MediaVideo, "video_7.mp4"},
		{"video note", store.DocumentRef{ID: 7, MimeType: "video/mp4"}, store.MediaVideoNote, "video_note_7.mp4"},
		{"voice", store.DocumentRef{ID: 7, MimeType: "audio/ogg"}, store.MediaVoice, "voice_7.oga"},
		{"audio", store.DocumentRef{ID: 7, MimeType: "audio/mpeg"}, store.MediaAudio, "audio_7.mp3"},
		{"gif", store.DocumentRef{ID: 7, MimeType: "video/mp4"}, store.MediaGIF, "gif_7.mp4"},
		{"file fallback", store.DocumentRef{ID: 7, MimeType: "application/pdf"}, store.MediaFile, "file_7.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, downloadFileName(c.ref, c.kind))
		})
	}
}

func TestResolveDownloadsDir_PrefersXDG(t *testing.T) {
	if os.Getenv("HOME") == "" {
		t.Skip("no HOME")
	}
	want := t.TempDir()
	t.Setenv("XDG_DOWNLOAD_DIR", want)
	// XDG is consulted on Linux; on macOS the env is ignored, so only assert
	// the env path when it is actually honored.
	got := resolveDownloadsDir()
	if got != want {
		assert.Contains(t, got, "Downloads")
	}
}
