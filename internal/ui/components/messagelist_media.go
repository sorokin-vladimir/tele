package components

import (
	"fmt"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/imagecache"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// renderUploadBar draws a fixed-width progress bar like "▰▰▰▱▱ 60%".
func renderUploadBar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	cells := width - 5 // leave room for " NNN%"
	if cells < 1 {
		cells = 1
	}
	filled := int(frac*float64(cells) + 0.5)
	bar := strings.Repeat("▰", filled) + strings.Repeat("▱", cells-filled)
	return fmt.Sprintf("%s %3.0f%%", bar, frac*100)
}

// localMediaLabel is the first line of an optimistic media bubble: a kind glyph
// plus the local file name. Photos use 🖼; documents (and any other kind) use 📎.
func localMediaLabel(lm *store.LocalMedia) string {
	if lm.Kind == store.MediaPhoto {
		name := lm.FileName
		if name == "" {
			name = "photo"
		}
		return "🖼 " + name
	}
	if lm.Kind == store.MediaVideo {
		name := lm.FileName
		if name == "" {
			name = "video"
		}
		return "🎥 " + name
	}
	name := lm.FileName
	if name == "" {
		name = "file"
	}
	return "📎 " + name
}

// uploadStatusLine returns the status line under an optimistic media bubble:
// a progress bar while uploading, or an error indicator if failed.
func uploadStatusLine(lm *store.LocalMedia, width int) string {
	if lm == nil {
		return ""
	}
	if lm.UploadState == store.UploadFailed {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("✗ upload failed")
	}
	return renderUploadBar(lm.UploadProgress, width)
}

// PhotoContentCols exposes the width (in cells) photos are rendered at.
func (ml *MessageList) PhotoContentCols() int {
	return ml.photoContentCols()
}

// photoBox returns the capped (cols, rows) cell box for an image in the chat
// pane: width from photoContentCols, height bounded by the viewport and the
// fixed 480px ceiling. All photo sizing (height reservation, render, Kitty
// transmit, bubble width) goes through this so footprints stay in lock-step.
func (ml *MessageList) photoBox(imgW, imgH int) (cols, rows int) {
	cw, ch := media.CellPx()
	return media.PhotoBox(imgW, imgH, ml.photoContentCols(), ml.viewHeight, ml.maxMediaPx, cw, ch, media.CellAspect())
}

// mediaBox returns the capped (cols, rows) cell box for a message's inline
// image. Borderless media stays picture-sized: static stickers use the compact
// cap, round video notes a slightly larger cap; everything else uses the photo
// cap. All sizing sites use this so footprints stay in lock-step.
func (ml *MessageList) mediaBox(msg store.Message, imgW, imgH int) (cols, rows int) {
	cw, ch := media.CellPx()
	maxCols := ml.photoContentCols()
	switch {
	case store.IsStaticSticker(msg.Media, msg.Document):
		maxCols = ml.compactMediaCols()
	case msg.Media != nil && msg.Media.Kind == store.MediaVideoNote:
		maxCols = ml.videoNoteCols()
	}
	return media.PhotoBox(imgW, imgH, maxCols, ml.viewHeight, ml.maxMediaPx, cw, ch, media.CellAspect())
}

// MediaBoxForID returns the capped (cols, rows) box for the inline image cached
// under id, applying the same message-aware cap (sticker vs photo) used when the
// image is rendered. Transmit sizing must go through this so the Kitty placement
// is marked ready at exactly the rendered width; otherwise Render never matches.
func (ml *MessageList) MediaBoxForID(id int64, imgW, imgH int) (cols, rows int) {
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		if pid, ok := ml.PreviewImageID(ml.items[i].msg); ok && pid == id {
			return ml.mediaBox(ml.items[i].msg, imgW, imgH)
		}
	}
	return ml.photoBox(imgW, imgH)
}

// SetMaxMediaPx sets the long-side pixel cap for inline images
// (photos.max_long_side_px). Zero leaves the media-package default in effect.
func (ml *MessageList) SetMaxMediaPx(px int) {
	if px != ml.maxMediaPx && ml.renderer != nil {
		ml.renderer.Reset()
	}
	if px != ml.maxMediaPx {
		ml.invalidateHeights() // image footprint cap changed → heights change
	}
	ml.maxMediaPx = px
}

// PhotoBox exposes the capped photo cell box to callers (Kitty transmit,
// retransmit sizing) so they match the rendered grid.
func (ml *MessageList) PhotoBox(imgW, imgH int) (int, int) {
	return ml.photoBox(imgW, imgH)
}

// SetImage caches a downloaded photo for rendering.
// If the viewport was at the natural bottom before the image changed message heights,
// it is re-anchored to the new natural bottom so newest messages stay visible.
func (ml *MessageList) SetImage(photoID int64, img image.Image) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
	if ml.imageCache != nil {
		ml.imageCache.Add(photoID, img)
	}
	ml.invalidateHeights() // image now available → its reserved footprint changes
	if wasAtBottom {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	}
}

// SetKnownImages injects the shared image cache. It stores the pointer (no
// copy): root writes and render reads then hit one bounded store, so eviction
// frees pixels. Re-anchors to the natural bottom if the viewport was there.
func (ml *MessageList) SetKnownImages(cache *imagecache.Cache) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
	ml.imageCache = cache
	ml.invalidateHeights() // newly known images change reserved footprints
	if wasAtBottom {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	}
}

