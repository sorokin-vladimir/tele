package components

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

type itemKind int

const (
	itemMessage         itemKind = iota
	itemDateSeparator            // date separator, 3 lines: blank + label + blank
	itemUnreadSeparator          // "New Messages" divider
	itemOutbox                   // a queued send: no message id, its own status line
)

type listItem struct {
	kind  itemKind
	msg   domain.Message   // valid when kind == itemMessage; the album anchor (parts[0])
	parts []domain.Message // album parts when kind == itemMessage; len 1 for a lone message
	label string           // valid when kind == itemDateSeparator, e.g. "May 18"
	entry domain.OutboxEntry
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

// FormatDateLabel is the exported form of formatSepLabel, for callers outside the
// list (e.g. the photo modal's date label) that need the same "Today" / "Month
// Day" / "Month Day, Year" rendering the date separators use.
func FormatDateLabel(t time.Time) string {
	return formatSepLabel(t)
}

// groupParts coalesces contiguous messages that belong to the same Telegram
// album: an album is a run of messages sharing a non-zero GroupedID and the same
// sender. Non-album messages, and any run reduced to a single part, come back as
// a group of one. Order is preserved. Album parts arrive contiguous in the
// timeline, so a single linear scan is sufficient; the function is pure so it can
// be table-tested and re-run cheaply on every rebuild.
func groupParts(msgs []domain.Message) [][]domain.Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([][]domain.Message, 0, len(msgs))
	for i := 0; i < len(msgs); {
		j := i + 1
		if msgs[i].GroupedID != 0 {
			for j < len(msgs) &&
				msgs[j].GroupedID == msgs[i].GroupedID &&
				msgs[j].SenderID == msgs[i].SenderID {
				j++
			}
		}
		out = append(out, msgs[i:j])
		i = j
	}
	return out
}

func (ml *MessageList) buildItems(msgs []domain.Message) []listItem {
	groups := groupParts(msgs)
	items := make([]listItem, 0, len(groups)+4)
	var prev time.Time
	unreadInserted := false
	for _, g := range groups {
		anchor := g[0]
		if !sameDay(prev, anchor.Date) {
			items = append(items, listItem{kind: itemDateSeparator, label: formatSepLabel(anchor.Date)})
			prev = anchor.Date
		}
		// The divider marks the first incoming unread message; own outgoing
		// messages never anchor it, even though their ID exceeds inboxReadMaxID.
		if !unreadInserted && !anchor.IsOut && ml.inboxReadMaxID > 0 && anchor.ID > ml.inboxReadMaxID {
			items = append(items, listItem{kind: itemUnreadSeparator})
			unreadInserted = true
		}
		items = append(items, listItem{kind: itemMessage, msg: anchor, parts: g})
	}
	return append(items, ml.outboxItems(items)...)
}

// outboxItems renders the send queue as list items, minus the entries whose
// message is already in the window. They always sit at the end, so there is
// nothing to merge with the window.
//
// "At the end" no longer means "newest". A failed send does not hold its chat,
// so the sends composed after it go out and land in the window above it, while
// it stays here waiting on a decision (#224). Below therefore reads as "not in
// the conversation yet" rather than "most recent", which is the distinction that
// matters to someone looking at it.
//
// Dropping the delivered ones is what makes the swap unobservable. An entry and
// its message carry the same text, so a frame holding both draws it twice, one
// bubble taller, and shifts everything above it — then the next frame shifts it
// back. That frame is not hypothetical: one recompute yields ChatAppend and
// ChatOutbox as separate deltas, and the client consumes one per tea.Msg, so it
// always renders between them (#226). SentMsgIDs is the correlation, set
// between a successful request and the moment the entry goes.
//
// One landed id is enough, where the owner's own clearSentOutbox waits for all
// of an album's parts: it is deciding when the durable row may go, this is
// deciding what to draw, and an album whose parts arrive one apply at a time
// should fill in bubble by bubble rather than double the parts already there.
func (ml *MessageList) outboxItems(window []listItem) []listItem {
	if len(ml.outbox) == 0 {
		return nil
	}
	inWindow := ml.landedIDs(window)
	out := make([]listItem, 0, len(ml.outbox))
	for _, e := range ml.outbox {
		if id := deliveredID(e, inWindow); id != 0 {
			// The cursor was parked on this entry when it was submitted, so it
			// has to follow the send into the window — otherwise the selection
			// indicator blinks out for exactly the frames this guard covers.
			if ml.cursorOutboxRef == e.Ref {
				ml.setCursor(id, "")
			}
			continue
		}
		out = append(out, listItem{kind: itemOutbox, entry: e, msg: ml.outboxBubble(e)})
	}
	return out
}

