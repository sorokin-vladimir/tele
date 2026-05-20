package store

import "time"

type PeerType int

const (
	PeerUser PeerType = iota
	PeerGroup
	PeerChannel
	PeerSuperGroup
)

type Peer struct {
	ID         int64
	Type       PeerType
	AccessHash int64
}

func (p Peer) IsUser() bool       { return p.Type == PeerUser }
func (p Peer) IsGroup() bool      { return p.Type == PeerGroup || p.Type == PeerSuperGroup }
func (p Peer) IsChannel() bool    { return p.Type == PeerChannel }
func (p Peer) IsSuperGroup() bool { return p.Type == PeerSuperGroup }

type Reaction struct {
	Emoji    string
	Count    int
	IsChosen bool
}

type MessageEntity struct {
	Type   string // "bold", "italic", "code", "pre" — UTF-16 offsets (Telegram encoding)
	Offset int
	Length int
}

type PhotoRef struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ThumbSize     string // e.g. "m" (320px), "s" (100px)
}

type Chat struct {
	ID              int64
	Title           string
	Peer            Peer
	Pinned          bool
	UnreadCount     int
	ReadInboxMaxID  int
	ReadOutboxMaxID int
	LastMessage     *Message
	IsContact       bool
	IsBot           bool
	IsMuted         bool
}

type FolderFilter struct {
	ID    int    // Telegram filter ID; 0 = "All Chats" sentinel
	Title string
	Emoji string

	PinnedPeers  []int64
	IncludePeers []int64
	ExcludePeers []int64

	// Category flags
	Contacts    bool
	NonContacts bool
	Groups      bool
	Broadcasts  bool
	Bots        bool

	// Exclusion flags
	ExcludeMuted    bool
	ExcludeRead     bool
	ExcludeArchived bool
}

type Message struct {
	ID           int
	ChatID       int64
	SenderID     int64
	SenderName   string
	Text         string
	Date         time.Time
	IsOut        bool
	Entities     []MessageEntity
	Photo        *PhotoRef  // nil if message has no photo
	ReplyToMsgID int        // 0 if not a reply
	EditDate     *time.Time // nil if not edited
	Reactions    []Reaction
}

type EventKind int

const (
	EventNewMessage EventKind = iota
	EventReadInbox
	EventReadOutbox
	EventReactionsUpdate
	EventDeleteMessages
)

type Event struct {
	Kind      EventKind
	Message   Message
	ChatID    int64
	ReadMaxID int
	MsgID     int
	MsgIDs    []int
	Reactions []Reaction
}
