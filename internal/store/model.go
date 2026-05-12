package store

import "time"

type PeerType int

const (
	PeerUser    PeerType = iota
	PeerGroup
	PeerChannel
)

type Peer struct {
	ID         int64
	Type       PeerType
	AccessHash int64
}

func (p Peer) IsUser() bool    { return p.Type == PeerUser }
func (p Peer) IsGroup() bool   { return p.Type == PeerGroup }
func (p Peer) IsChannel() bool { return p.Type == PeerChannel }

type Chat struct {
	ID          int64
	Title       string
	Peer        Peer
	Pinned      bool
	UnreadCount int
	LastMessage *Message
}

type Message struct {
	ID       int
	ChatID   int64
	SenderID int64
	Text     string
	Date     time.Time
	IsOut    bool
}

type EventKind int

const (
	EventNewMessage EventKind = iota
)

type Event struct {
	Kind    EventKind
	Message Message
}