// landedIDs collects the message ids the window holds, and only when some entry
// claims one: SentMsgIDs is empty for the whole life of an ordinary queued send,
// and the queue is rebuilt on every upload progress frame.
func (ml *MessageList) landedIDs(window []listItem) map[int]struct{} {
	claimed := false
	for _, e := range ml.outbox {
		if len(e.SentMsgIDs) > 0 {
			claimed = true
			break
		}
	}
	if !claimed {
		return nil
	}
	ids := make(map[int]struct{}, len(window))
	for _, it := range window {
		if it.kind != itemMessage {
			continue
		}
		for _, p := range it.parts {
			ids[p.ID] = struct{}{}
		}
	}
	return ids
}

// deliveredID returns the id of the first message this entry produced that is
// already in the window, or 0 when none is.
func deliveredID(e domain.OutboxEntry, inWindow map[int]struct{}) int {
	for _, id := range e.SentMsgIDs {
		if _, ok := inWindow[id]; ok {
			return id
		}
	}
	return 0
}

// outboxBubble is how a queued send is drawn: an ordinary outgoing bubble.
// Where the send has got to is a glyph in the bottom border, in the same slot
// the delivery ticks take once the message exists — so the whole lifecycle
// reads left to right in one place and the bubble's height never moves.
//
// Drawn as a message rather than as a widget of its own so the bubble and its
// measured height stay in lock-step by construction — the drift groupHeight
// needs a dedicated test to guard against.
func (ml *MessageList) outboxBubble(e domain.OutboxEntry) domain.Message {
	msg := domain.Message{
		ChatID: e.ChatID,
		Date:   e.CreatedAt,
		IsOut:  true,
	}
	if e.Message != nil {
		msg.Text = e.Message.Text
		msg.Entities = e.Message.Entities
		msg.ReplyToMsgID = e.Message.ReplyToMsgID
	}
	if e.Media != nil && len(e.Media.Parts) > 0 {
		msg.Text = e.Media.Caption
		msg.Entities = e.Media.Entities
		msg.ReplyToMsgID = e.Media.ReplyToMsgID
		msg.LocalMedia = localMediaFor(e, ml.uploadProgress[e.Ref])
	}
	return msg
}

// localMediaFor renders a queued media send as the bubble's view model: a group
// is named by its count, a lone file by its name. The upload fraction is an
// event the list was told about, not something the entry carries (#195).
func localMediaFor(e domain.OutboxEntry, p uploadProgress) *domain.LocalMedia {
	lm := &domain.LocalMedia{
		Kind:           e.Media.Parts[0].SendAs,
		Parts:          len(e.Media.Parts),
		Part:           p.part,
		UploadProgress: p.frac,
	}
	if len(e.Media.Parts) == 1 {
		lm.FileName = e.Media.Parts[0].Name
		lm.Size = e.Media.Parts[0].Size
	}
	return lm
}

// outboxStatusGlyph is what a queued send shows in the bottom border, in the
// slot the delivery ticks occupy once the message exists:
//
//	⋯     waiting its turn        ✓   delivered
//	↻ 4s  waiting out a backoff   ✓✓  read
//	↑     in flight
//	✕     not sent
//
// The backoff is the one state carrying a number: "retry in 2s" and "retry in
// 12m" are different news. It does not tick — it is redrawn when the queue
// changes state, not on a timer, because a per-second repaint of every pending
// bubble is not worth a countdown nobody is watching.
func outboxStatusGlyph(e domain.OutboxEntry) string {
	switch e.State {
	case domain.OutboxSending:
		return " " + theme.S().TickOutbox.Render("↑")
	case domain.OutboxFailed:
		return " " + theme.S().TickFailed.Render("✕")
	default:
		if d := time.Until(e.NextAttemptAt); d > 0 {
			return " " + theme.S().TickOutbox.Render("↻ "+d.Round(time.Second).String())
		}
		return " " + theme.S().TickOutbox.Render("⋯")
	}
}

