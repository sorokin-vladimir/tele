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
	RemoveMessage(chatID int64, msgID int)
	RemoveMessages(chatID int64, msgIDs []int)
	UpdateChatReadMaxID(chatID int64, maxID int)
	UpdateChatOutboxReadMaxID(chatID int64, maxID int)
}
