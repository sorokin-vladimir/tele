package store

import "time"

type Store interface {
	GetChat(id int64) (Chat, bool)
	SetChat(chat Chat)
	Chats() []Chat
	Messages(chatID int64) []Message
	SetMessages(chatID int64, msgs []Message)
	AppendMessage(msg Message)
	// BumpChatLastMessage updates a chat's last-message preview and ordering
	// without appending to its message slice (e.g. a forward target).
	BumpChatLastMessage(chatID int64, msg Message)
	UpdateMessageID(chatID int64, oldID, newID int)
	UpdateMessageText(chatID int64, msgID int, text string, editDate time.Time)
	UpdateMessageReactions(chatID int64, msgID int, reactions []Reaction)
	UpdateMessageMedia(chatID int64, msgID int, photo *PhotoRef, document *DocumentRef)
	// UpdateLocalMediaProgress sets the upload fraction (0..1) on an optimistic
	// outgoing message's LocalMedia, located by its (negative) sentinel ID.
	UpdateLocalMediaProgress(sentinelID int, frac float64)
	// MarkLocalMediaFailed marks an optimistic message's upload as failed.
	MarkLocalMediaFailed(sentinelID int)
	// ClearLocalMedia drops the LocalMedia from a message once the upload is
	// confirmed (the bubble then renders from the server-confirmed media).
	ClearLocalMedia(sentinelID int)
	// AdoptServerMedia replaces a just-sent message's media refs with the ones the
	// server assigned (fetched via RefreshMessage) and clears LocalMedia, so the
	// outgoing photo renders inline without waiting for a manual refresh.
	AdoptServerMedia(chatID int64, msgID int, photo *PhotoRef, doc *DocumentRef, media *MediaRef)
	RemoveMessage(chatID int64, msgID int)
	RemoveMessages(chatID int64, msgIDs []int)
	RemoveMessagesByID(msgIDs []int) (affected []int64)
	IncrementChatUnread(chatID int64)
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
	FolderFilters() []FolderFilter
	SetFolderFilters(filters []FolderFilter)
	ClearForNewAccount(ownerID int64)
}
