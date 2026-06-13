package components

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

var (
	inNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	editNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	tsStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sentStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	readStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	indicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	quoteStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	sepStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	unreadSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

var senderPalette = [8]struct{ light, dark color.Color }{
	{lipgloss.Color("1"), lipgloss.Color("9")},
	{lipgloss.Color("2"), lipgloss.Color("10")},
	{lipgloss.Color("4"), lipgloss.Color("12")},
	{lipgloss.Color("5"), lipgloss.Color("13")},
	{lipgloss.Color("6"), lipgloss.Color("14")},
	{lipgloss.Color("130"), lipgloss.Color("208")},
	{lipgloss.Color("92"), lipgloss.Color("141")},
	{lipgloss.Color("28"), lipgloss.Color("118")},
}

type itemKind int

const (
	itemMessage         itemKind = iota
	itemDateSeparator            // date separator, 3 lines: blank + label + blank
	itemUnreadSeparator          // "New Messages" divider
)

type listItem struct {
	kind  itemKind
	msg   store.Message // valid when kind == itemMessage
	label string        // valid when kind == itemDateSeparator, e.g. "May 18"
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func formatSepLabel(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.Month() == now.Month() && t.Day() == now.Day() {
		return "Today"
	}
	if t.Year() == now.Year() {
		return t.Format("January 2")
	}
	return t.Format("January 2, 2006")
}

func (ml *MessageList) buildItems(msgs []store.Message) []listItem {
	if len(msgs) == 0 {
		return nil
	}
	items := make([]listItem, 0, len(msgs)+4)
	var prev time.Time
	unreadInserted := false
	for _, msg := range msgs {
		if !sameDay(prev, msg.Date) {
			items = append(items, listItem{kind: itemDateSeparator, label: formatSepLabel(msg.Date)})
			prev = msg.Date
		}
		// The divider marks the first incoming unread message; own outgoing
		// messages never anchor it, even though their ID exceeds inboxReadMaxID.
		if !unreadInserted && !msg.IsOut && ml.inboxReadMaxID > 0 && msg.ID > ml.inboxReadMaxID {
			items = append(items, listItem{kind: itemUnreadSeparator})
			unreadInserted = true
		}
		items = append(items, listItem{kind: itemMessage, msg: msg})
	}
	return items
}

func buildReactStr(reactions []store.Reaction) string {
	if len(reactions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reactions))
	for _, r := range reactions {
		s := r.Emoji + " " + strconv.Itoa(r.Count)
		if r.IsChosen {
			parts = append(parts, readStyle.Render(s))
		} else {
			parts = append(parts, tsStyle.Render(s))
		}
	}
	sep := tsStyle.Render(" · ")
	return " " + strings.Join(parts, sep) + " "
}

const (
	indicatorChar = "┃"
	quoteGlyph    = "▌ "
)

func replyName(orig *store.Message) string {
	if orig.SenderName != "" {
		return orig.SenderName
	}
	if orig.IsOut {
		return "You"
	}
	return "?"
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// measurePreviewBlock returns the content width needed for a reply preview
// block, accounting for both the original sender's name and the quoted snippet,
// capped at maxContentW. Measuring the snippet keeps a short reply (e.g. "ok")
// from squeezing the original message down to an unreadable width.
func measurePreviewBlock(senderName, snippet string, maxContentW int) int {
	w := lipgloss.Width(quoteGlyph + inNameStyle.Render(senderName))
	if sw := lipgloss.Width(quoteGlyph + quoteStyle.Render(snippet)); sw > w {
		w = sw
	}
	if w > maxContentW {
		return maxContentW
	}
	return w
}

// renderPreviewLines returns the bubble content lines for a reply or forward
// preview block. senderName == "" signals the original is unavailable
// (renders a placeholder). actualW is the finalized content width for the bubble.
func (ml *MessageList) renderPreviewLines(senderID int64, senderName, snippet string, actualW int, bs lipgloss.Style) []string {
	b := lipgloss.RoundedBorder()
	glyphW := lipgloss.Width(quoteGlyph)

	if senderName == "" {
		placeholder := quoteGlyph + quoteStyle.Render("Original not available")
		pw := lipgloss.Width(placeholder)
		if pw < actualW {
			placeholder += strings.Repeat(" ", actualW-pw)
		}
		return []string{bs.Render(b.Left) + " " + placeholder + " " + bs.Render(b.Right)}
	}

	ns := ml.senderNameStyle(senderID)
	namePart := ns.Render(quoteGlyph) + ns.Render(senderName)
	nw := lipgloss.Width(namePart)
	if nw > actualW {
		maxNameW := actualW - glyphW - 1
		if maxNameW < 1 {
			maxNameW = 1
		}
		senderName = runewidth.Truncate(senderName, maxNameW, "…")
		namePart = ns.Render(quoteGlyph) + ns.Render(senderName)
		nw = lipgloss.Width(namePart)
	}
	if nw < actualW {
		namePart += strings.Repeat(" ", actualW-nw)
	}
	nameRow := bs.Render(b.Left) + " " + namePart + " " + bs.Render(b.Right)

	maxSnippetW := actualW - glyphW
	if maxSnippetW < 1 {
		maxSnippetW = 1
	}
	snippet = runewidth.Truncate(snippet, maxSnippetW, "…")
	textPart := ns.Render(quoteGlyph) + quoteStyle.Render(snippet)
	tw := lipgloss.Width(textPart)
	if tw < actualW {
		textPart += strings.Repeat(" ", actualW-tw)
	}
	snippetRow := bs.Render(b.Left) + " " + textPart + " " + bs.Render(b.Right)

	return []string{nameRow, snippetRow}
}

const (
	forwardLabelText  = "Forwarded from"
	forwardHiddenName = "Hidden"
)

// measureForwardBlock returns the content width needed for a forwarded-message
// header block (the "Forwarded from" label and the origin name), capped at
// maxContentW.
func measureForwardBlock(from string, maxContentW int) int {
	name := from
	if name == "" {
		name = forwardHiddenName
	}
	w := lipgloss.Width(quoteGlyph + inNameStyle.Render(name))
	if lw := lipgloss.Width(quoteGlyph + quoteStyle.Render(forwardLabelText)); lw > w {
		w = lw
	}
	if w > maxContentW {
		return maxContentW
	}
	return w
}

// renderForwardLines returns the bubble content lines for a forwarded-message
// header block: a dim "Forwarded from" label line followed by the origin name.
// An empty from renders the hidden-sender placeholder. actualW is the finalized
// content width for the bubble.
func renderForwardLines(from string, actualW int, bs lipgloss.Style) []string {
	b := lipgloss.RoundedBorder()
	glyphW := lipgloss.Width(quoteGlyph)

	name := from
	if name == "" {
		name = forwardHiddenName
	}

	maxTextW := actualW - glyphW
	if maxTextW < 1 {
		maxTextW = 1
	}

	label := runewidth.Truncate(forwardLabelText, maxTextW, "…")
	labelPart := quoteStyle.Render(quoteGlyph) + quoteStyle.Render(label)
	if lw := lipgloss.Width(labelPart); lw < actualW {
		labelPart += strings.Repeat(" ", actualW-lw)
	}
	labelRow := bs.Render(b.Left) + " " + labelPart + " " + bs.Render(b.Right)

	name = runewidth.Truncate(name, maxTextW, "…")
	namePart := quoteStyle.Render(quoteGlyph) + inNameStyle.Render(name)
	if nw := lipgloss.Width(namePart); nw < actualW {
		namePart += strings.Repeat(" ", actualW-nw)
	}
	nameRow := bs.Render(b.Left) + " " + namePart + " " + bs.Render(b.Right)

	return []string{labelRow, nameRow}
}

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
	images            map[int64]image.Image
	showIndicator     bool
	hasDarkBackground bool
	renderer          media.Renderer

	// Voice playback state: the document being played, its progress (0..1) and
	// current position in seconds. playingVoiceID == 0 means nothing is playing.
	playingVoiceID int64
	voiceProgress  float64
	voicePosition  int

	// selRect is the rectangle of the selected bubble from the most recent
	// View(), in coordinates local to View()'s output. selRectOK is false when
	// no message is selected or no render has happened yet.
	selRect   Rect
	selRectOK bool
}

// SetVoicePlayback marks a voice message (by document id) as currently playing,
// driving the animated waveform playhead and live position. Pass docID 0 to clear.
func (ml *MessageList) SetVoicePlayback(docID int64, progress float64, posSecs int) {
	ml.playingVoiceID = docID
	ml.voiceProgress = progress
	ml.voicePosition = posSecs
}

func NewMessageList(height, width int) *MessageList {
	return &MessageList{
		viewHeight: height,
		viewWidth:  width,
		images:     make(map[int64]image.Image),
		renderer:   media.NewBlockRenderer(),
	}
}

// SetRenderer swaps the active image renderer (block-art or Kitty).
func (ml *MessageList) SetRenderer(r media.Renderer) {
	ml.renderer = r
}

// PhotoContentCols exposes the width (in cells) photos are rendered at.
func (ml *MessageList) PhotoContentCols() int {
	return ml.photoContentCols()
}

// PhotoFootprint exposes the active renderer's row footprint for an image, so
// callers (e.g. Kitty transmit) stay in lock-step with the rendered grid.
func (ml *MessageList) PhotoFootprint(imgW, imgH, cols int) int {
	return ml.renderer.Footprint(imgW, imgH, cols)
}

// SetImage caches a downloaded photo for rendering.
// If the viewport was at the natural bottom before the image changed message heights,
// it is re-anchored to the new natural bottom so newest messages stay visible.
func (ml *MessageList) SetImage(photoID int64, img image.Image) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
	ml.images[photoID] = img
	if wasAtBottom {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	}
}

