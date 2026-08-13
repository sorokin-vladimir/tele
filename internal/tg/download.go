package tg

// Downloads are streams, not images: the owner writes them to disk and clients
// decode from the file, so nothing here decodes and no image decoder is
// registered in this package any more (#196).
import (
	"context"
	"fmt"
	"io"

	"github.com/gotd/td/telegram/downloader"
	gotdtg "github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// DownloadPhotoToFile streams the raw photo bytes (the size named by
// ref.ThumbSize) directly into dst without decoding, so a photo can be saved to
// disk at full quality. Mirrors DownloadDocumentToFile.
func (c *GotdClient) DownloadPhotoToFile(ctx context.Context, ref domain.PhotoRef, dst io.Writer) error {
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

// DownloadUserAvatarToFile streams a person's avatar into dst at the small
// size, which is the one an avatar is rendered at: a handful of cells wide.
//
// Unlike every other download here it carries no file reference. An avatar is
// addressed by peer and photo id, both of which stay valid, so none of the
// expiry-and-refresh machinery around message media applies (#223).
func (c *GotdClient) DownloadUserAvatarToFile(ctx context.Context, addr UserAddress, avatarID int64, dst io.Writer) error {
	if avatarID == 0 {
		return &telerr.Error{Kind: telerr.NotFound, Op: "download avatar", Detail: "user has no avatar"}
	}
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	peer, err := addr.inputPeer()
	if err != nil {
		return err
	}

	loc := &gotdtg.InputPeerPhotoFileLocation{
		Peer:    peer,
		PhotoID: avatarID,
		// Big is deliberately left false: the big size is 640px for a picture
		// that is drawn at roughly 80, and the small one is the whole point of
		// keeping avatars apart from chat media.
	}

	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, dst); err != nil {
		return fmt.Errorf("download avatar %d of user %d: %w", avatarID, addr.UserID, err)
	}
	return nil
}

// DownloadDocumentToFile streams the full document into dst without buffering
// the whole file in memory, so it stays bounded regardless of file size.
func (c *GotdClient) DownloadDocumentToFile(ctx context.Context, ref domain.DocumentRef, dst io.Writer) error {
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

// DownloadDocumentThumbToFile streams the document's thumbnail (the size named
// by ref.ThumbSize) into dst. It is separate from DownloadDocumentToFile
// because that one deliberately ignores ThumbSize and always streams the full
// file; a poster frame needs the thumbnail location instead.
func (c *GotdClient) DownloadDocumentThumbToFile(ctx context.Context, ref domain.DocumentRef, dst io.Writer) error {
	if ref.ThumbSize == "" {
		return &telerr.Error{Kind: telerr.NotFound, Op: "download document thumb", Detail: "no thumbnail"}
	}
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}

	loc := &gotdtg.InputDocumentFileLocation{
		ID:            ref.ID,
		AccessHash:    ref.AccessHash,
		FileReference: ref.FileReference,
		ThumbSize:     ref.ThumbSize,
	}

	d := downloader.NewDownloader()
	if _, err := d.Download(api, loc).Stream(ctx, dst); err != nil {
		return fmt.Errorf("download document thumb %d: %w", ref.ID, err)
	}
	return nil
}
