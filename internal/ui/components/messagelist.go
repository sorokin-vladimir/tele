package components

import (
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/imagecache"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

// MessageList renders a virtual viewport of messages (newest at bottom).
type MessageList struct {
	items             []listItem
	viewStart         int // index of first (possibly partial) visible message
	lineOffset        int // lines of messages[viewStart] to skip from the top
	viewHeight        int
	viewWidth         int
	isGroup           bool
	outboxReadMaxID   int
	inboxReadMaxID    int
	imageCache        *imagecache.Cache
	showIndicator     bool
	hasDarkBackground bool
	renderer          media.Renderer
	maxMediaPx        int        // photos.max_long_side_px; 0 => media package default
	imageMode         media.Mode // inline-image backend; static stickers render in Kitty only

	// Voice playback state: the document being played, its progress (0..1) and
	// current position in seconds. playingVoiceID == 0 means nothing is playing.
	playingVoiceID int64
	voiceProgress  float64
	voicePosition  int

	// GIF loading state: the document being downloaded/decoded and the current
	// spinner glyph shown in its "GIF" badge. gifLoadingID == 0 means none.
	gifLoadingID      int64
	gifLoadingSpinner string

	// selRect is the rectangle of the selected bubble from the most recent
	// View(), in coordinates local to View()'s output. selRectOK is false when
	// no message is selected or no render has happened yet.
	selRect   Rect
	selRectOK bool

	// cursorMsgID is the explicit "active message" the user steps over with
	// CursorUp/CursorDown. 0 means unset; selection then falls back to the
	// newest visible message. It is the target for per-message actions.
	cursorMsgID int

	// highlightedMsgID is the message currently flashed by a highlight;
	// highlightStep counts down HighlightFadeSteps → 0 (0 = none);
	// highlightKind selects the accent (info jump-to vs error rollback).
	highlightedMsgID int
	highlightStep    int
	highlightKind    HighlightKind

	// heightCache memoizes itemHeight by item index. Measuring a message's
	// rendered height runs a full word-wrap (RenderEntities + lipgloss), and
	// ScrollInfo/positionAtBottom/View recompute every item's height several
	// times per frame at the tick cadence — the dominant idle/per-chat CPU cost
	// (issue #146). The cache is keyed by index and fully invalidated whenever
	// anything that affects heights changes (items rebuilt, resize, image load,
	// group flag, media cap), so a stale entry is never read.
	heightCache map[int]int
	// heightComputes counts cache misses (full height computations); test-only
	// instrumentation exposed via HeightComputesForTest.
	heightComputes int
}

// HeightComputesForTest reports the cumulative number of full message-height
// computations (height-cache misses). Tests use deltas across calls to assert
// memoization and invalidation behavior.
func (ml *MessageList) HeightComputesForTest() int { return ml.heightComputes }

// SetVoicePlayback marks a voice message (by document id) as currently playing,
// driving the animated waveform playhead and live position. Pass docID 0 to clear.
func (ml *MessageList) SetVoicePlayback(docID int64, progress float64, posSecs int) {
	ml.playingVoiceID = docID
	ml.voiceProgress = progress
	ml.voicePosition = posSecs
}

// SetGifLoading marks a GIF (by document id) as downloading/decoding so its badge
// shows the given spinner glyph. Pass docID 0 to clear.
func (ml *MessageList) SetGifLoading(docID int64, spinner string) {
	ml.gifLoadingID = docID
	ml.gifLoadingSpinner = spinner
}

// overlayLabelFor returns the thumbnail overlay for a message, adding the loading
// spinner to a GIF badge while that GIF is being fetched.
func (ml *MessageList) overlayLabelFor(msg store.Message) string {
	base := videoOverlayLabel(msg.Media)
	if base == "GIF" && ml.gifLoadingID != 0 && ml.gifLoadingSpinner != "" &&
		msg.Document != nil && msg.Document.ID == ml.gifLoadingID {
		return ml.gifLoadingSpinner + " GIF"
	}
	return base
}

// defaultImageCacheCap bounds the message list's own image cache before the
// shared root cache is injected via SetKnownImages (which replaces it).
const defaultImageCacheCap = 256

func NewMessageList(height, width int) *MessageList {
	return &MessageList{
		viewHeight: height,
		viewWidth:  width,
		imageCache: imagecache.New(defaultImageCacheCap),
		renderer:   media.NewBlockRenderer(),
	}
}

// SetRenderer swaps the active image renderer (block-art or Kitty).
func (ml *MessageList) SetRenderer(r media.Renderer) {
	ml.renderer = r
}

func (ml *MessageList) SetSize(width, height int) {
	if width != ml.viewWidth && ml.renderer != nil {
		ml.renderer.Reset()
	}
	// Heights depend on both dimensions: width drives word-wrap, height caps the
	// inline-image footprint (mediaBox → photoBox). Invalidate on any change.
	if width != ml.viewWidth || height != ml.viewHeight {
		ml.invalidateHeights()
	}
	ml.viewWidth = width
	ml.viewHeight = height
}

func (ml *MessageList) Count() int {
	n := 0
	for _, item := range ml.items {
		if item.kind == itemMessage {
			n++
		}
	}
	return n
}

func (ml *MessageList) ViewStart() int  { return ml.viewStart }
func (ml *MessageList) LineOffset() int { return ml.lineOffset }
func (ml *MessageList) ViewHeight() int { return ml.viewHeight }

func (ml *MessageList) AtTop() bool                   { return ml.viewStart == 0 && ml.lineOffset == 0 }
func (ml *MessageList) SetIsGroup(v bool)             { ml.isGroup = v; ml.invalidateHeights() }
func (ml *MessageList) SetOutboxReadMaxID(id int)     { ml.outboxReadMaxID = id }
func (ml *MessageList) SetInboxReadMaxID(id int)      { ml.inboxReadMaxID = id }
func (ml *MessageList) SetDarkBackground(isDark bool) { ml.hasDarkBackground = isDark }

// SetImageMode tells the list which inline-image backend is active. Static
// stickers only render in Kitty mode (transparency); other modes keep the
// emoji placeholder.
func (ml *MessageList) SetImageMode(mode media.Mode) {
	if mode != ml.imageMode {
		ml.invalidateHeights() // toggles static-sticker inline image → height changes
	}
	ml.imageMode = mode
}

func (ml *MessageList) SetShowIndicator(v bool) { ml.showIndicator = v }
