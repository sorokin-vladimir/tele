package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// wrappedLineCount returns how many rendered rows the message body occupies at
// the given content width. It uses the same lipgloss word-wrap that renderMessage
// applies, so the height estimate and the actual render stay in lock-step. A naive
// ceil(runes/width) under-counts, because word-wrap cannot split words and leaves
// ragged line ends, silently clipping the tail message (issue #115).
func wrappedLineCount(text string, entities []store.MessageEntity, contentW int) int {
	if contentW < 1 {
		contentW = 1
	}
	rendered := RenderEntities(text, entities)
	wrapStyle := lipgloss.NewStyle().Width(contentW)
	n := 0
	for _, part := range strings.Split(rendered, "\n") {
		if part == "" {
			n++ // blank line preserved as one row
			continue
		}
		n += len(strings.Split(wrapStyle.Render(part), "\n"))
	}
	return n
}

// msgHeight estimates the rendered line count for a single message:
// 2 border lines (top with header title + bottom) + wrapped body lines.
// isBareMedia reports whether a message should render borderless (no message
// bubble): a static WEBP sticker or a round video note whose image is loaded,
// with no caption, reply, or forward header that would need the bubble layout.
func (ml *MessageList) isBareMedia(msg store.Message) bool {
	if msg.Text != "" || msg.Forward != nil || msg.ReplyToMsgID != 0 || msg.Media == nil {
		return false
	}
	if !store.IsStaticSticker(msg.Media, msg.Document) && msg.Media.Kind != store.MediaVideoNote {
		return false
	}
	id, ok := ml.PreviewImageID(msg)
	if !ok {
		return false
	}
	_, has := ml.cachedImage(id)
	return has
}

// bareMediaRows returns the media's art-row footprint (without the overlay,
// meta, or sender-name lines). Caller must have verified isBareMedia.
func (ml *MessageList) bareMediaRows(msg store.Message) int {
	id, _ := ml.PreviewImageID(msg)
	img, _ := ml.cachedImage(id)
	bb := img.Bounds()
	_, rows := ml.mediaBox(msg, bb.Dx(), bb.Dy())
	return rows
}

func (ml *MessageList) msgHeight(msg store.Message) int {
	if ml.viewWidth <= 0 {
		return 4
	}
	if ml.isBareMedia(msg) {
		h := ml.bareMediaRows(msg) + 1 // art rows + timestamp line, no borders
		if videoOverlayLabel(msg.Media) != "" {
			h++ // play/duration overlay line (video notes)
		}
		if !msg.IsOut && ml.isGroup {
			h++ // sender-name line above the media
		}
		return h
	}
	maxBubbleW := ml.viewWidth * 3 / 4
	if maxBubbleW < 10 {
		maxBubbleW = 10
	}
	// border(2)+padding(2) = 4 overhead
	maxContentW := maxBubbleW - 4
	if maxContentW < 4 {
		maxContentW = 4
	}

	h := 0

	if msg.Forward != nil {
		// "Forwarded from" label + origin name (renderForwardLines → 2 rows).
		h += 2
		// Blank separator between the forward header and any following content,
		// matching renderMessage; without this the tail clips (issue #115).
		if msg.ReplyToMsgID != 0 || msg.Text != "" || msg.Media != nil {
			h++
		}
	}

	if msg.ReplyToMsgID != 0 {
		if ml.findMessage(msg.ReplyToMsgID) != nil {
			h += 2
		} else {
			h += 1
		}
		if msg.Text != "" || msg.Media != nil {
			h++ // blank separator line between preview and body
		}
	}

	if msg.Media != nil {
		// Reserve the full image footprint as soon as the image bytes are known,
		// regardless of whether a Kitty placement has been transmitted yet. The
		// renderer draws a full-height placeholder box until the image is ready,
		// so the rendered height always equals this reserved height: no hidden
		// tail (issue #115) and no scroll jump when the placement lands.
		if id, ok := ml.PreviewImageID(msg); ok {
			if img, has := ml.cachedImage(id); has {
				b := img.Bounds()
				_, rows := ml.mediaBox(msg, b.Dx(), b.Dy())
				h += rows
				if videoOverlayLabel(msg.Media) != "" {
					h++ // play/duration overlay line under the thumbnail
				}
			} else {
				h++ // text placeholder line (bytes not downloaded yet)
			}
		} else {
			h++ // text placeholder line
		}
		if msg.Text != "" {
			h++ // blank separator line between media and caption
		}
	}

	if msg.Text != "" {
		h += wrappedLineCount(msg.Text, msg.Entities, maxContentW)
	}

	if h == 0 {
		h = 1 // at least one content line for empty-text messages
	}
	return h + 2 // +2 border lines (top+bottom)
}

// itemHeight returns the rendered line count for item i, memoized in heightCache.
// The expensive path (msgHeight → full word-wrap) runs only on a cache miss;
// invalidateHeights clears the cache whenever an input to the height changes.
func (ml *MessageList) itemHeight(i int) int {
	if h, ok := ml.heightCache[i]; ok {
		return h
	}
	h := ml.computeItemHeight(i)
	if ml.heightCache == nil {
		ml.heightCache = make(map[int]int, len(ml.items))
	}
	ml.heightCache[i] = h
	return h
}

func (ml *MessageList) computeItemHeight(i int) int {
	ml.heightComputes++
	if ml.items[i].kind == itemDateSeparator || ml.items[i].kind == itemUnreadSeparator {
		return 3
	}
	return ml.msgHeight(ml.items[i].msg)
}

// invalidateHeights drops every memoized height. Cheap and called only on
// non-per-frame mutations (items rebuilt, resize, image load, group/media-cap
// changes), so steady-state frames keep hitting a warm cache.
func (ml *MessageList) invalidateHeights() {
	clear(ml.heightCache)
}
