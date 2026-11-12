package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// HistoryContents is everything a chat subscription currently shows: the message
// window plus the header and per-chat state the pane renders around it.
type HistoryContents struct {
	ChatID int64
	// TopicID and TopicTitle name the forum topic the window shows, when it
	// shows one; TopicTitle is empty for a topic nothing was learned about yet.
	TopicID    int
	TopicTitle string
	Title      string
	IsUser     bool
	// IsGroup covers groups and channels: the message list shows sender names
	// there and not in a 1:1 chat.
	IsGroup     bool
	Online      bool
	Messages    []domain.Message
	AnchorMsgID int
	// HasOlder and HasNewer report what the window's run holds outside the
	// window. A window never reaches into another run: what lies between two
	// runs was never fetched.
	HasOlder bool
	HasNewer bool
	// AtTail reports that the window's run ends at the history's newest
	// message, and AtBeginning that it reaches the oldest one there is.
	AtTail      bool
	AtBeginning bool
	// NewCount is how many messages landed at the tail since the window left
	// it, and zero while the window's run is the tail run.
	NewCount        int
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

// BuildHistory resolves the window's anchor against the stored history and slices
// out Before..After around it.
//
// HasOlder and HasNewer report what the store holds outside the window; they
// drive the client's scroll affordance and say nothing about Telegram. It never
// fetches: a window that comes back shorter than it asked for is how the core
// learns the store fell short (see Owner.needsBackfill).
func BuildHistory(r Reader, w HistoryWindow) HistoryContents {
	out := HistoryContents{ChatID: w.ChatID, TopicID: w.TopicID}
	chat, ok := r.GetChat(w.ChatID)
	if ok {
		out.Title = chat.Title
		out.IsUser = chat.Peer.IsUser()
		out.IsGroup = chat.Peer.IsGroup() || chat.Peer.IsChannel()
		out.Online = chat.Online
	}
	// What is read, unread and half-written belongs to the history: a topic
	// has its own, and the forum's say nothing about it.
	read := readState{
		unread:     chat.UnreadCount,
		readInbox:  chat.ReadInboxMaxID,
		readOutbox: chat.ReadOutboxMaxID,
		mentions:   chat.UnreadMentionsCount,
		reactions:  chat.UnreadReactionsCount,
		draft:      chat.Draft,
	}
	if w.TopicID != 0 {
		topic, _ := r.Topic(w.ChatID, w.TopicID)
		out.TopicTitle = topic.Title
		read = readState{
			unread:     topic.UnreadCount,
			readInbox:  topic.ReadInboxMaxID,
			readOutbox: topic.ReadOutboxMaxID,
			mentions:   topic.UnreadMentionsCount,
			reactions:  topic.UnreadReactionsCount,
			draft:      topic.Draft,
		}
	}
	out.ReadInboxMaxID = read.readInbox
	out.ReadOutboxMaxID = read.readOutbox
	out.UnreadReactions = read.reactions
	out.UnreadMentions = read.mentions
	out.Draft = read.draft
	// Read before the empty-history return below: a chat with nothing stored can
	// still hold a queued send, and that is the only thing it has to show.
	out.Outbox = r.Outbox(w.History())

	runs := r.Runs(w.History())
	run, idx, anchorID := resolveAnchor(runs, read, w.Anchor)
	out.AnchorMsgID = anchorID
	if idx < 0 {
		// The anchor names a message the store does not hold, or the history
		// holds nothing to anchor on, so the window cannot be sliced at all.
		// HasOlder stays false: it reports what the store holds outside the
		// window, and the store holds nothing usable here. The shortfall is
		// what tells the core to fetch.
		return out
	}
	out.AtTail = run.AtTail
	out.AtBeginning = run.AtBeginning
	if !run.AtTail {
		out.NewCount = newSince(runs, w.TailMark)
	}
	all := run.Messages

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

// readState is what a history knows about being read and written to: the
// chat's for an ordinary chat, the topic's for a topic.
type readState struct {
	unread     int
	readInbox  int
	readOutbox int
	mentions   int
	reactions  int
	draft      string
}

// resolveAnchor finds the anchor among a history's runs: the run holding it,
// its index in that run's messages, oldest first, and its id. An index of -1
// means the store holds no message to anchor on: the anchor names one it does
// not hold, or the history has no tail run for a window anchored at the tail.
func resolveAnchor(runs []domain.Run, read readState, a Anchor) (domain.Run, int, int) {
	switch a.Kind {
	case AnchorMessage:
		if run, i, ok := findMessage(runs, a.MsgID); ok {
			return run, i, a.MsgID
		}
		return domain.Run{}, -1, a.MsgID

	case AnchorFirstUnread:
		// Once pinned by the registry the window stops following the read
		// pointer. A pin that no longer names a stored message (deleted, or
		// trimmed by the store cap) resolves afresh rather than emptying.
		if a.MsgID != 0 {
			if run, i, ok := findMessage(runs, a.MsgID); ok {
				return run, i, a.MsgID
			}
		}
		// Unread messages are the newest ones, so the first of them is looked
		// for on the tail run.
		if tail, ok := tailRun(runs); ok && read.unread > 0 {
			for i, m := range tail.Messages {
				if m.ID > read.readInbox {
					return tail, i, m.ID
				}
			}
		}
		// No unread, or the read pointer is past everything stored: the anchor
		// is the newest message.
		fallthrough

	default: // AnchorNewest
		tail, ok := tailRun(runs)
		if !ok {
			return domain.Run{}, -1, 0
		}
		last := len(tail.Messages) - 1
		return tail, last, tail.Messages[last].ID
	}
}

// findMessage finds a message among runs.
func findMessage(runs []domain.Run, id int) (domain.Run, int, bool) {
	for _, r := range runs {
		for i, m := range r.Messages {
			if m.ID == id {
				return r, i, true
			}
		}
	}
	return domain.Run{}, -1, false
}

// tailRun is the run live messages land on, when the history holds one.
func tailRun(runs []domain.Run) (domain.Run, bool) {
	for _, r := range runs {
		if r.AtTail && len(r.Messages) > 0 {
			return r, true
		}
	}
	return domain.Run{}, false
}

// tailNewest is the id of the tail run's newest message, zero with no tail run.
func tailNewest(runs []domain.Run) int {
	tail, ok := tailRun(runs)
	if !ok {
		return 0
	}
	return tail.Messages[len(tail.Messages)-1].ID
}

// newSince counts the messages of the tail run newer than mark: what landed at
// the tail since a window left it. A window with no mark yet counts nothing.
func newSince(runs []domain.Run, mark int) int {
	tail, ok := tailRun(runs)
	if !ok || mark == 0 {
		return 0
	}
	n := 0
	for _, m := range tail.Messages {
		if m.ID > mark {
			n++
		}
	}
	return n
}