// OutboxReason names a terminal failure in the taxonomy's terms rather than
// Telegram's: the reader is deciding what to do about it, not debugging a
// protocol. The bubble shows only a glyph; this is what the status bar says
// when the cursor rests on the entry (#193).
//
// It takes the entry rather than the kind because a refusal is only useful once
// it is specific. "Telegram would not accept this" leaves a person with nothing
// to do; "this file is not one Telegram accepts as a photo" and "this message is
// too long" lead to different actions (#224). The raw Telegram type stays in the
// entry's detail and in the log, for a report rather than for the screen.
func OutboxReason(e domain.OutboxEntry) string {
	if e.ErrKind == telerr.Rejected {
		return RejectionReason(e.ErrReason)
	}
	switch e.ErrKind {
	case telerr.Forbidden:
		return "not allowed in this chat"
	case telerr.PeerNotFound:
		return "chat is unreachable"
	case telerr.NotFound:
		return "chat no longer exists"
	default:
		return "unexpected error"
	}
}

// RejectionReason phrases one refusal. The fallback covers a reason this build
// has no phrase for, and an entry rejected by an older build, which recorded
// none: both are honest as "Telegram would not accept it", and neither is worth
// leaking a protocol constant over.
//
// Exported because the toast raised at the moment of the refusal and the status
// bar reminder that outlives it must say the same thing.
func RejectionReason(r telerr.Reason) string {
	switch r {
	case telerr.ReasonPhotoType:
		return "not a file Telegram accepts as a photo"
	case telerr.ReasonPhotoDimensions:
		return "image is too large or oddly shaped"
	case telerr.ReasonMediaUnreadable:
		return "Telegram could not read this file"
	case telerr.ReasonMediaUnsupported:
		return "attachment is not supported here"
	case telerr.ReasonTextEmpty:
		return "there was nothing to send"
	case telerr.ReasonTextTooLong:
		return "message is too long"
	case telerr.ReasonMarkupTooLong:
		return "formatting is too long"
	default:
		return "Telegram would not accept it"
	}
}

// SetOutbox replaces the pending sends drawn below the window. Only the tail is
// rebuilt, so an entry merely changing state does not re-seat the message list.
//
// A newly submitted send does scroll into view: you just typed it, and a bubble
// added below the fold with nothing but the scrollbar moving is how it reads as
// lost. A state change on an entry already drawn leaves the viewport alone.
func (ml *MessageList) SetOutbox(entries []domain.OutboxEntry) {
	added := hasNewRef(ml.outbox, entries)
	ml.outbox = entries
	ml.forgetGoneProgress()
	ml.redrawOutbox()
	if added {
		ml.viewStart, ml.lineOffset = ml.positionAtBottom()
		ml.setCursorNewest()
		return
	}
	// A cursor with nothing to sit on is placed; one that already rests
	// somewhere is left alone. Otherwise the first message ever sent to a chat
	// would be unselectable — there is no history to have parked a cursor on.
	if ml.cursorIndex() < 0 {
		ml.setCursorNewest()
	}
}

// SetUploadProgress records how far a queued media send has uploaded and
// redraws it. Nothing else moves: the entries are unchanged, only their bubbles.
//
// parts is accepted and ignored — the count comes from the entry, which is the
// authority — so that a caller is never tempted to invent one.
func (ml *MessageList) SetUploadProgress(ref string, part, _ int, frac float64) {
	if ml.uploadProgress == nil {
		ml.uploadProgress = make(map[string]uploadProgress)
	}
	ml.uploadProgress[ref] = uploadProgress{part: part, frac: frac}
	ml.redrawOutbox()
}

// redrawOutbox rebuilds the queue's items in place, leaving the window and the
// cursor where they are.
func (ml *MessageList) redrawOutbox() {
	kept := ml.items
	for len(kept) > 0 && kept[len(kept)-1].kind == itemOutbox {
		kept = kept[:len(kept)-1]
	}
	ml.items = append(kept, ml.outboxItems(kept)...)
	ml.invalidateHeights()
}

