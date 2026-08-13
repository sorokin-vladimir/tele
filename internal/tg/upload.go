package tg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
)

// UploadParams configures a single file upload.
type UploadParams struct {
	Path string
	// Name is the name the file goes up under. Telegram reads the file's type
	// off it and refuses content whose name does not carry a matching
	// extension, so it is a parameter of the call rather than something to
	// infer from the path down here (#230). Empty means the name the file has
	// on disk, which is what gotd would have derived on its own.
	Name       string
	OnProgress func(sent, total int64) // optional; nil-safe
}

// progressAdapter implements uploader.Progress. gotd may call Chunk concurrently
// when uploading with multiple threads, so it serializes calls under a mutex and
// forwards a clean (sent, total) stream to the user callback.
type progressAdapter struct {
	mu sync.Mutex
	cb func(sent, total int64)
}

func newProgressAdapter(cb func(sent, total int64)) *progressAdapter {
	return &progressAdapter{cb: cb}
}

func (p *progressAdapter) Chunk(_ context.Context, state uploader.ProgressState) error {
	if p.cb == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cb(state.Uploaded, state.Total)
	return nil
}

// UploadFile uploads a local file in chunks and returns the resulting InputFile
// (small) or InputFileBig (>~10MB). gotd's uploader selects upload.saveFilePart vs
// upload.saveBigFilePart by size internally. Cancel via ctx.
//
// The file is opened here rather than through uploader.FromPath because that
// helper names the upload from the path and there is no way to say otherwise.
// This is FromPath's own chain - open, stat, upload - with the name supplied.
// The size still comes from the stat, so the small/big decision is unchanged.
func (c *GotdClient) UploadFile(ctx context.Context, p UploadParams) (tg.InputFileClass, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	src, err := os.Open(filepath.Clean(p.Path))
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", p.Path, err)
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", p.Path, err)
	}

	u := uploader.NewUploader(api).WithProgress(newProgressAdapter(p.OnProgress))
	f, err := u.Upload(ctx, uploader.NewUpload(uploadFileName(p, info), src, info.Size()))
	if err != nil {
		return nil, fmt.Errorf("upload %s: %w", p.Path, err)
	}
	return f, nil
}

// uploadFileName resolves the name the upload is announced under: the caller's
// if it asked for one, otherwise the file's own, which is what gotd derived
// before the parameter existed.
func uploadFileName(p UploadParams, info os.FileInfo) string {
	if p.Name != "" {
		return p.Name
	}
	return info.Name()
}
