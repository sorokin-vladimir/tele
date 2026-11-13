package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// SearchScope is where a message search looks: one chat, one topic of a forum
// when TopicID is set too, or every chat of the main list when ChatID is zero.
type SearchScope struct {
	ChatID  int64
	TopicID int
}

// SearchPosition is where the next page of a message search starts: below the
// last message found, named by Telegram's rate, the chat's id and the
// message's id. The zero position is the first page. It names a chat by id
// alone: the owner holds the address the next page is asked with (#349).
type SearchPosition struct {
	Rate   int
	ChatID int64
	ID     int
}

// SearchPage is one page of a message search's answer, newest first, and where
// the next page starts, nil when there is none.
type SearchPage struct {
	Found []FoundMessage
	Next  *SearchPosition
}

// FoundMessage is one message a search found, with the title of the chat it
// was found in: a search of every chat finds messages in chats the client may
// hold nothing of.
type FoundMessage struct {
	Message domain.Message
	Chat    string
}
