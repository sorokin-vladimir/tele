package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// ChatContents is everything a chat subscription currently shows: the message
// window plus the header and per-chat state the pane renders around it.
type ChatContents struct {
	ChatID int64
	Title  string
	IsUser bool
	// IsGroup covers groups and channels: the message list shows sender names
	// there and not in a 1:1 chat.
	IsGroup         bool
	Online          bool
	Messages        []domain.Message
	AnchorMsgID     int
	HasOlder        bool
	HasNewer        bool
	ReadInboxMaxID  int
	ReadOutboxMaxID int
	// UnreadReactions is the chat's unread-reaction count. It is per-chat state,
	// not per-message: a reaction can land on a message far outside the window,
	// and the client still has to mark it read while the user is looking.
	UnreadReactions int
	// UnreadMentions is the chat's unread-mention count, per-chat for the same
	// reason: opening the chat reads them, wherever they landed.
	UnreadMentions int
	Draft          string
	// Outbox is this chat's queued sends, oldest first. It is a separate list
	// rather than synthetic messages because an entry has no message ID and
	// carries what a message does not: attempts, an error kind, a retry time.
	// A pending send is by definition newer than the window, so a client
	// appends rather than merges (#193).
	Outbox []domain.OutboxEntry
}

// BuildChat resolves the window's anchor against the stored history and slices
// out Before..After around it.
//
// HasOlder and HasNewer report what the store holds outside the window; they
// drive the client's scroll affordance and say nothing about Telegram. It never
// fetches: a window that comes back shorter than it asked for is how the core
// learns the store fell short (see Owner.needsBackfill).
func BuildChat(r Reader, w ChatWindow) ChatContents {
	out := ChatContents{ChatID: w.ChatID}
	chat, ok := r.GetChat(w.ChatID)
	if ok {
		out.Title = chat.Title
		out.IsUser = chat.Peer.IsUser()
		out.IsGroup = chat.Peer.IsGroup() || chat.Peer.IsChannel()
		out.Online = chat.Online
		out.ReadInboxMaxID = chat.ReadInboxMaxID
		out.ReadOutboxMaxID = chat.ReadOutboxMaxID
		out.UnreadReactions = chat.UnreadReactionsCount
		out.UnreadMentions = chat.UnreadMentionsCount
		out.Draft = chat.Draft
	}
	// Read before the empty-history return below: a chat with nothing stored can
	// still hold a queued send, and that is the only thing it has to show.
	out.Outbox = r.Outbox(w.ChatID)

	all := r.Messages(w.ChatID)
	if len(all) == 0 {
		return out
	}

	idx, anchorID := resolveAnchor(all, chat, w.Anchor)
	out.AnchorMsgID = anchorID
	if idx < 0 {
		// The anchor names a message the store does not hold, so the window
		// cannot be sliced at all. HasOlder stays false: it reports what the
		// store holds outside the window, and the store holds nothing usable
		// here. The shortfall is what tells the core to fetch.
		return out
	}

	start := idx - w.Before
	if start < 0 {
		start = 0
	}
	end := idx + w.After + 1
	if w.Anchor.Kind == AnchorFirstUnread {
		// Anchored on the first unread, the window runs to the newest message:
		// everything past the anchor is unread by definition, and a window that
		// stopped at the separator would hide exactly what it was opened for.
		end = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	out.Messages = all[start:end]
	out.HasOlder = start > 0
	out.HasNewer = end < len(all)
	return out
}

// resolveAnchor returns the index of the anchor message in all, oldest first,
// and its id. An index of -1 means the anchor names a message the store does not
// hold.
func resolveAnchor(all []domain.Message, chat domain.Chat, a Anchor) (int, int) {
	switch a.Kind {
	case AnchorMessage:
		for i, m := range all {
			if m.ID == a.MsgID {
				return i, m.ID
			}
		}
		return -1, a.MsgID

	case AnchorFirstUnread:
		// Once pinned by the registry the window stops following the read
		// pointer. A pin that no longer names a stored message (deleted, or
		// trimmed by the store cap) resolves afresh rather than emptying.
		if a.MsgID != 0 {
			for i, m := range all {
				if m.ID == a.MsgID {
					return i, m.ID
				}
			}
		}
		if chat.UnreadCount > 0 {
			for i, m := range all {
				if m.ID > chat.ReadInboxMaxID {
					return i, m.ID
				}
			}
		}
		// No unread, or the read pointer is past everything stored: the anchor
		// is the newest message.
		fallthrough

	default: // AnchorNewest
		last := len(all) - 1
		return last, all[last].ID
	}
}
