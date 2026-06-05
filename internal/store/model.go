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
	ThumbSize     string // inline: "m" (320px) or best available
	FullThumbSize string // full quality: best large size ("x"→800px, "y"→1280px, "w"→2560px)
}

// MediaKind classifies the media a message carries, for display purposes.
type MediaKind int

const (
	MediaPhoto MediaKind = iota
	MediaVideo
	MediaVideoNote // round video message (кружок)
	MediaVoice
	MediaAudio
	MediaSticker
	MediaGIF
	MediaFile
	MediaLocation
	MediaOther // generic fallback (contact, poll, dice, …)
)

// MediaRef is the display-level description of a message's media. PhotoRef
// remains the download-capable reference and is set only for photos.
type MediaRef struct {
	Kind  MediaKind
	Emoji string // sticker's alt emoji; populated now for stickers
	// Populated in a later iteration:
	Duration int    // seconds, for video/voice/audio/note
	FileName string // for files
	Size     int64  // bytes, for files
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
	Online          bool
}

type FolderFilter struct {
	ID    int // Telegram filter ID; 0 = "All Chats" sentinel
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
	Media        *MediaRef  // nil if message has no media
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
	EventUserPresence
	EventTyping
)

type TypingAction int

const (
	TypingActionUnknown TypingAction = iota
	TypingActionTyping
	TypingActionRecordAudio
	TypingActionUploadAudio
	TypingActionRecordVideo
	TypingActionUploadVideo
	TypingActionUploadPhoto
	TypingActionUploadDocument
	TypingActionChooseSticker
	TypingActionRecordRound
	TypingActionCancel
)

func (a TypingAction) Label() string {
	switch a {
	case TypingActionTyping:
		return "typing"
	case TypingActionRecordAudio:
		return "recording audio"
	case TypingActionUploadAudio:
		return "sending audio"
	case TypingActionRecordVideo:
		return "recording video"
	case TypingActionUploadVideo:
		return "sending video"
	case TypingActionUploadPhoto:
		return "sending a photo"
	case TypingActionUploadDocument:
		return "sending a file"
	case TypingActionChooseSticker:
		return "choosing a sticker"
	case TypingActionRecordRound:
		return "recording a video message"
	default:
		return ""
	}
}

type Event struct {
	Kind         EventKind
	Message      Message
	ChatID       int64
	ReadMaxID    int
	MsgID        int
	MsgIDs       []int
	Reactions    []Reaction
	Online       bool
	TypingAction TypingAction
}
