package media

import (
	"context"
	"fmt"
	"image"
	"io"
	"os/exec"
)

// readFrame reads exactly w*h*4 bytes from r and returns them as an *image.NRGBA
// (8-bit non-premultiplied RGBA, the layout ffmpeg emits for -pix_fmt rgba). It
// returns io.EOF when the stream ends cleanly on a frame boundary (no bytes read)
// and io.ErrUnexpectedEOF on a partial frame.
func readFrame(r io.Reader, w, h int) (*image.NRGBA, error) {
	buf := make([]byte, w*h*4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return &image.NRGBA{Pix: buf, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}, nil
}

// frameBufferSize bounds the decode-ahead channel. A few frames smooth out tick
// jitter; keeping it small means a paused consumer quickly fills the buffer and
// ffmpeg blocks on the full pipe (natural backpressure), rather than decoding the
// whole file into memory.
const frameBufferSize = 4

// FrameSource streams decoded RGBA frames from a local video file via ffmpeg.
// Frames are exactly w×h. Not safe for concurrent use by multiple goroutines.
type FrameSource struct {
	ctx    context.Context // cancelled by Close; unblocks an in-flight frame send
	cancel context.CancelFunc
	cmd    *exec.Cmd
	frames chan *image.NRGBA
	err    error // set by the reader goroutine before frames closes; read after drain
	done   chan struct{}
}

// OpenFrameSource spawns ffmpeg to decode path, scaled to exactly w×h px at fps,
// audio dropped, emitting raw RGBA on stdout. A reader goroutine slices the
// stream into frames. It returns an error if ffmpeg cannot be started (e.g. the
// binary is missing).
func OpenFrameSource(ctx context.Context, path string, w, h, fps int) (*FrameSource, error) {
	cctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(cctx, "ffmpeg",
		"-nostdin",
		"-loglevel", "error",
		"-i", path,
		"-an",
		"-vf", fmt.Sprintf("scale=%d:%d,fps=%d", w, h, fps),
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"pipe:1",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	fs := &FrameSource{
		ctx:    cctx,
		cancel: cancel,
		cmd:    cmd,
		frames: make(chan *image.NRGBA, frameBufferSize),
		done:   make(chan struct{}),
	}
	go fs.read(stdout, w, h)
	return fs, nil
}

// read decodes frames until EOF/error, then closes the frames channel. A clean
// EOF leaves err nil; any other read error is recorded. cmd.Wait runs after the
// pipe drains so the process is reaped.
func (fs *FrameSource) read(stdout io.Reader, w, h int) {
	defer close(fs.done)
	defer close(fs.frames)
	// Reap the process once decoding stops. A non-zero exit is expected when the
	// consumer cancels mid-stream (Close), so the wait error is not surfaced. This
	// runs before close(frames)/close(done) so Close's <-done sees a reaped process.
	defer func() { _ = fs.cmd.Wait() }()
	for {
		frame, err := readFrame(stdout, w, h)
		if err != nil {
			if err != io.EOF {
				fs.err = err
			}
			break
		}
		// A consumer that stops pulling (e.g. DecodeAllFrames hits maxFrames, or
		// Close is called) leaves the buffered channel full; select on ctx so the
		// send is cancellable and the goroutine can't deadlock on it.
		select {
		case fs.frames <- frame:
		case <-fs.ctx.Done():
			return
		}
	}
}

// Next returns the next decoded frame, or ok=false once the stream ends (EOF or
// error — check Err). The returned image is owned by the caller.
func (fs *FrameSource) Next() (image.Image, bool) {
	frame, ok := <-fs.frames
	if !ok {
		return nil, false
	}
	return frame, true
}

// Err returns the first decode error, if any. Valid only after Next has returned
// ok=false (the channel is closed). A clean end-of-stream returns nil.
func (fs *FrameSource) Err() error {
	<-fs.done
	return fs.err
}

// Close stops ffmpeg and releases resources. Safe to call multiple times.
func (fs *FrameSource) Close() error {
	fs.cancel()
	<-fs.done
	return nil
}

// DecodeAllFrames decodes up to maxFrames frames of path (scaled to w×h at fps)
// into memory. Intended for short GIF loops that play from memory without a live
// ffmpeg process. It returns the decode error, if any.
func DecodeAllFrames(ctx context.Context, path string, w, h, fps, maxFrames int) ([]image.Image, error) {
	fs, err := OpenFrameSource(ctx, path, w, h, fps)
	if err != nil {
		return nil, err
	}
	var frames []image.Image
	for len(frames) < maxFrames {
		f, ok := fs.Next()
		if !ok {
			break
		}
		frames = append(frames, f)
	}
	// Close before Err: when we stop early (maxFrames), the reader goroutine is
	// blocked on a full channel; Close cancels it so done closes and Err returns.
	// Calling Err first (e.g. via defer Close) would deadlock — Err waits on done,
	// done waits on Close.
	_ = fs.Close()
	return frames, fs.Err()
}