// forgetGoneProgress drops the fractions of sends that have left the queue, so
// the map cannot grow for the life of the process and a later entry cannot
// inherit a stale bar.
func (ml *MessageList) forgetGoneProgress() {
	if len(ml.uploadProgress) == 0 {
		return
	}
	live := make(map[string]struct{}, len(ml.outbox))
	for _, e := range ml.outbox {
		live[e.Ref] = struct{}{}
	}
	for ref := range ml.uploadProgress {
		if _, ok := live[ref]; !ok {
			delete(ml.uploadProgress, ref)
		}
	}
}

// hasNewRef reports whether next holds an entry prev did not.
func hasNewRef(prev, next []domain.OutboxEntry) bool {
	known := make(map[string]struct{}, len(prev))
	for _, e := range prev {
		known[e.Ref] = struct{}{}
	}
	for _, e := range next {
		if _, ok := known[e.Ref]; !ok {
			return true
		}
	}
	return false
}

// Outbox returns the pending sends currently drawn.
func (ml *MessageList) Outbox() []domain.OutboxEntry { return ml.outbox }

func (ml *MessageList) SetMessages(msgs []domain.Message) {
	ml.items = ml.buildItems(msgs)
	ml.invalidateHeights()
	ml.viewStart, ml.lineOffset = ml.positionAtBottom()
	ml.setCursorNewest()
}

// SetMessagesKeepScroll replaces the message list without resetting the scroll position.
// Use for in-place data updates (e.g. reactions, edits) where the message count is
// unchanged.
//
// An edit can change a message's line count. If the viewport was at the natural
// bottom, re-anchor to the new bottom so the newest content stays fully visible
// instead of being clipped by a now-stale offset (same fix as SetImage). When
// scrolled up in history, keep the top anchor so the position does not jump.
func (ml *MessageList) SetMessagesKeepScroll(msgs []domain.Message) {
	botIdx, botOff := ml.positionAtBottom()
	wasAtBottom := ml.viewStart == botIdx && ml.lineOffset >= botOff

	vs, lo := ml.viewStart, ml.lineOffset
	ml.items = ml.buildItems(msgs)
	ml.invalidateHeights()
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
	msgs := make([]domain.Message, 0, len(ml.items))
	for _, item := range ml.items {
		if item.kind != itemMessage {
			continue
		}
		// Reconstruct from every album part, not just the anchor, so removing one
		// part of an album keeps its siblings.
		for _, p := range item.parts {
			if p.ID == id {
				found = true
			} else {
				msgs = append(msgs, p)
			}
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
	ml.invalidateHeights()

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

	// If the cursor was on the removed message, fall back to the newest.
	if ml.cursorIndex() < 0 {
		ml.setCursorNewest()
	}
}

// PrependMessages inserts older messages at the front and shifts viewStart so
// that the currently-visible messages stay on screen. Messages whose IDs already
// exist in the list are skipped: rapid scroll-up can fire several identical
// "load older" requests before the first resolves, so the same chunk may arrive
// more than once. Without this guard the duplicates would stack into a repeating
// date-range "ring" that never advances toward older history (issue #120).
func (ml *MessageList) PrependMessages(older []domain.Message) {
	if len(older) == 0 {
		return
	}
	current := make([]domain.Message, 0, len(ml.items))
	existing := make(map[int]struct{}, len(ml.items))
	for _, item := range ml.items {
		if item.kind != itemMessage {
			continue
		}
		// Include every album part so the flat slice round-trips through buildItems
		// without losing non-anchor parts.
		for _, p := range item.parts {
			current = append(current, p)
			existing[p.ID] = struct{}{}
		}
	}
	fresh := make([]domain.Message, 0, len(older))
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
	ml.invalidateHeights()
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

func (ml *MessageList) findMessage(id int) *domain.Message {
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		for j := range ml.items[i].parts {
			if ml.items[i].parts[j].ID == id {
				return &ml.items[i].parts[j]
			}
		}
	}
	return nil
}
