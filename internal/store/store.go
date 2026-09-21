package store

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

type Store interface {
	GetChat(id int64) (domain.Chat, bool)
	SetChat(chat domain.Chat)
	Chats() []domain.Chat
	Messages(chatID int64) []domain.Message
	SetMessages(chatID int64, msgs []domain.Message)
	// MergeMessages merges a fetched page into a chat's stored history under a
	// single hold of the store's lock and reports how many messages it added.
	// It is how a page fetched from Telegram is applied: SetMessages after a
	// read outside the lock would drop anything that arrived in between.
	MergeMessages(chatID int64, msgs []domain.Message) int
	// RepairMessages merges a page the way MergeMessages does but leaves the
	// chat no deeper than it found it: a page nobody asked for must not raise
	// what the chat costs to hold.
	RepairMessages(chatID int64, msgs []domain.Message) int
	// Gap reports the message a chat's missing range opens after, if it has
	// one. A gap is recorded when it opens rather than noticed later: once
	// updates resume the tail moves past the hole and nothing can see it.
	Gap(chatID int64) (int, bool)
	// MarkGap records a missing range, keeping the earlier of the positions
	// when one is already recorded.
	MarkGap(chatID int64, afterMsgID int)
	// AdvanceGap moves an open gap from one position to a later one as a repair
	// covers ground, only while the record still says what the repair last saw,
	// and reports whether it did. A hole recorded underneath a running repair
	// must not be written over.
	AdvanceGap(chatID int64, from, to int) bool
	// CloseGap forgets a gap that still stands where the repair left it, and
	// reports whether it did.
	CloseGap(chatID int64, at int) bool
	// ClearGap forgets a chat's gap whatever it says, for the tail reload that
	// threw away the history it was a hole in.
	ClearGap(chatID int64)
	// TailMessageID is the id of the newest message held for a chat, answered
	// without loading the chat's history into memory.
	TailMessageID(chatID int64) int
	// LoadMessages loads a chat's persisted message tail into memory on first
	// open (idempotent per chat). See issue #139.
	LoadMessages(chatID int64)
	// AppendMessage stores an arriving message, replacing the stored copy when
	// the same id is already held. It reports whether the message was new, which
	// is how a second delivery of one arrival is told from a first.
	AppendMessage(msg domain.Message) bool
	// AdvanceAppliedPosition records that a change at this position has been
	// applied to a message, reporting false when the stored copy has already
	// applied that position or a later one. A zero position orders against
	// nothing: it passes and records nothing. So does an unknown message, which
	// has applied nothing at all (ADR 0016).
	AdvanceAppliedPosition(chatID int64, msgID int, position int) bool
	// BumpChatLastMessage updates a chat's last-message preview and ordering
	// without appending to its message slice (e.g. a forward target).
	BumpChatLastMessage(chatID int64, msg domain.Message)
	UpdateMessageText(chatID int64, msgID int, text string, entities []domain.MessageEntity)
	// MarkMessageEdited records when a message was last edited and whether
	// Telegram asked for the "edited" label to be hidden. It is separate from
	// the text write because a hidden edit changes the text without earning the
	// label, and a reaction bump earns neither (#269).
	MarkMessageEdited(chatID int64, msgID int, editDate time.Time, hidden bool)
	UpdateMessageReactions(chatID int64, msgID int, reactions []domain.Reaction)
	UpdateMessageMedia(chatID int64, msgID int, photo *domain.PhotoRef, document *domain.DocumentRef)
	// ReplaceMessage overwrites a stored message wholesale. It is how a refused
	// edit is undone: no field-wise update clears an edit marker, and a message
	// that was never edited must not keep one (#118).
	ReplaceMessage(chatID int64, msg domain.Message)
	RemoveMessage(chatID int64, msgID int)
	RemoveMessages(chatID int64, msgIDs []int)
	RemoveMessagesByID(msgIDs []int) (affected []int64)
	// ApplyUnreadMessage records an inbound message as unread for its chat.
	// Idempotent per message ID so a replayed update cannot inflate the count;
	// no-op for an unknown chat or a message at or below the read pointer.
	// Returns true when the count changed.
	ApplyUnreadMessage(chatID int64, msgID int) bool
	UpdateChatReadMaxID(chatID int64, maxID int) bool
	UpdateChatOutboxReadMaxID(chatID int64, maxID int)
	UpdateChatOnline(userID int64, online bool) bool
	SetChatMuted(chatID int64, muted bool)
	// SetChatDraft updates the synced message draft for a chat (#62). The draft
	// is held in memory only; it is reloaded from the server on restart.
	SetChatDraft(chatID int64, text string)
	SetChatUnreadMark(chatID int64, mark bool)
	SetChatArchived(chatID int64, archived bool)
	// ApplyUnreadReaction idempotently adjusts a chat's unread-reaction count
	// from a per-message signal, tracking which messages carry unread reactions.
	// Returns true when the count changed.
	ApplyUnreadReaction(chatID int64, msgID int, hasUnread bool) bool
	// SetChatReactionsRead clears a chat's unread-reaction count and its tracked
	// message set (e.g. on open or readReactions completion).
	SetChatReactionsRead(chatID int64)
	// ApplyUnreadMention idempotently adjusts a chat's unread-mention count from a
	// per-message signal, tracking which messages carry an unread mention.
	// Returns true when the count changed.
	ApplyUnreadMention(chatID int64, msgID int, hasMention bool) bool
	// SetChatMentionsRead clears a chat's unread-mention count and its tracked
	// message set (e.g. on open or readMentions completion).
	SetChatMentionsRead(chatID int64)
	FolderFilters() []domain.FolderFilter
	SetFolderFilters(filters []domain.FolderFilter)
	SetOwnerID(ownerID int64)
}
