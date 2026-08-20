package components

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/imagecache"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
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

// localMediaLabel is the first line of a queued media bubble: a kind glyph plus
// the file name, or the count for an album group, which is one bubble because it
// will be one album. Photos use 🖼, videos 🎥, everything else 📎.
//
// Which file of the group is uploading belongs here rather than on the progress
// line: the bubble is measured by this label, so the counter widens the bubble
// instead of overflowing a line that was already exactly as wide as it is
// allowed to be.
func localMediaLabel(lm *domain.LocalMedia) string {
	glyph := mediaKindGlyph(lm.Kind)
	if lm.Parts > 1 {
		part := lm.Part
		if part < 1 {
			// Nothing is uploading yet. The first file is still the one this
			// send is about to be on, and showing it keeps the label — and with
			// it the bubble — from widening the moment the first frame lands.
			part = 1
		}
		// The number is padded to the total's digits for the same reason: 9/10
		// and 10/10 must occupy the same width.
		digits := len(strconv.Itoa(lm.Parts))
		return fmt.Sprintf("%s %d %s %*d/%d",
			glyph, lm.Parts, mediaKindNoun(lm.Kind, lm.Parts), digits, part, lm.Parts)
	}
	name := lm.FileName
	if name == "" {
		name = mediaKindNoun(lm.Kind, 1)
	}
	return glyph + " " + name
}

func mediaKindGlyph(k domain.MediaKind) string {
	switch k {
	case domain.MediaPhoto:
		return "🖼"
	case domain.MediaVideo:
		return "🎥"
	default:
		return "📎"
	}
}

// mediaKindNoun names what is being sent: "3 photos", "2 videos", "5 files".
func mediaKindNoun(k domain.MediaKind, n int) string {
	var one, many string
	switch k {
	case domain.MediaPhoto:
		one, many = "photo", "photos"
	case domain.MediaVideo:
		one, many = "video", "videos"
	default:
		one, many = "file", "files"
	}
	if n == 1 {
		return one
	}
	return many
}