// SetKnownImages bulk-loads images from an external cache.
// Re-anchors to the natural bottom if the viewport was there before the load.
func (ml *MessageList) SetKnownImages(cache map[int64]image.Image) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
	for id, img := range cache {
		ml.images[id] = img
	}
	if wasAtBottom {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	}
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
		return "📎 file"
	case store.MediaLocation:
		return "📍 location"
	default:
		return "📦 media"
	}
}

// durationLabel appends a mm:ss suffix to a base label when the duration is known.
func durationLabel(base string, dur int) string {
	if dur > 0 {
		return base + " " + formatDuration(dur)
	}
	return base
}

// PreviewImageID returns the image-cache key for a message's inline thumbnail
// (photos and videos with an embedded thumbnail) and whether one applies.
func PreviewImageID(msg store.Message) (int64, bool) {
	if msg.Media == nil {
		return 0, false
	}
	switch {
	case msg.Media.Kind == store.MediaPhoto && msg.Photo != nil:
		return msg.Photo.ID, true
	case msg.Media.Kind.IsVideo() && msg.Document != nil && msg.Document.ThumbSize != "":
		return msg.Document.ID, true
	}
	return 0, false
}

// videoOverlayLabel returns the play affordance shown under a video thumbnail,
// or "" for non-video media.
func videoOverlayLabel(m *store.MediaRef) string {
	if m != nil && m.Kind.IsVideo() {
		return "▶ " + formatDuration(m.Duration)
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

func (ml *MessageList) SetSize(width, height int) {
	if width != ml.viewWidth && ml.renderer != nil {
		ml.renderer.Reset()
	}
	ml.viewWidth = width
	ml.viewHeight = height
}

func (ml *MessageList) SetMessages(msgs []store.Message) {
	ml.items = ml.buildItems(msgs)
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
}

// SetMessagesKeepScroll replaces the message list without resetting the scroll position.
// Use for in-place data updates (e.g. reactions, edits) where the message count is
// unchanged.
//
// An edit can change a message's line count. If the viewport was at the natural
// bottom, re-anchor to the new bottom so the newest content stays fully visible
// instead of being clipped by a now-stale offset (same fix as SetImage). When
// scrolled up in history, keep the top anchor so the position does not jump.
func (ml *MessageList) SetMessagesKeepScroll(msgs []store.Message) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff

	vs, lo := ml.viewStart, ml.lineOffset
	ml.items = ml.buildItems(msgs)
	if wasAtBottom {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
		return
	}
	if vs >= len(ml.items) {
		vs = max(0, len(ml.items)-1)
		lo = 0
	}
	ml.viewStart, ml.lineOffset = vs, lo
}

// RemoveMessage removes the message with the given ID while preserving scroll position.
func (ml *MessageList) RemoveMessage(id int) {
	found := false
	msgs := make([]store.Message, 0, len(ml.items))
	for _, item := range ml.items {
		if item.kind != itemMessage {
			continue
		}
		if item.msg.ID == id {
			found = true
		} else {
			msgs = append(msgs, item.msg)
		}
	}
	if !found {
		return
	}

	var anchorID int
	for i := ml.viewStart; i < len(ml.items); i++ {
		if ml.items[i].kind == itemMessage {
			anchorID = ml.items[i].msg.ID
			break
		}
	}

	ml.items = ml.buildItems(msgs)

	if len(ml.items) == 0 {
		ml.viewStart = 0
		ml.lineOffset = 0
		return
	}

	if anchorID != 0 && anchorID != id {
		for i, item := range ml.items {
			if item.kind == itemMessage && item.msg.ID == anchorID {
				ml.viewStart = i
				return
			}
		}
	}

	if ml.viewStart >= len(ml.items) {
		ml.viewStart = len(ml.items) - 1
		ml.lineOffset = 0
	}
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

// ScrollInfo reports the message list's scroll position in rendered lines,
// measured over the currently loaded window. Total grows as older history is
// prepended; the viewport is top-anchored so visible content does not jump.
func (ml *MessageList) ScrollInfo() ScrollInfo {
	total := 0
	for i := range ml.items {
		total += ml.itemHeight(i)
	}
	offset := 0
	for i := 0; i < ml.viewStart && i < len(ml.items); i++ {
		offset += ml.itemHeight(i)
	}
	offset += ml.lineOffset
	return ScrollInfo{Total: total, Visible: ml.viewHeight, Offset: offset}
}

// VisiblePhotoIDs returns the inline-image cache keys (PreviewImageID) for the
// messages currently within the viewport, top to bottom. Kitty placements are a
// bounded terminal resource, so the root transmits/keeps only on-screen images.
// The visible range mirrors View's accumulation from viewStart.
func (ml *MessageList) VisiblePhotoIDs() []int64 {
	var ids []int64
	lines := 0
	for i := ml.viewStart; i < len(ml.items) && lines < ml.viewHeight; i++ {
		h := ml.itemHeight(i)
		if i == ml.viewStart {
			h -= ml.lineOffset
		}
		lines += h
		if ml.items[i].kind == itemMessage {
			if id, ok := PreviewImageID(ml.items[i].msg); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// SelectedBubbleRect returns the rectangle of the selected message bubble from
// the most recent View() call, local to View()'s output. ok is false when there
// is no selected message or View() has not run yet.
func (ml *MessageList) SelectedBubbleRect() (Rect, bool) { return ml.selRect, ml.selRectOK }
func (ml *MessageList) AtTop() bool                      { return ml.viewStart == 0 && ml.lineOffset == 0 }
func (ml *MessageList) SetIsGroup(v bool)                { ml.isGroup = v }
func (ml *MessageList) SetOutboxReadMaxID(id int)        { ml.outboxReadMaxID = id }
func (ml *MessageList) SetInboxReadMaxID(id int)         { ml.inboxReadMaxID = id }
func (ml *MessageList) SetDarkBackground(isDark bool)    { ml.hasDarkBackground = isDark }

func (ml *MessageList) senderNameStyle(senderID int64) lipgloss.Style {
	idx := senderID % 8
	if idx < 0 {
		idx = -idx
	}
	pair := senderPalette[idx]
	fg := lipgloss.LightDark(ml.hasDarkBackground)(pair.light, pair.dark)
	return lipgloss.NewStyle().Foreground(fg).Bold(true)
}

func (ml *MessageList) ScrollToBottom() {
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
}

func (ml *MessageList) ScrollToTop() {
	ml.viewStart = 0
	ml.lineOffset = 0
}

func (ml *MessageList) ScrollDownBy(n int) {
	for i := 0; i < n; i++ {
		ml.ScrollDown()
	}
}

func (ml *MessageList) ScrollUpBy(n int) {
	for i := 0; i < n; i++ {
		ml.ScrollUp()
	}
}

// PrependMessages inserts older messages at the front and shifts viewStart so
// that the currently-visible messages stay on screen. Messages whose IDs already
// exist in the list are skipped: rapid scroll-up can fire several identical
// "load older" requests before the first resolves, so the same chunk may arrive
// more than once. Without this guard the duplicates would stack into a repeating
// date-range "ring" that never advances toward older history (issue #120).
func (ml *MessageList) PrependMessages(older []store.Message) {
	if len(older) == 0 {
		return
	}
	current := make([]store.Message, 0, len(ml.items))
	existing := make(map[int]struct{}, len(ml.items))
	for _, item := range ml.items {
		if item.kind == itemMessage {
			current = append(current, item.msg)
			existing[item.msg.ID] = struct{}{}
		}
	}
	fresh := make([]store.Message, 0, len(older))
	for _, msg := range older {
		if _, dup := existing[msg.ID]; dup {
			continue
		}
		fresh = append(fresh, msg)
	}
	if len(fresh) == 0 {
		return
	}
	oldLen := len(ml.items)
	ml.items = ml.buildItems(append(fresh, current...))
	ml.viewStart += len(ml.items) - oldLen
}

func (ml *MessageList) OldestID() int {
	for _, item := range ml.items {
		if item.kind == itemMessage {
			return item.msg.ID
		}
	}
	return 0
}

// VisibleReadMaxID returns the highest message ID that is "sufficiently visible" to count
// as read: either more than half its lines are in the viewport, or it fills the entire
// viewport (so more than half is impossible to show at once). Returns 0 if none qualify.
func (ml *MessageList) VisibleReadMaxID() int {
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 || len(ml.items) == 0 {
		return 0
	}
	maxID := 0
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.items) && linesUsed < ml.viewHeight; i++ {
		h := ml.itemHeight(i)
		if ml.items[i].kind != itemMessage {
			linesUsed += h
			continue
		}
		msg := ml.items[i].msg
		skipped := 0
		if i == ml.viewStart {
			skipped = ml.lineOffset
		}
		visibleLines := h - skipped
		remaining := ml.viewHeight - linesUsed
		if visibleLines > remaining {
			visibleLines = remaining
		}
		if visibleLines > 0 && (visibleLines*2 > h || h >= ml.viewHeight) {
			if msg.ID > maxID {
				maxID = msg.ID
			}
		}
		linesUsed += visibleLines
	}
	return maxID
}

// ScrollToFirstUnread positions the viewport at the first message with ID > readMaxID.
// If an itemUnreadSeparator immediately precedes that message it is included at the top
// so the divider is always visible. If the remaining messages don't fill the viewport,
// older messages are pulled in to fill the space (same as positionAtBottom).
// Returns false if all messages are already read (nothing to jump to).
func (ml *MessageList) ScrollToFirstUnread(readMaxID int) bool {
	for i, item := range ml.items {
		if item.kind != itemMessage {
			continue
		}
		if item.msg.ID > readMaxID {
			start := i
			if i > 0 && ml.items[i-1].kind == itemUnreadSeparator {
				start = i - 1
			}
			ml.viewStart = start
			ml.lineOffset = 0
			lines := 0
			for j := start; j < len(ml.items); j++ {
				lines += ml.itemHeight(j)
			}
			if lines < ml.viewHeight {
				ml.viewStart, ml.lineOffset = ml.positionAtBottom()
			}
			return true
		}
	}
	return false
}

// ScrollUp moves the viewport one line toward older messages.
// When crossing a message boundary, small messages (h <= viewHeight) are entered at
// lineOffset=h-2 so at least content+bottom are visible (never bottom-border-only).
// Large messages are entered at their bottom portion (lineOffset=h-viewHeight).
func (ml *MessageList) ScrollUp() {
	if ml.lineOffset > 0 {
		ml.lineOffset--
		return
	}
	if ml.viewStart > 0 {
		ml.viewStart--
		h := ml.itemHeight(ml.viewStart)
		if h > ml.viewHeight {
			ml.lineOffset = h - ml.viewHeight
		} else {
			// Enter showing content+bottom border; lineOffset=h-1 (bottom-only) is skipped.
			ml.lineOffset = h - 2
		}
	}
}

// ScrollDown moves the viewport one line toward newer messages.
// Scrolls line-by-line but skips lineOffset=h-1 (bottom-border-only frame).
// The at-bottom check (positionAtBottom) is the primary stop condition.
func (ml *MessageList) ScrollDown() {
	botIdx, botOff := ml.positionAtBottom()
	if ml.viewStart > botIdx || (ml.viewStart == botIdx && ml.lineOffset >= botOff) {
		return
	}
	h := ml.itemHeight(ml.viewStart)
	if ml.lineOffset+1 < h-1 {
		ml.lineOffset++
		return
	}
	if ml.viewStart+1 < len(ml.items) {
		ml.viewStart++
		ml.lineOffset = 0
	}
}

// positionAtBottom returns (msgIdx, lineOffset) for the viewport bottom position.
// lineOffset > 0 means the first visible message is shown from its bottom portion,
// filling the space that would otherwise be empty above the last full messages.
func (ml *MessageList) positionAtBottom() (int, int) {
	lineCount := 0
	for i := len(ml.items) - 1; i >= 0; i-- {
		h := ml.itemHeight(i)
		if lineCount+h >= ml.viewHeight {
			// Adding this message meets or exceeds the viewport.
			// Show it from the offset that makes total lines == viewHeight.
			overflow := lineCount + h - ml.viewHeight
			return i, overflow
		}
		lineCount += h
	}
	return 0, 0
}

// msgHeight estimates the rendered line count for a single message:
// 2 border lines (top with header title + bottom) + wrapped body lines.
func (ml *MessageList) msgHeight(msg store.Message) int {
	if ml.viewWidth <= 0 {
		return 4
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
		cols := ml.photoContentCols()
		// Reserve the full image footprint as soon as the image bytes are known,
		// regardless of whether a Kitty placement has been transmitted yet. The
		// renderer draws a full-height placeholder box until the image is ready,
		// so the rendered height always equals this reserved height: no hidden
		// tail (issue #115) and no scroll jump when the placement lands.
		if id, ok := PreviewImageID(msg); ok {
			if img, has := ml.images[id]; has {
				b := img.Bounds()
				h += ml.renderer.Footprint(b.Dx(), b.Dy(), cols)
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
		for _, part := range strings.Split(msg.Text, "\n") {
			r := []rune(part)
			if len(r) == 0 {
				h++
			} else {
				h += (len(r) + maxContentW - 1) / maxContentW
			}
		}
	}

	if h == 0 {
		h = 1 // at least one content line for empty-text messages
	}
	return h + 2 // +2 border lines (top+bottom)
}

func (ml *MessageList) itemHeight(i int) int {
	if ml.items[i].kind == itemDateSeparator || ml.items[i].kind == itemUnreadSeparator {
		return 3
	}
	return ml.msgHeight(ml.items[i].msg)
}

func (ml *MessageList) SetShowIndicator(v bool) { ml.showIndicator = v }

func (ml *MessageList) findMessage(id int) *store.Message {
	for i := range ml.items {
		if ml.items[i].kind == itemMessage && ml.items[i].msg.ID == id {
			return &ml.items[i].msg
		}
	}
	return nil
}

func (ml *MessageList) SelectedMessageID() int {
	return ml.computeSelectedMsgID()
}

func (ml *MessageList) SelectedMessageIsOut() bool {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.IsOut
	}
	return false
}

func (ml *MessageList) SelectedMessageReplyToMsgID() int {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.ReplyToMsgID
	}
	return 0
}

func (ml *MessageList) SelectedMessagePhotoID() int64 {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Photo != nil {
		return msg.Photo.ID
	}
	return 0
}

// SelectedMessageVideo returns the document ref of the selected message when it
// is a playable video, for opening in an external player.
func (ml *MessageList) SelectedMessageVideo() (store.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind.IsVideo() && msg.Document != nil {
		return *msg.Document, true
	}
	return store.DocumentRef{}, false
}

// SelectedMessageVoice returns the document ref of the selected message when it
// is a voice message, for in-app playback.
func (ml *MessageList) SelectedMessageVoice() (store.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind == store.MediaVoice && msg.Document != nil {
		return *msg.Document, true
	}
	return store.DocumentRef{}, false
}

func (ml *MessageList) ScrollToMessage(id int) bool {
	for i, item := range ml.items {
		if item.kind != itemMessage || item.msg.ID != id {
			continue
		}
		ml.viewStart = i
		ml.lineOffset = 0
		lines := 0
		for j := i; j < len(ml.items); j++ {
			lines += ml.itemHeight(j)
		}
		if lines < ml.viewHeight {
			ml.viewStart, ml.lineOffset = ml.positionAtBottom()
		}
		return true
	}
	return false
}

func (ml *MessageList) computeSelectedMsgID() int {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.ID
	}
	return 0
}

func (ml *MessageList) computeSelectedMsg() *store.Message {
	if len(ml.items) == 0 {
		return nil
	}
	selectedIdx := -1
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.items); i++ {
		skipped := 0
		if i == ml.viewStart {
			skipped = ml.lineOffset
		}
		h := ml.itemHeight(i)
		if ml.items[i].kind == itemMessage {
			firstContentVP := linesUsed + (1 - skipped)
			if firstContentVP >= 0 && firstContentVP < ml.viewHeight {
				selectedIdx = i
			}
		}
		visible := h - skipped
		if visible < 0 {
			visible = 0
		}
		linesUsed += visible
		if linesUsed >= ml.viewHeight {
			break
		}
	}
	if selectedIdx < 0 {
		return nil
	}
	return &ml.items[selectedIdx].msg
}

// renderMessage returns the display lines for a single message bubble.
// selected: when true, injects << / >> indicator into allLines[1] (added in Task 2).
func (ml *MessageList) renderMessage(msg store.Message, selected bool) []string {
	if ml.viewWidth <= 0 {
		return []string{""}
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

	var borderFg color.Color
	if msg.IsOut {
		borderFg = lipgloss.Color("25")
	} else {
		borderFg = lipgloss.Color("238")
	}
	b := lipgloss.RoundedBorder()
	bs := lipgloss.NewStyle().Foreground(borderFg)

	// Measure content width from text only.
	actualW := 0
	if msg.Text != "" {
		measureStyle := lipgloss.NewStyle().Width(maxContentW)
		for _, part := range strings.Split(msg.Text, "\n") {
			if part == "" {
				continue
			}
			for _, wl := range strings.Split(measureStyle.Render(part), "\n") {
				if w := lipgloss.Width(strings.TrimRight(wl, " ")); w > actualW {
					actualW = w
				}
			}
		}
		if actualW > maxContentW {
			actualW = maxContentW
		}
	}
	if actualW < 1 {
		actualW = 1
	}

	// Ensure photo content width is reflected in bubble sizing. Photos pre-size
	// the bubble even before the image loads; video thumbnails widen it only
	// once the thumbnail is available (the text placeholder is narrow).
	widenToPhotoCols := msg.Photo != nil
	if !widenToPhotoCols {
		if id, ok := PreviewImageID(msg); ok {
			if _, has := ml.images[id]; has {
				widenToPhotoCols = true
			}
		}
	}
	if widenToPhotoCols {
		photoCols := ml.photoContentCols()
		if photoCols > actualW {
			actualW = photoCols
		}
	}

	// Ensure bubble is wide enough for a media placeholder label.
	// Measured with lipgloss.Width so wide emoji match placeholderLine's padding.
	if msg.Media != nil {
		if w := lipgloss.Width(placeholderFor(msg.Media)); w > actualW {
			actualW = w
		}
	}

	// Ensure bubble is wide enough for the forwarded-message header block.
	if msg.Forward != nil {
		if minW := measureForwardBlock(msg.Forward.From, maxContentW); actualW < minW {
			actualW = minW
		}
	}

	// Ensure bubble is wide enough for the reply preview block.
	if msg.ReplyToMsgID != 0 {
		orig := ml.findMessage(msg.ReplyToMsgID)
		var minW int
		if orig != nil {
			minW = measurePreviewBlock(replyName(orig), firstLine(orig.Text), maxContentW)
		} else {
			w := lipgloss.Width(quoteGlyph + quoteStyle.Render("Original not available"))
			if w > maxContentW {
				w = maxContentW
			}
			minW = w
		}
		if actualW < minW {
			actualW = minW
		}
	}

	// innerW = actualW (content) + 2 (padding 1 each side).
	innerW := actualW + 2

	// Timestamp + optional status indicator in bottom border.
	var statusStr string
	if msg.IsOut {
		if msg.ID > 0 && msg.ID <= ml.outboxReadMaxID {
			statusStr = " " + readStyle.Render("✓✓")
		} else if msg.ID > 0 {
			statusStr = " " + sentStyle.Render("✓")
		}
	}
	editMark := ""
	if msg.EditDate != nil {
		editMark = tsStyle.Render("edited") + " · "
	}
	tsStr := " " + editMark + tsStyle.Render(msg.Date.Format("15:04")) + statusStr + " "
	tsW := lipgloss.Width(tsStr)
	if innerW < tsW {
		innerW = tsW
		actualW = innerW - 2
	}
	reactStr := buildReactStr(msg.Reactions)
	reactW := lipgloss.Width(reactStr)
	if innerW < reactW+tsW+1 {
		innerW = reactW + tsW + 1
		actualW = innerW - 2
	}

	// Ensure bubble is wide enough for the sender name in the top border.
	// rightFill = innerW - titleW - 1 must be >= 0, so innerW >= titleW + 1.
	if !msg.IsOut && ml.isGroup {
		name := msg.SenderName
		if name == "" {
			name = "?"
		}
		titleW := lipgloss.Width(" " + ml.senderNameStyle(msg.SenderID).Render(name) + " ")
		if innerW < titleW+1 {
			innerW = titleW + 1
			actualW = innerW - 2
		}
	}

	// Top border: sender/indicator left-aligned for incoming; plain for outgoing.
	var top string
	if !msg.IsOut {
		var senderStyled string
		if ml.isGroup {
			name := msg.SenderName
			if name == "" {
				name = "?"
			}
			senderStyled = ml.senderNameStyle(msg.SenderID).Render(name)
		}
		var titleStr string
		if senderStyled != "" {
			titleStr = " " + senderStyled + " "
		}
		titleW := lipgloss.Width(titleStr)
		rightFill := innerW - titleW - 1 // 1 fill char on the left
		if rightFill < 0 {
			rightFill = 0
		}
		top = bs.Render(b.TopLeft+b.Top) + titleStr + bs.Render(strings.Repeat(b.Top, rightFill)+b.TopRight)
	} else {
		top = bs.Render(b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight)
	}

	// Bottom border: reactions left, timestamp right.
	fillW := innerW - reactW - tsW
	if fillW < 0 {
		fillW = 0
	}
	bottom := bs.Render(b.BottomLeft) + reactStr + bs.Render(strings.Repeat(b.Bottom, fillW)) + tsStr + bs.Render(b.BottomRight)

	// Content lines: forward header (if any), reply quote block (if reply),
	// photo art (if any), then text.
	var sideLines []string

	if msg.Forward != nil {
		sideLines = append(sideLines, renderForwardLines(msg.Forward.From, actualW, bs)...)
		// Separate the forward header from any following content with a blank line.
		if msg.ReplyToMsgID != 0 || msg.Text != "" || msg.Media != nil {
			sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
		}
	}

	if msg.ReplyToMsgID != 0 {
		orig := ml.findMessage(msg.ReplyToMsgID)
		var origSenderID int64
		var name, snippet string
		if orig != nil {
			origSenderID = orig.SenderID
			name = replyName(orig)
			snippet = firstLine(orig.Text)
		}
		sideLines = append(sideLines, ml.renderPreviewLines(origSenderID, name, snippet, actualW, bs)...)
		if msg.Text != "" || msg.Media != nil {
			sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
		}
	}

	if msg.Media != nil {
		cols := ml.photoContentCols()
		var artLines []string
		hasBytes, footprint := false, 0
		if id, ok := PreviewImageID(msg); ok {
			if img, has := ml.images[id]; has {
				hasBytes = true
				bb := img.Bounds()
				footprint = ml.renderer.Footprint(bb.Dx(), bb.Dy(), cols)
				artLines = ml.renderer.Render(id, img, cols)
			}
		}
		blankRow := bs.Render(b.Left) + strings.Repeat(" ", innerW) + bs.Render(b.Right)
		switch {
		case artLines != nil:
			for _, al := range artLines {
				lw := lipgloss.Width(al)
				if lw < actualW {
					al += strings.Repeat(" ", actualW-lw)
				}
				sideLines = append(sideLines, bs.Render(b.Left)+" "+al+" "+bs.Render(b.Right))
			}
			if overlay := videoOverlayLabel(msg.Media); overlay != "" {
				sideLines = append(sideLines, labelLine(overlay, actualW, b, bs))
			}
		case hasBytes:
			// Bytes are known but the Kitty placement is not transmitted yet. Fill
			// the full reserved footprint with a placeholder box (label on the first
			// row) so the rendered height matches msgHeight — the image swaps in at
			// the same size with no scroll jump or hidden tail (issue #115).
			for i := 0; i < footprint; i++ {
				if i == 0 {
					sideLines = append(sideLines, placeholderLine(msg.Media, actualW, b, bs))
				} else {
					sideLines = append(sideLines, blankRow)
				}
			}
			if videoOverlayLabel(msg.Media) != "" {
				sideLines = append(sideLines, labelLine(videoOverlayLabel(msg.Media), actualW, b, bs))
			}
		case msg.Media.Kind == store.MediaVoice && msg.Document != nil &&
			msg.Document.ID == ml.playingVoiceID:
			// Voice currently playing: waveform with playhead + live position.
			label := voicePlayingLabel(msg.Media, ml.voiceProgress, ml.voicePosition)
			sideLines = append(sideLines, labelLine(label, actualW, b, bs))
		default:
			sideLines = append(sideLines, placeholderLine(msg.Media, actualW, b, bs))
		}
		if msg.Text != "" {
			sideLines = append(sideLines, blankRow)
		}
	}

	if msg.Text != "" {
		rendered := RenderEntities(msg.Text, msg.Entities)
		wrapStyle := lipgloss.NewStyle().Width(actualW)
		for _, part := range strings.Split(rendered, "\n") {
			if part == "" {
				sideLines = append(sideLines, bs.Render(b.Left)+strings.Repeat(" ", innerW)+bs.Render(b.Right))
				continue
			}
			for _, wl := range strings.Split(wrapStyle.Render(part), "\n") {
				lw := lipgloss.Width(wl)
				if lw < actualW {
					wl += strings.Repeat(" ", actualW-lw)
				}
				sideLines = append(sideLines, bs.Render(b.Left)+" "+wl+" "+bs.Render(b.Right))
			}
		}
	} else if len(sideLines) == 0 {
		sideLines = []string{bs.Render(b.Left) + strings.Repeat(" ", innerW) + bs.Render(b.Right)}
	}

	allLines := make([]string, 0, len(sideLines)+2)
	allLines = append(allLines, top)
	allLines = append(allLines, sideLines...)
	allLines = append(allLines, bottom)

	// Outgoing bubbles are right-aligned; incoming stay at the left margin.
	if msg.IsOut {
		bubbleW := lipgloss.Width(allLines[0])
		leftPad := ml.viewWidth - bubbleW
		if leftPad < 0 {
			leftPad = 0
		}
		pad := strings.Repeat(" ", leftPad)
		for i := range allLines {
			allLines[i] = pad + allLines[i]
		}
		// Draw indicator bar on every content line (all except top and bottom border).
		// First leftPad bytes are ASCII spaces, so byte-slicing is safe.
		if selected && ml.showIndicator && len(allLines) > 2 && leftPad >= 2 {
			bar := " " + indicatorStyle.Render(indicatorChar)
			for i := 1; i < len(allLines)-1; i++ {
				allLines[i] = allLines[i][:leftPad-2] + bar + allLines[i][leftPad:]
			}
		}
	} else {
		// Draw indicator bar on every content line to the right of the bubble.
		if selected && ml.showIndicator && len(allLines) > 2 {
			bubbleW := lipgloss.Width(allLines[0])
			available := ml.viewWidth - bubbleW
			if available >= 2 {
				bar := " " + indicatorStyle.Render(indicatorChar)
				for i := 1; i < len(allLines)-1; i++ {
					allLines[i] = allLines[i] + bar
				}
			}
		}
	}

	return allLines
}

func (ml *MessageList) renderSeparator(label string) []string {
	labelW := lipgloss.Width(label)
	fill := (ml.viewWidth - labelW - 2) / 2
	if fill < 0 {
		fill = 0
	}
	rightFill := ml.viewWidth - fill - 1 - labelW - 1
	if rightFill < 0 {
		rightFill = 0
	}
	line := sepStyle.Render(strings.Repeat("─", fill)) + " " + label + " " + sepStyle.Render(strings.Repeat("─", rightFill))
	return []string{"", line, ""}
}

func (ml *MessageList) renderUnreadSeparator() []string {
	const label = "New Messages"
	labelW := lipgloss.Width(label)
	fill := (ml.viewWidth - labelW - 2) / 2
	if fill < 0 {
		fill = 0
	}
	rightFill := ml.viewWidth - fill - 1 - labelW - 1
	if rightFill < 0 {
		rightFill = 0
	}
	line := unreadSepStyle.Render(strings.Repeat("─", fill)) + " " + unreadSepStyle.Render(label) + " " + unreadSepStyle.Render(strings.Repeat("─", rightFill))
	return []string{"", line, ""}
}

func (ml *MessageList) renderItem(i int, selected bool) []string {
	item := ml.items[i]
	if item.kind == itemDateSeparator {
		return ml.renderSeparator(item.label)
	}
	if item.kind == itemUnreadSeparator {
		return ml.renderUnreadSeparator()
	}
	return ml.renderMessage(item.msg, selected)
}

func (ml *MessageList) View() string {
	ml.selRectOK = false
	if ml.viewWidth <= 0 || ml.viewHeight <= 0 {
		return ""
	}
	if len(ml.items) == 0 {
		return strings.Repeat("\n", ml.viewHeight-1)
	}

	selectedID := ml.computeSelectedMsgID()

	var allLines []string
	reachedEnd := true
	selTopRaw, selHeight, selLeft, selWidth := 0, 0, 0, 0
	for i := ml.viewStart; i < len(ml.items); i++ {
		var selected bool
		if ml.items[i].kind == itemMessage {
			selected = ml.items[i].msg.ID == selectedID
		}
		itemLines := ml.renderItem(i, selected)

		// Measure alignment from the top border line (index 0); it never carries
		// the selection indicator, and every line of a bubble shares the same
		// left padding, so this yields the bubble's left/width reliably.
		var selFirstFull string
		if selected && len(itemLines) > 0 {
			selFirstFull = itemLines[0]
		}

		if i == ml.viewStart && ml.lineOffset > 0 {
			if ml.lineOffset < len(itemLines) {
				itemLines = itemLines[ml.lineOffset:]
			} else {
				itemLines = nil
			}
		}

		if selected {
			selTopRaw = len(allLines)
			selHeight = len(itemLines)
			trimmed := strings.TrimLeft(selFirstFull, " ")
			selLeft = lipgloss.Width(selFirstFull) - lipgloss.Width(trimmed)
			selWidth = lipgloss.Width(trimmed)
			ml.selRectOK = true
		}

		allLines = append(allLines, itemLines...)
		if len(allLines) >= ml.viewHeight {
			reachedEnd = (i == len(ml.items)-1)
			break
		}
	}

	// delta tracks how the pad/trim step below shifts every line's index, so the
	// captured selTopRaw can be mapped to its final viewport row.
	delta := 0

	// Pad to viewHeight.
	// If we rendered all the way to the last message, anchor content to the bottom
	// (chat-like: newest messages visible). Otherwise we're in the middle of history,
	// so anchor to the top so the jump target is immediately visible.
	if len(allLines) < ml.viewHeight {
		padding := make([]string, ml.viewHeight-len(allLines))
		if reachedEnd {
			allLines = append(padding, allLines...)
			delta = len(padding)
		} else {
			allLines = append(allLines, padding...)
		}
	}

	// Trim to viewport height.
	// At the natural bottom of the chat, trim from the top so the newest content
	// stays visible. When scrolling through history, trim from the bottom so the
	// current scroll position is preserved.
	if len(allLines) > ml.viewHeight {
		botIdx, botOff := ml.positionAtBottom()
		atNaturalBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff
		if reachedEnd && atNaturalBottom {
			cut := len(allLines) - ml.viewHeight
			allLines = allLines[cut:]
			delta = -cut
		} else {
			allLines = allLines[:ml.viewHeight]
		}
	}

	if ml.selRectOK {
		ml.selRect = Rect{Top: selTopRaw + delta, Left: selLeft, Height: selHeight, Width: selWidth}
	}

	return strings.Join(allLines, "\n")
}
