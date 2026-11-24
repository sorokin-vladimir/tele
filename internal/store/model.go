package store

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

type EventKind int

const (
	EventNewMessage EventKind = iota
	EventReadInbox
	EventReadOutbox
	EventReactionsUpdate
	EventDeleteMessages
	EventUserPresence
	EventTyping
	// EventMuteUpdate reports that a chat's mute state changed server-side
	// (e.g. muted/unmuted from another device).
	EventMuteUpdate
	// EventEditMessage reports that a message was edited on another client.
	// The updated message is carried in Event.Message.
	EventEditMessage
	// EventDraftMessage reports that a chat's draft changed server-side (e.g.
	// edited on another device, or cleared on send). The text is in Event.Draft.
	EventDraftMessage
	// EventChannelGap reports that Telegram refused to say what a channel
	// missed: the stored position is too far behind to be caught up from, so a
	// range of messages will never arrive through the update stream. The
	// channel is Event.ChatID.
	EventChannelGap
	// EventGapScan reports the same thing about the account's own state, which
	// covers every chat that is not a channel and names none of them. What is
	// missing can only be found by asking Telegram where each chat now ends.
	EventGapScan
	// EventTopicsChanged reports that something about a forum's topics changed
	// other than a message arriving: a topic opened, edited or read, or what is
	// pinned (#275). It is not applied but answered, by reading the topics
	// named in Event.MsgIDs again, or the first page when Event.TopicsPage is
	// set, since only that page states the order topics are pinned in.
	EventTopicsChanged
)

type Event struct {
	Kind      EventKind
	Message   domain.Message
	ChatID    int64
	ReadMaxID int
	MsgID     int
	MsgIDs    []int
	Reactions []domain.Reaction
	// ReactionsUnread reports whether an EventReactionsUpdate carries unread
	// recent reactions. nil means the update carried no recent reactions list
	// (e.g. min set or chat where recent list is not shown), leaving the unread
	// state unchanged.
	ReactionsUnread *bool
	// ReactionEmoji is the emoji of the newest unread reaction (for notifications);
	// empty for a custom-emoji reaction. ReactionDate is when that reaction was
	// added, used to suppress stale catch-up reactions after an idle reconnect.
	ReactionEmoji string
	ReactionDate  time.Time
	Online        bool
	TypingAction  domain.TypingAction
	Muted         bool
	// Draft carries the new draft text for EventDraftMessage.
	Draft string
	// TopicsPage asks for a forum's first page of topics to be read again, for
	// EventTopicsChanged.
	TopicsPage bool
	// TopicID names the forum topic a draft or typing event is about, 0 when it
	// is about the chat itself (#275).
	TopicID int
}