// uploadStatusLine returns the status line under a queued media bubble: the
// progress bar, and nothing else. It fills exactly the width it is given, which
// is what the bubble was sized to — labelLine pads a short line but does not
// truncate a long one, so anything extra here tears the right border.
//
// A failed send is not shown here either: that is the entry's state, and it
// already reads as ✕ in the bottom border where every other queue state lives
// (#193).
func uploadStatusLine(lm *domain.LocalMedia, width int) string {
	if lm == nil {
		return ""
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
func (ml *MessageList) mediaBox(msg domain.Message, imgW, imgH int) (cols, rows int) {
	cw, ch := media.CellPx()
	maxCols := ml.photoContentCols()
	switch {
	case domain.IsStaticSticker(msg.Media, msg.Document):
		maxCols = ml.compactMediaCols()
	case msg.Media != nil && msg.Media.Kind == domain.MediaVideoNote:
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
		// Album parts render into the downscaled per-part box, so their Kitty
		// placement must be sized the same way or the image is drawn at the wrong
		// scale (crop/overflow). Match any part, not just the anchor.
		if len(ml.items[i].parts) > 1 {
			// A gridded album transmits each tile at its cover box; the stack path
			// (fallback) uses the downscaled per-part box. Both are metadata-derived
			// and stable as siblings load in.
			if mid := ml.msgIDForPreviewID(ml.items[i].parts, id); mid != 0 {
				if g, ok := ml.albumTileGeom(ml.items[i].parts, mid, imgW, imgH); ok {
					return g.transmitBox()
				}
			}
			budget := ml.albumImageRows(ml.items[i].parts)
			for _, p := range ml.items[i].parts {
				if pid, ok := ml.PreviewImageID(p); ok && pid == id {
					return ml.albumPartBox(budget, imgW, imgH)
				}
			}
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

// MaxMediaPx is the cap currently in force. It exists so that a reload can be
// checked to have reached the list, rather than only to have reached the config.
func (ml *MessageList) MaxMediaPx() int { return ml.maxMediaPx }

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
func placeholderFor(m *domain.MediaRef) string {
	switch m.Kind {
	case domain.MediaPhoto:
		return "📷 photo"
	case domain.MediaVideo:
		return durationLabel("🎥 video", m.Duration)
	case domain.MediaVideoNote:
		return durationLabel("⭕ video note", m.Duration)
	case domain.MediaVoice:
		return voiceLabel(m)
	case domain.MediaAudio:
		return audioLabel(m)
	case domain.MediaSticker:
		if m.Emoji != "" {
			return m.Emoji + " sticker"
		}
		return "sticker"
	case domain.MediaGIF:
		return "🎞 GIF"
	case domain.MediaFile:
		return fileLabel(m)
	case domain.MediaLocation:
		return "📍 location"
	default:
		return "📦 media"
	}
}

// fileLabel renders a generic document's placeholder: a paperclip plus the file
// name and a human-readable size when both are known, falling back to "📎 file".
func fileLabel(m *domain.MediaRef) string {
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
func (ml *MessageList) PreviewImageID(msg domain.Message) (int64, bool) {
	if msg.Media == nil {
		return 0, false
	}
	switch {
	case msg.Media.Kind == domain.MediaPhoto && msg.Photo != nil:
		return msg.Photo.ID, true
	case msg.Media.Kind.IsVideo() && msg.Document != nil && msg.Document.ThumbSize != "":
		return msg.Document.ID, true
	case msg.Media.Kind == domain.MediaGIF && msg.Document != nil && msg.Document.ThumbSize != "":
		// Telegram GIFs are silent MP4s; show the document thumbnail inline like a
		// video (Phase 2b animates the selected one).
		return msg.Document.ID, true
	case ml.imageMode == media.ModeKitty && domain.IsStaticSticker(msg.Media, msg.Document):
		return msg.Document.ID, true
	}
	return 0, false
}

// PreviewImageIDForTest exposes PreviewImageID for tests in other packages.
func (ml *MessageList) PreviewImageIDForTest(msg domain.Message) (int64, bool) {
	return ml.PreviewImageID(msg)
}

// videoOverlayLabel returns the affordance shown under a thumbnail: the play +
// duration for video, a "GIF" badge for animated GIFs (so they read differently
// from a still photo), or "" for other media.
func videoOverlayLabel(m *domain.MediaRef) string {
	if m == nil {
		return ""
	}
	if m.Kind.IsVideo() {
		return "▶ " + formatDuration(m.Duration)
	}
	if m.Kind == domain.MediaGIF {
		return "GIF"
	}
	return ""
}

// labelLine renders one bordered, right-padded content line for a label.
// Width is measured with lipgloss.Width so wide emoji pad correctly.
//
// The label arrives as plain text and is painted here rather than by whoever
// produced it. Every producer of one — placeholderFor, localMediaLabel,
// uploadStatusLine, overlayLabelFor, voicePlayingLabel, albumBadgeLabel — is
// measured, concatenated with prefixes or compared in tests, and none of them
// carries a reset, so the line that lays them out is free to paint them (#227,
// and the clarification in ADR 0002). Measured before rendering, since the
// padding is owed on the label's width and not on its escapes.
func labelLine(label string, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
	return paintedLabelLine(theme.S().Body.Render(label), lipgloss.Width(label), actualW, b, bs)
}

// paintedLabelLine is the same line for a label that arrived already painted.
// Content holding a reset in the middle of it cannot be painted by wrapping —
// the background would be lost from that reset onward — so such a label paints
// each of its own runs and hands over the width it occupies, which can no longer
// be read off the string as cheaply. The voice waveform is the one label with
// that shape: its played run takes a colour of its own.
func paintedLabelLine(label string, labelW, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
	padding := theme.PadTo(labelW, actualW)
	return bs.Render(b.Left) + theme.Pad(1) + label + padding + theme.Pad(1) + bs.Render(b.Right)
}

// placeholderLine renders one bordered label line for a media placeholder.
func placeholderLine(m *domain.MediaRef, actualW int, b lipgloss.Border, bs lipgloss.Style) string {
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
