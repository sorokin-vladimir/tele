package tg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/webp" // register WEBP decoder for image.Decode (static stickers)

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
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
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

// DownloadPhotoToFile streams the raw photo bytes (the size named by
// ref.ThumbSize) directly into dst without decoding, so a photo can be saved to
// disk at full quality. Mirrors DownloadDocumentToFile.
func (c *GotdClient) DownloadPhotoToFile(ctx context.Context, ref store.PhotoRef, dst io.Writer) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}

	loc := &gotdtg.InputPhotoFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
		ThumbSize:     ref.ThumbSize,
	}

	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, dst); err != nil {
		return fmt.Errorf("download photo %d: %w", ref.ID, err)
	}
	return nil
}

// DownloadDocument fetches the full document file as raw bytes.
func (c *GotdClient) DownloadDocument(ctx context.Context, ref store.DocumentRef) ([]byte, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
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

// DownloadDocumentToFile streams the full document into dst without buffering
// the whole file in memory, so it stays bounded regardless of file size.
func (c *GotdClient) DownloadDocumentToFile(ctx context.Context, ref store.DocumentRef, dst io.Writer) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}

	loc := &gotdtg.InputDocumentFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
	}

	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, dst); err != nil {
		return fmt.Errorf("download document %d: %w", ref.ID, err)
	}
	return nil
}

// DownloadDocumentThumb fetches and decodes the document's thumbnail named by
// ref.ThumbSize for an inline preview.
func (c *GotdClient) DownloadDocumentThumb(ctx context.Context, ref store.DocumentRef) (image.Image, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
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

// DownloadDocumentImage fetches the full document file and decodes it as an
// image. Used for static WEBP stickers: unlike DownloadDocumentThumb it streams
// the main file (no ThumbSize), so transparency from the full sticker is kept.
func (c *GotdClient) DownloadDocumentImage(ctx context.Context, ref store.DocumentRef) (image.Image, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	loc := &gotdtg.InputDocumentFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
	}

	var buf bytes.Buffer
	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, &buf); err != nil {
		return nil, fmt.Errorf("download document image %d: %w", ref.ID, err)
	}

	img, _, err := image.Decode(&buf)
	if err != nil {
		return nil, fmt.Errorf("decode document image %d: %w", ref.ID, err)
	}
	return img, nil
}
