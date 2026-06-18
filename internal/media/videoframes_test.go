package media

import (
	"bytes"
	"context"
	"image"
	"io"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadFrame_OneFrame(t *testing.T) {
	const w, h = 2, 2
	// 2x2 RGBA: 16 bytes, distinct first/last pixel.
	buf := make([]byte, w*h*4)
	buf[0], buf[1], buf[2], buf[3] = 10, 20, 30, 255     // top-left
	buf[12], buf[13], buf[14], buf[15] = 40, 50, 60, 255 // bottom-right
	img, err := readFrame(bytes.NewReader(buf), w, h)
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, w, h), img.Bounds())
	tl := img.NRGBAAt(0, 0)
	assert.Equal(t, [4]uint8{10, 20, 30, 255}, [4]uint8{tl.R, tl.G, tl.B, tl.A})
	assert.Equal(t, uint8(40), img.NRGBAAt(1, 1).R)
}

func TestReadFrame_CleanEOF(t *testing.T) {
	_, err := readFrame(bytes.NewReader(nil), 2, 2)
	assert.ErrorIs(t, err, io.EOF)
}

func TestReadFrame_PartialFrameIsUnexpectedEOF(t *testing.T) {
	_, err := readFrame(bytes.NewReader([]byte{1, 2, 3}), 2, 2)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestOpenFrameSource_NoFFmpeg_IsError(t *testing.T) {
	if HasFFmpeg() {
		t.Skip("ffmpeg present; this checks the missing-binary path")
	}
	_, err := OpenFrameSource(context.Background(), "/nonexistent.mp4", 16, 16, 10)
	require.Error(t, err)
}

func TestDecodeAllFrames_WithFFmpeg(t *testing.T) {
	if !HasFFmpeg() {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.mp4")
	gen := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc=duration=1:size=160x120:rate=10", src)
	require.NoError(t, gen.Run(), "failed to synthesize sample video")

	const w, h = 160, 120
	frames, err := DecodeAllFrames(context.Background(), src, w, h, 10, 100)
	require.NoError(t, err)
	// 1s at 10fps ~= 10 frames; allow a small tolerance for encoder rounding.
	assert.GreaterOrEqual(t, len(frames), 8)
	assert.LessOrEqual(t, len(frames), 12)
	for _, f := range frames {
		assert.Equal(t, image.Rect(0, 0, w, h), f.Bounds())
	}
}

func TestDecodeAllFrames_RespectsMaxFrames(t *testing.T) {
	if !HasFFmpeg() {
		t.Skip("ffmpeg not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.mp4")
	gen := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc=duration=2:size=80x60:rate=10", src)
	require.NoError(t, gen.Run())

	frames, err := DecodeAllFrames(context.Background(), src, 80, 60, 10, 5)
	require.NoError(t, err)
	assert.Len(t, frames, 5, "must stop at maxFrames")
}
