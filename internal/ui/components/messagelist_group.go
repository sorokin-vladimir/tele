package components

import (
	"strconv"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// defaultAlbumImgW/H are the representative image dimensions used to size album
// previews before any part's bytes are cached. A 4:3 landscape keeps the
// pre-load reservation close to the typical photo footprint.
const (
	defaultAlbumImgW = 1024
	defaultAlbumImgH = 768
)

// groupMedia is one renderable/openable media part of an album, tagged with its
// 1-based display index. The index is stable across the bubble badges, the open
// picker labels, and the modal paging order.
type groupMedia struct {
	Index int
	Msg   domain.Message
}

// hasRenderableMedia reports whether a message carries media the list draws
// inline: a photo or any document-backed media.
func hasRenderableMedia(msg domain.Message) bool {
	return msg.Photo != nil || (msg.Media != nil && msg.Document != nil)
}

// groupMediaParts returns the album's media-bearing parts in order, numbered from
// 1. Text-only parts (an album can hold at most one caption-only message in
// practice, but guard anyway) are skipped.
func groupMediaParts(parts []domain.Message) []groupMedia {
	out := make([]groupMedia, 0, len(parts))
	n := 0
	for _, p := range parts {
		if !hasRenderableMedia(p) {
			continue
		}
		n++
		out = append(out, groupMedia{Index: n, Msg: p})
	}
	return out
}

// albumContentW is the caption/text content width for an album bubble: the same
// three-quarter-viewport budget the single-message path uses.
func (ml *MessageList) albumContentW() int {
	return ml.maxContentWidth()
}

// albumImageRows returns the scaled art-row count for each image part of an
// album, chosen so the whole bubble — borders, one badge per part, any compact
// file rows, and the caption — fits the viewport height. A readability floor of 2
// rows applies. Both groupHeight and renderGroupBubble call this so they stay in
// lock-step.
func (ml *MessageList) albumImageRows(parts []domain.Message) int {
	media := groupMediaParts(parts)
	imgCount, fileCount := 0, 0
	for _, gm := range media {
		// Count by metadata, not current cache state: a part that WILL show a
		// preview (a photo, or a video/GIF with a thumbnail) must claim its budget
		// slot from the start. Otherwise a late-loading sibling (e.g. a video
		// thumbnail) would shrink the budget and resize the already-transmitted
		// photos, whose Kitty placements would then stop matching the render.
		if ml.albumPartReservesPreview(gm.Msg) {
			imgCount++
		} else {
			fileCount++
		}
	}
	if imgCount == 0 {
		return 0
	}
	// Overhead the images divide the remaining height against: borders, one blank
	// line between adjacent parts, and a badge row for each file part. Image badges
	// are folded onto the picture's first row, so they cost no extra row — which is
	// why the budget must not reserve one for them (otherwise a loaded album falls
	// short of the pane). Metadata-derived, so the budget stays stable as parts
	// load in.
	overhead := 2 + fileCount + (len(media) - 1)
	if caption := albumCaption(parts); caption != "" {
		overhead += 1 + wrappedLineCount(caption, albumCaptionEntities(parts), ml.albumContentW())
	}
	budget := ml.viewHeight - overhead
	_, normal := ml.photoBox(defaultAlbumImgW, defaultAlbumImgH)
	rows := normal
	if per := budget / imgCount; per < rows {
		rows = per
	}
	if rows < 2 {
		rows = 2
	}
	return rows
}

// albumCaption returns the album's single caption. Telegram attaches the caption
// to one part (usually the first); return the first non-empty text in order.
func albumCaption(parts []domain.Message) string {
	for _, p := range parts {
		if p.Text != "" {
			return p.Text
		}
	}
	return ""
}

// albumCaptionEntities returns the entities of the caption-bearing part so the
// caption wraps and styles identically to a normal message body.
func albumCaptionEntities(parts []domain.Message) []domain.MessageEntity {
	for _, p := range parts {
		if p.Text != "" {
			return p.Entities
		}
	}
	return nil
}

// albumPartReservesPreview reports whether an album part will eventually show an
// inline preview, based on metadata alone (independent of download state): a
// photo, or a video/GIF/sticker that carries a thumbnail. Used to size the shared
// per-part budget so it stays stable as parts load in.
func (ml *MessageList) albumPartReservesPreview(msg domain.Message) bool {
	if _, ok := ml.PreviewImageID(msg); ok {
		return true
	}
	return msg.Photo != nil
}

// albumPartHasCachedArt reports whether an album part's inline image bytes are
// cached, so its preview can be drawn now with the badge folded onto the image's
// first row. A photo still awaiting bytes, or a generic file, reports false and is
// described by a standalone badge line instead.
func (ml *MessageList) albumPartHasCachedArt(msg domain.Message) bool {
	if id, ok := ml.PreviewImageID(msg); ok {
		_, has := ml.cachedImage(id)
		return has
	}
	return false
}

// albumPartBox fits an album part's image within the album content width and the
// per-part row budget, preserving aspect. PhotoBox downscales (never crops): it
// caps height at viewHeight*2/3, so passing budgetRows*3/2 makes the row cap equal
// the budget. Both the render and the Kitty transmit sizing go through this so the
// placement matches the drawn grid.
func (ml *MessageList) albumPartBox(budgetRows, imgW, imgH int) (cols, rows int) {
	cw, ch := media.CellPx()
	return media.PhotoBox(imgW, imgH, ml.photoContentCols(), budgetRows*3/2, ml.maxMediaPx, cw, ch, media.CellAspect())
}

// albumPartRows returns the reserved art rows for one album part: the downscaled
// box height when its image bytes are cached, the full budget as a placeholder box
// for a photo still awaiting bytes, or 0 for a badge-only part (a generic file).
func (ml *MessageList) albumPartRows(budgetRows int, msg domain.Message) int {
	if id, ok := ml.PreviewImageID(msg); ok {
		if img, has := ml.cachedImage(id); has {
			b := img.Bounds()
			_, rows := ml.albumPartBox(budgetRows, b.Dx(), b.Dy())
			return rows
		}
	}
	if msg.Photo != nil {
		return budgetRows
	}
	return 0
}

// albumBadgeText describes an album part next to its index: an icon plus type and
// context (a video's duration, a file's name and size), reusing the same labels
// the single-message placeholders use. Photos without a MediaRef fall back to a
// plain photo label rather than dereferencing a nil Media.
func albumBadgeText(msg domain.Message) string {
	if msg.Media != nil {
		return placeholderFor(msg.Media)
	}
	if msg.Photo != nil {
		return "📷 photo"
	}
	return "📦 media"
}

// albumBadgeLabel is the full badge line content for a part: "[n] <type/context>".
func albumBadgeLabel(index int, msg domain.Message) string {
	return "[" + strconv.Itoa(index) + "] " + albumBadgeText(msg)
}
