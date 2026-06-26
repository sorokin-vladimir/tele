package components

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/store"
)

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

func (ml *MessageList) SetMessages(msgs []store.Message) {
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
func (ml *MessageList) SetMessagesKeepScroll(msgs []store.Message) {
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

func (ml *MessageList) findMessage(id int) *store.Message {
	for i := range ml.items {
		if ml.items[i].kind == itemMessage && ml.items[i].msg.ID == id {
			return &ml.items[i].msg
		}
	}
	return nil
}
