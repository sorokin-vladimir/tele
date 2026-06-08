package store

import "time"

type Store interface {
	GetChat(id int64) (Chat, bool)
	SetChat(chat Chat)
	Chats() []Chat
	Messages(chatID int64) []Message
	SetMessages(chatID int64, msgs []Message)
	AppendMessage(msg Message)
	UpdateMessageID(chatID int64, oldID, newID int)
	UpdateMessageText(chatID int64, msgID int, text string, editDate time.Time)
	UpdateMessageReactions(chatID int64, msgID int, reactions []Reaction)
	UpdateMessageMedia(chatID int64, msgID int, photo *PhotoRef, document *DocumentRef)
	RemoveMessage(chatID int64, msgID int)
	RemoveMessages(chatID int64, msgIDs []int)
	IncrementChatUnread(chatID int64)
	UpdateChatReadMaxID(chatID int64, maxID int)
	UpdateChatOutboxReadMaxID(chatID int64, maxID int)
	UpdateChatOnline(userID int64, online bool)
	FolderFilters() []FolderFilter
	SetFolderFilters(filters []FolderFilter)
	ClearForNewAccount(ownerID int64)
}
