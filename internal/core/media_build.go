package core

import (
	"context"
	"os"
	"path/filepath"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/media"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// uploadPart uploads one local file and wraps it in the InputMedia Telegram
// expects for its kind.
//
// This is protocol work, and it lives here rather than in a client for two
// reasons: gotd values cannot cross a process boundary, and a second client — a
// CLI, or whatever attaches to the owner next — inherits the whole path instead
// of reimplementing it (#195).
//
// onProgress is nil-safe and reports this part's own bytes; the caller
// aggregates across the entry.
func (o *Owner) uploadPart(ctx context.Context, part domain.OutboxMediaPart, onProgress func(sent, total int64)) (tg.InputMediaClass, error) {
	mime, err := media.DetectMIME(part.Path)
	if err != nil {
		return nil, err
	}
	// SendAs is the user's intent and wins; detection only fills the gap when
	// there was no choice to make.
	kind := part.SendAs
	if kind == 0 {
		kind = media.DefaultMediaType(mime)
	}

	f, err := o.client.UploadFile(ctx, internaltg.UploadParams{
		Path:       part.Path,
		Name:       uploadName(part.Name, mime),
		OnProgress: onProgress,
	})
	if err != nil {
		return nil, err
	}

	switch kind {
	case domain.MediaPhoto:
		return internaltg.BuildInputMediaUploadedPhoto(f), nil
	case domain.MediaVideo:
		return o.buildVideo(ctx, part, mime, f), nil
	default:
		return internaltg.BuildInputMediaUploadedDocument(f, part.Name, mime), nil
	}
}

// uploadName is the name the file is announced to Telegram under, which is not
// the name it arrives under. Telegram validates the type against this name and
// refuses a photo whose name carries no extension - a JPEG saved by a browser,
// say - even though the bytes were never in question. Where the type was
// detected and the name says nothing, the extension for that type is appended
// (#230).
//
// The divergence from name is deliberate. name is what the person picked and
// what the recipient sees, and rewriting it would rename their file on the
// other end without being asked; this one is protocol detail whose only job is
// letting Telegram identify what it was sent.
//
// A name that already carries an extension is believed, wrong or not: catching
// a .png that is really a JPEG would mean sniffing every file instead of
// trusting the name, for a case not observed and one Telegram re-encodes photos
// past anyway. A type that was not recognized leaves nothing to append, and the
// send goes as it would have.
func uploadName(name, mime string) string {
	if filepath.Ext(name) != "" {
		return name
	}
	ext, _ := media.ExtensionFor(mime)
	return name + ext
}

// buildVideo probes the source for duration and dimensions and extracts a
// thumbnail frame. ffmpeg and ffprobe are optional: a missing binary or a failed
// probe degrades to sending the video with no metadata and no client thumbnail,
// and Telegram fills the gaps server-side.
func (o *Owner) buildVideo(ctx context.Context, part domain.OutboxMediaPart, mime string, f tg.InputFileClass) tg.InputMediaClass {
	var meta media.VideoMeta
	if probed, err := media.ProbeVideo(ctx, part.Path); err == nil {
		meta = probed
	}
	thumb := o.uploadVideoThumb(ctx, part.Path)
	return internaltg.BuildInputMediaUploadedVideo(
		f, part.Name, mime, meta.Duration, meta.Width, meta.Height, thumb,
	)
}

// uploadVideoThumb extracts a frame from the source video and uploads it,
// returning nil on any failure — no ffmpeg, a failed extract, a refused upload —
// so the caller sends the video without a client thumbnail rather than not at all.
func (o *Owner) uploadVideoThumb(ctx context.Context, path string) tg.InputFileClass {
	tmp, err := os.CreateTemp("", "tele-thumb-*.jpg")
	if err != nil {
		return nil
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := media.ExtractThumbnail(ctx, path, tmpPath); err != nil {
		return nil
	}
	thumb, err := o.client.UploadFile(ctx, internaltg.UploadParams{Path: tmpPath})
	if err != nil {
		return nil
	}
	return thumb
}
