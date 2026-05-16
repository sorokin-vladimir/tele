package store

type Store interface {
	GetChat(id int64) (Chat, bool)
	SetChat(chat Chat)
	Chats() []Chat
	Messages(chatID int64) []Message
	SetMessages(chatID int64, msgs []Message)
	AppendMessage(msg Message)
	UpdateMessageID(chatID int64, oldID, newID int)
	RemoveMessage(chatID int64, msgID int)
	UpdateChatReadMaxID(chatID int64, maxID int)
}
