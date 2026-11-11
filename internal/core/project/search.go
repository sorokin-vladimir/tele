package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// SearchScope is where a message search looks: one chat, one topic of a forum
// when TopicID is set too, or every chat of the main list when ChatID is zero.
type SearchScope struct {
	ChatID  int64
	TopicID int
}

// FoundMessage is one message a search found, with the title of the chat it
// was found in: a search of every chat finds messages in chats the client may
// hold nothing of.
type FoundMessage struct {
	Message domain.Message
	Chat    string
}
