package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFFprobeOutput_AllFields(t *testing.T) {
	out := "width=1920\nheight=1080\nduration=42.7\n"
	meta := parseFFprobeOutput(out)
	assert.Equal(t, 1920, meta.Width)
	assert.Equal(t, 1080, meta.Height)
	assert.Equal(t, 42, meta.Duration, "duration is truncated to whole seconds")
}

func TestParseFFprobeOutput_Empty(t *testing.T) {
	assert.Equal(t, VideoMeta{}, parseFFprobeOutput(""))
}

func TestParseFFprobeOutput_PartialAndJunk(t *testing.T) {
	out := "width=640\ngarbage\nduration=\nheight=480\n"
	meta := parseFFprobeOutput(out)
	assert.Equal(t, 640, meta.Width)
	assert.Equal(t, 480, meta.Height)
	assert.Equal(t, 0, meta.Duration, "unparsable duration stays zero")
}

// TestProbeAndThumbnail_WithFFmpeg is an integration check that runs only when
// ffmpeg/ffprobe are installed. It generates a tiny synthetic clip, probes it,
// and extracts a thumbnail.
func TestProbeAndThumbnail_WithFFmpeg(t *testing.T) {
	if !HasFFmpeg() || !HasFFprobe() {
		t.Skip("ffmpeg/ffprobe not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.mp4")
	gen := exec.Command("ffmpeg", "-y", "-f", "lavfi",
		"-i", "testsrc=duration=1:size=320x240:rate=15", src)
	require.NoError(t, gen.Run(), "failed to synthesize sample video")

	meta, err := ProbeVideo(context.Background(), src)
	require.NoError(t, err)
	assert.Equal(t, 320, meta.Width)
	assert.Equal(t, 240, meta.Height)
	assert.GreaterOrEqual(t, meta.Duration, 1)

	out := filepath.Join(dir, "thumb.jpg")
	require.NoError(t, ExtractThumbnail(context.Background(), src, out))
	fi, err := os.Stat(out)
	require.NoError(t, err)
	assert.Positive(t, fi.Size(), "thumbnail must be non-empty")
}
