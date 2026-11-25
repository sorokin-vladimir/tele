// Package project turns domain state into the windowed, view-shaped data a
// client subscribes to. Builders are pure functions of (reader, window), and
// deltas are computed by diffing successive contents rather than by mapping
// change kinds, so a change no window contains costs nothing.
package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// SubID identifies one live subscription.
type SubID int

// Window is what a subscription looks at. The implementations are
// ChatListWindow and HistoryWindow.
type Window interface{ isWindow() }

// ChatListWindow is a slice of the ordered, folder-filtered chat list. Folder 0
// is All Chats; domain.ArchiveFolderID is the Archive virtual folder.
type ChatListWindow struct {
	Folder int
	Offset int
	Limit  int
}

func (ChatListWindow) isWindow() {}

// TopicListWindow is a slice of one forum's topics, in the order the official
// clients list them (#275).
type TopicListWindow struct {
	ChatID int64
	Offset int
	Limit  int
}

func (TopicListWindow) isWindow() {}

// AnchorKind names what a chat window is positioned on. Every hard case in the
// UI is one of these three: opening a chat (Newest), opening one with unread
// (FirstUnread, which needs context above the separator), and jumping to a
// quoted message (Message, which needs context on both sides).
type AnchorKind int

const (
	AnchorNewest AnchorKind = iota
	AnchorFirstUnread
	AnchorMessage
)

type Anchor struct {
	Kind AnchorKind
	// MsgID names the anchor message. The client sets it for AnchorMessage; for
	// AnchorFirstUnread the registry fills it in once the window has resolved,
	// so the window holds still while the read pointer moves.
	MsgID int
}

// HistoryWindow is a range of one history around an anchor. Before and
// After rather than a single limit, because the anchor is not always at an edge.
// They count messages either side of the anchor, which is always carried on top
// of them: a window is at most Before+After+1 messages.
type HistoryWindow struct {
	ChatID int64
	// TopicID names the forum topic whose history this is, and is 0 for an
	// ordinary chat's one history. General is domain.GeneralTopicID (#275).
	TopicID int
	Anchor  Anchor
	Before  int
	After   int
	// TailMark is the newest message of the tail run at the moment the window
	// left it, which is what "N new" counts from. The registry sets it; a
	// client leaves it zero.
	TailMark int
}

// History names the history the window is a range of.
func (w HistoryWindow) History() domain.HistoryKey {
	return domain.HistoryKey{ChatID: w.ChatID, TopicID: w.TopicID}
}

func (HistoryWindow) isWindow() {}
