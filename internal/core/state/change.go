package state

import "github.com/sorokin-vladimir/tele/internal/domain"

// ChangeKind names what happened to domain state. It is deliberately not
// store.EventKind: an event means "Telegram told us X", a Change means "state
// now differs in way Y". They diverge — an incoming message already covered by
// the read pointer is an event that changes no counter.
type ChangeKind int

const (
	ChangeNewMessage ChangeKind = iota
	ChangeReadInbox
	ChangeReadOutbox
	ChangeMessageEdited
	// ChangeMessageReactions covers both the explicit reactions update and the
	// hidden edit Telegram uses to deliver 1:1 peer reactions. They were two
	// branches with identical handling; the distinction was never meaningful to
	// a client.
	ChangeMessageReactions
	ChangeDraft
	ChangeMessagesDeleted
	ChangePresence
	ChangeMute
	// ChangeTyping is ephemeral: it has no persisted state and is published
	// straight through. In the projection model the typing label belongs to
	// chat:<id> alongside the message window and the draft.
	ChangeTyping
	// ChangeHistory reports that a chat's stored history was replaced by a
	// fetched page. It is how a backfill re-enters state so the chat:<id>
	// projection rebuilds from the same place every other change does.
	ChangeHistory
	// ChangeArchived and ChangeUnreadMark are per-chat flags a client toggles
	// and another device can change; both restyle or reorder a chat-list row.
	ChangeArchived
	ChangeUnreadMark
	// ChangeFolders reports that folder membership changed, which re-filters
	// every chat-list window bound to a folder.
	ChangeFolders
	// ChangeReactionsRead and ChangeMentionsRead clear the per-chat badges that
	// sit next to the unread count.
	ChangeReactionsRead
	ChangeMentionsRead
	// ChangeMessageRestored undoes a delete Telegram refused. It is not
	// ChangeNewMessage: a restored message is not new and must move no counter
	// and raise no notification.
	ChangeMessageRestored
	// ChangeMediaRef reports that a message's media file reference was replaced
	// after Telegram expired the old one. Nothing the user can see changes; the
	// chat window rebuilds so the next fetch addresses the fresh reference.
	ChangeMediaRef
	// ChangeTopics reports that what is known about a forum's topics changed:
	// a page or a topic was read from Telegram, or a topic was deleted (#275).
	ChangeTopics
	// ChangeChatsAdded reports that chats the store had no row for were read
	// from Telegram, which adds rows to every chat-list window.
	ChangeChatsAdded
)

// Change describes one applied difference in domain state, carrying what a
// client needs to react to it.
type Change struct {
	Kind    ChangeKind
	ChatID  int64
	Message domain.Message
	MsgID   int
	MsgIDs  []int
	Draft   string
	Online  bool
	Muted   bool
	Typing  domain.TypingAction
	// UnreadChanged reports that this change moved a chat's unread or mention
	// count, so views derived from unread (the folder bar) need recomputing.
	UnreadChanged bool
	// ReactionsUnread reports that the reactions carry at least one unread
	// entry; UnreadReactionChanged reports that the per-chat unread-reaction
	// count actually moved as a result.
	ReactionsUnread       bool
	UnreadReactionChanged bool
	// TopicID names the forum topic a draft or typing change is about, 0 when
	// it is about a chat that is no forum.
	TopicID int
	// UnknownTopic names the forum topic a new message landed in when nothing
	// about that topic was known yet, so the owner can ask Telegram (#275).
	UnknownTopic int
}