// cachedImage returns the cached image for id, marking it most-recently-used so
// visible images stay hot. Returns (nil, false) before the cache is injected.
func (ml *MessageList) cachedImage(id int64) (image.Image, bool) {
	if ml.imageCache == nil {
		return nil, false
	}
	return ml.imageCache.Get(id)
}

// placeholderFor returns the text label shown for a media message until (and
// unless) richer content such as photo block-art is available.
func placeholderFor(m *store.MediaRef) string {
	switch m.Kind {
	case store.MediaPhoto:
		return "📷 photo"
	case store.MediaVideo:
		return durationLabel("🎥 video", m.Duration)
	case store.MediaVideoNote:
		return durationLabel("⭕ video note", m.Duration)
	case store.MediaVoice:
		return voiceLabel(m)
	case store.MediaAudio:
		return audioLabel(m)
	case store.MediaSticker:
		if m.Emoji != "" {
			return m.Emoji + " sticker"
		}
		return "sticker"
	case store.MediaGIF:
		return "🎞 GIF"
	case store.MediaFile:
		return fileLabel(m)
	case store.MediaLocation:
		return "📍 location"
	default:
		return "📦 media"
	}
}

// fileLabel renders a generic document's placeholder: a paperclip plus the file
// name and a human-readable size when both are known, falling back to "📎 file".
func fileLabel(m *store.MediaRef) string {
	if m.FileName == "" {
		return "📎 file"
	}
	if m.Size > 0 {
		return "📎 " + m.FileName + " · " + humanSize(m.Size)
	}
	return "📎 " + m.FileName
}

// durationLabel appends a mm:ss suffix to a base label when the duration is known.
func durationLabel(base string, dur int) string {
	if dur > 0 {
		return base + " " + formatDuration(dur)
	}
	return base
}

// PreviewImageID returns the image-cache key for a message's inline image and
// whether one applies: photos, videos with an embedded thumbnail, and static
// WEBP stickers (Kitty mode only).
func (ml *MessageList) PreviewImageID(msg store.Message) (int64, bool) {
	if msg.Media == nil {
		return 0, false
	}
	switch {
	case msg.Media.Kind == store.MediaPhoto && msg.Photo != nil:
		return msg.Photo.ID, true
	case msg.Media.Kind.IsVideo() && msg.Document != nil && msg.Document.ThumbSize != "":
		return msg.Document.ID, true
	case msg.Media.Kind == store.MediaGIF && msg.Document != nil && msg.Document.ThumbSize != "":
		// Telegram GIFs are silent MP4s; show the document thumbnail inline like a
		// video (Phase 2b animates the selected one).
		return msg.Document.ID, true
	case ml.imageMode == media.ModeKitty && store.IsStaticSticker(msg.Media, msg.Document):
		return msg.Document.ID, true
	}
	return 0, false
}

// PreviewImageIDForTest exposes PreviewImageID for tests in other packages.
func (ml *MessageList) PreviewImageIDForTest(msg store.Message) (int64, bool) {
	return ml.PreviewImageID(msg)
}

// videoOverlayLabel returns the affordance shown under a thumbnail: the play +
// duration for video, a "GIF" badge for animated GIFs (so they read differently
// from a still photo), or "" for other media.
func videoOverlayLabel(m *store.MediaRef) string {
	if m == nil {
		return ""
	}
	if m.Kind.IsVideo() {
		return "▶ " + formatDuration(m.Duration)
	}
	if m.Kind == store.MediaGIF {
		return "GIF"
	}
	return ""
}

// labelLine renders one bordered, right-padded content line for a label.
// Width is measured with lipgloss.Width so wide emoji pad correctly.
func labelLine(label string, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
	padding := ""
	if pw := lipgloss.Width(label); actualW > pw {
		padding = strings.Repeat(" ", actualW-pw)
	}
	return bs.Render(b.Left) + " " + label + padding + " " + bs.Render(b.Right)
}

// placeholderLine renders one bordered label line for a media placeholder.
func placeholderLine(m *store.MediaRef, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
	return labelLine(placeholderFor(m), actualW, b, bs)
}

func (ml *MessageList) photoContentCols() int {
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	maxContentW := maxBubbleW - 4
	if maxContentW > 60 {
		maxContentW = 60
	}
	if maxContentW < 4 {
		maxContentW = 4
	}
	return maxContentW
}

// compactMediaCols is the inline-image width cap for borderless media (static
// stickers and round video notes): a third of the photo cap so they read as
// compact pictures and do not dominate the pane. Bounded like photoContentCols.
func (ml *MessageList) compactMediaCols() int {
	cols := ml.photoContentCols() / 3
	if cols > 20 {
		cols = 20
	}
	if cols < 4 {
		cols = 4
	}
	return cols
}

// CompactMediaColsForTest exposes compactMediaCols for tests.
func (ml *MessageList) CompactMediaColsForTest() int { return ml.compactMediaCols() }

// videoNoteCols is the inline-image width cap for round video notes: larger than
// the sticker cap so a face stays legible, but still well under the photo cap.
// Bounded like photoContentCols.
func (ml *MessageList) videoNoteCols() int {
	cols := ml.photoContentCols()
	if cols > 30 {
		cols = 30
	}
	if cols < 4 {
		cols = 4
	}
	return cols
}

// VideoNoteColsForTest exposes videoNoteCols for tests.
func (ml *MessageList) VideoNoteColsForTest() int { return ml.videoNoteCols() }
