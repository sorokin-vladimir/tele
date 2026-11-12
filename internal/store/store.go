package store

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

type Store interface {
	GetChat(id int64) (domain.Chat, bool)
	SetChat(chat domain.Chat)
	Chats() []domain.Chat
	// Messages returns one history's held tail, oldest first. An ordinary chat's
	// history is keyed by the chat alone; a forum has one per topic (#275).
	Messages(h domain.HistoryKey) []domain.Message
	// Message returns one held message by the chat and id Telegram addresses
	// it by, whichever of the chat's histories holds it.
	Message(chatID int64, msgID int) (domain.Message, bool)
	// SetMessages replaces everything a chat holds with a page, laid out among
	// its histories: in a forum, every topic at once.
	SetMessages(chatID int64, msgs []domain.Message)
	// Runs returns a history's runs, oldest first: the unbroken stretches it
	// is stored as, each with the messages it holds.
	Runs(h domain.HistoryKey) []domain.Run
	// MergeAround, MergeOlder and MergeNewer each merge a page fetched from
	// Telegram under a single hold of the store's lock: SetMessages after a
	// read outside the lock would drop anything that arrived in between. They
	// differ in where the page lies against what is held.
	//
	// MergeAround stores a page fetched around one message as a run of its
	// own, joined to whichever runs it meets, and reports how many messages it
	// added.
	MergeAround(h domain.HistoryKey, msgs []domain.Message) int
	// MergeOlder stores a page fetched older than message below, growing the
	// run that holds below; zero for below means the newest page, which ends
	// at the tail. A page with nothing older than below marks the run as
	// reaching the beginning. It reports how many messages it added.
	MergeOlder(h domain.HistoryKey, below int, msgs []domain.Message) int
	// MergeNewer stores a page fetched newer than message above, growing the
	// run that holds above. A page with nothing newer than above marks the
	// run as ending at the tail. It reports how many messages it added.
	MergeNewer(h domain.HistoryKey, above int, msgs []domain.Message) int
	// RepairMessages merges a page fetched to close a gap, laid out among the
	// chat's histories on their tail runs. It takes a chat rather than a
	// history because a gap is the chat's.
	RepairMessages(chatID int64, msgs []domain.Message) int
	// SetShown records what windows show of each history, replacing what was
	// recorded before. The cap never drops what is shown, so a scrollback
	// somebody loaded is held while somebody looks at it (ADR 0037).
	SetShown(shown map[domain.HistoryKey][]Shown)
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
	// LoadMessages loads a history's persisted tail into memory on first open
	// (idempotent per history). See issue #139.
	LoadMessages(h domain.HistoryKey)
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
	// UpdateMessageReactions replaces a message's reaction set. via names what
	// the set came from, for the reaction trace only (#248): an edit, a
	// reactions update and our own optimistic write all land here.
	UpdateMessageReactions(chatID int64, msgID int, reactions []domain.Reaction, via string)
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
	// Topics returns a forum's known topics, pinned first in pin order, then by
	// newest message (#275).
	Topics(chatID int64) []domain.Topic
	Topic(chatID int64, topicID int) (domain.Topic, bool)
	// SetTopicsPage records the first page of a forum's topics, the only
	// statement of the order they were pinned in.
	SetTopicsPage(chatID int64, topics []domain.Topic)
	// UpdateTopics records topics read again by id, each keeping its pin place.
	UpdateTopics(chatID int64, topics []domain.Topic)
	// RemoveTopics forgets topics Telegram reported deleted.
	RemoveTopics(chatID int64, ids []int)
	// SetTopicRead moves a forum topic's read pointer up to maxID.
	SetTopicRead(chatID int64, topicID, maxID int)
	// SetTopicMentionsRead and SetTopicReactionsRead clear a forum topic's
	// unread mentions and reactions.
	SetTopicMentionsRead(chatID int64, topicID int)
	SetTopicReactionsRead(chatID int64, topicID int)
	// SetTopicMute records a forum topic's own notification setting and
	// reports the one it replaced.
	SetTopicMute(chatID int64, topicID int, mute domain.TopicMute) domain.TopicMute
	// SetTopicDraft records a forum topic's unsent draft.
	SetTopicDraft(chatID int64, topicID int, text string)
	// ApplyIncomingTopic records what an arriving message means for its topic:
	// its newest message and its unread count. known reports whether the topic
	// was described before, so a caller can ask Telegram about one that was not.
	ApplyIncomingTopic(msg domain.Message) (h domain.HistoryKey, known bool)
	FolderFilters() []domain.FolderFilter
	SetFolderFilters(filters []domain.FolderFilter)
	// Address returns how to reach a chat the account holds no dialog for: a
	// person found by search, or one whose profile was opened from a group.
	// A chat with a dialog is addressed by its own row, which wins over this.
	Address(chatID int64) (domain.Peer, bool)
	// RememberAddress keeps an address Telegram handed out, so that a send to
	// such a chat still resolves after a restart (#278).
	RememberAddress(p domain.Peer)
	SetOwnerID(ownerID int64)
}
