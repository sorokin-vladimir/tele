package tg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/gotd/td/telegram/downloader"
	gotdtg "github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// IsFileReferenceExpired reports whether err is Telegram's FILE_REFERENCE_EXPIRED.
// Kept here so the ui layer can detect it without importing gotd directly.
func IsFileReferenceExpired(err error) bool {
	return tgerr.Is(err, "FILE_REFERENCE_EXPIRED")
}

func (c *GotdClient) DownloadPhoto(ctx context.Context, ref store.PhotoRef) (image.Image, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	loc := &gotdtg.InputPhotoFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
		ThumbSize:     ref.ThumbSize,
	}

	var buf bytes.Buffer
	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, &buf); err != nil {
		return nil, fmt.Errorf("download photo %d: %w", ref.ID, err)
	}

	img, _, err := image.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode photo %d: %w", ref.ID, err)
	}
	return img, nil
}

// DownloadDocument fetches the full document file as raw bytes.
func (c *GotdClient) DownloadDocument(ctx context.Context, ref store.DocumentRef) ([]byte, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	loc := &gotdtg.InputDocumentFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
	}

	var buf bytes.Buffer
	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, &buf); err != nil {
		return nil, fmt.Errorf("download document %d: %w", ref.ID, err)
	}
	return buf.Bytes(), nil
}

// DownloadDocumentThumb fetches and decodes the document's thumbnail named by
// ref.ThumbSize for an inline preview.
func (c *GotdClient) DownloadDocumentThumb(ctx context.Context, ref store.DocumentRef) (image.Image, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}
	if ref.ThumbSize == "" {
		return nil, fmt.Errorf("document %d has no thumbnail", ref.ID)
	}

	loc := &gotdtg.InputDocumentFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
		ThumbSize:     ref.ThumbSize,
	}

	var buf bytes.Buffer
	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, &buf); err != nil {
		return nil, fmt.Errorf("download document thumb %d: %w", ref.ID, err)
	}

	img, _, err := image.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode document thumb %d: %w", ref.ID, err)
	}
	return img, nil
}
