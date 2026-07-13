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
	Type   string // "bold", "italic", "code", "pre", "strike", "underline", "text_url", "url", "email", "phone", "bank_card", "mention", "mention_name", "hashtag", "cashtag", "bot_command" — UTF-16 offsets (Telegram encoding)
	Offset int
	Length int
	URL    string // for "text_url": the hidden target URL; empty otherwise
	// UserID/AccessHash are set only for Type=="mention_name" (name-based
	// mention of a user without a public username).
	UserID     int64
	AccessHash int64
}

// ChatMember is a group/channel participant offered by the @mention autocomplete.
type ChatMember struct {
	UserID      int64
	Username    string // without leading '@'; empty if the user has no public username
	DisplayName string // First + Last, trimmed
	AccessHash  int64
}

type PhotoRef struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ThumbSize     string // inline: "m" (320px) or best available
	FullThumbSize string // full quality: best large size ("x"→800px, "y"→1280px, "w"→2560px)
}

// DocumentRef is the download-capable reference for document-backed media
// (video, round video, voice, audio, sticker, gif, file). The full file is
// fetched via DownloadDocument; ThumbSize (when present) names a PhotoSize in
// the document's thumbnail set for an inline preview.
type DocumentRef struct {
	ID            int64
	AccessHash    int64
	FileReference []byte
	DCID          int
	ThumbSize     string // best thumbnail PhotoSize type, "" if no thumbnail
	MimeType      string
	FileName      string
	Size          int64
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

// IsVideo reports whether the kind is a playable video (regular or round note),
// both of which render an inline thumbnail preview and open in an external player.
func (k MediaKind) IsVideo() bool {
	return k == MediaVideo || k == MediaVideoNote
}

// IsStaticSticker reports whether the media is a sticker whose document is a
// static WEBP image (renderable inline), as opposed to an animated .tgs or
// video .webm sticker.
func IsStaticSticker(m *MediaRef, d *DocumentRef) bool {
	return m != nil && m.Kind == MediaSticker &&
		d != nil && d.MimeType == "image/webp"
}

// MediaRef is the display-level description of a message's media. PhotoRef
// remains the download-capable reference and is set only for photos.
type MediaRef struct {
	Kind  MediaKind
	Emoji string // sticker's alt emoji; populated now for stickers
	// Audio/voice metadata.
	Duration  int    // seconds, for video/voice/audio/note
	Waveform  []byte // bitpacked 5-bit amplitude samples, for voice messages
	Title     string // song title, for audio
	Performer string // performer, for audio
	// File metadata (from the document), populated for document-backed media.
	FileName string // original file name
	Size     int64  // bytes
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
	// UnreadMark is the Telegram dialog `unread_mark` flag: a manual
	// "mark as unread" that is independent of UnreadCount.
	UnreadMark bool
	// UnreadReactionsCount is the Telegram dialog `unread_reactions_count`:
	// reactions on your messages that you have not yet viewed. Persisted like
	// UnreadCount; cleared by messages.readReactions when the chat is opened.
	UnreadReactionsCount int
	// UnreadMentionsCount is the Telegram dialog `unread_mentions_count`:
	// messages that mention you (or reply to you) that you have not yet viewed.
	// Persisted like UnreadCount; cleared by messages.readMentions on open.
	UnreadMentionsCount int
	// IsArchived reports whether the chat lives in the built-in Archive
	// folder (folder_id 1).
	IsArchived bool
	// Draft is the unsent message draft synced with Telegram (#62). It is
	// loaded from the dialog list and kept current via updateDraftMessage; it
	// is not persisted to disk (the server is the source of truth on restart).
	Draft string
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
	Media        *MediaRef    // nil if message has no media
	Photo        *PhotoRef    // nil if message has no photo
	Document     *DocumentRef // nil if message has no document-backed media
	ReplyToMsgID int          // 0 if not a reply
	EditDate     *time.Time   // nil if not edited
	Reactions    []Reaction
	// HasUnreadReactions is true when the raw message carried at least one recent
	// reaction flagged unread (a not-yet-viewed reaction on one of our messages).
	HasUnreadReactions bool
	// Mentioned is the raw message `mentioned` flag: the message mentions us
	// (@username, or a reply to one of our messages). Drives the chat-list
	// unread-mention indicator when the message is incoming and unread (#155).
	Mentioned bool
	Forward   *ForwardInfo // nil if not forwarded
	// LocalMedia describes media being uploaded for an optimistic outgoing bubble,
	// rendered from a local file before the server confirms. nil for normal messages.
	LocalMedia *LocalMedia
}

// LocalMedia is the local-file description of media on an optimistic outgoing
// message, so the bubble can render (via the inbound media pipeline, keyed on Kind)
// before messages.sendMedia is confirmed.
type LocalMedia struct {
	Path     string
	Kind     MediaKind
	FileName string
	Size     int64
	// UploadProgress is the fraction uploaded (0..1) for the optimistic bubble.
	UploadProgress float64
	// UploadState tracks the optimistic upload; the zero value is UploadUploading.
	// Success clears LocalMedia entirely, so there is no "done" state.
	UploadState UploadState
}

// UploadState is the state of an in-flight optimistic media upload.
type UploadState int

const (
	UploadUploading UploadState = iota
	UploadFailed
)

// ForwardInfo describes the origin of a forwarded message.
type ForwardInfo struct {
	From string // display name of the original sender; empty if hidden
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
	// EventMuteUpdate reports that a chat's mute state changed server-side
	// (e.g. muted/unmuted from another device).
	EventMuteUpdate
	// EventEditMessage reports that a message was edited on another client.
	// The updated message is carried in Event.Message.
	EventEditMessage
	// EventDraftMessage reports that a chat's draft changed server-side (e.g.
	// edited on another device, or cleared on send). The text is in Event.Draft.
	EventDraftMessage
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
	Kind      EventKind
	Message   Message
	ChatID    int64
	ReadMaxID int
	MsgID     int
	MsgIDs    []int
	Reactions []Reaction
	// ReactionsUnread reports that an EventReactionsUpdate carries at least one
	// unread recent reaction (from UpdateMessageReactions).
	ReactionsUnread bool
	// ReactionEmoji is the emoji of the newest unread reaction (for notifications);
	// empty for a custom-emoji reaction. ReactionDate is when that reaction was
	// added, used to suppress stale catch-up reactions after an idle reconnect.
	ReactionEmoji string
	ReactionDate  time.Time
	Online        bool
	TypingAction  TypingAction
	Muted         bool
	// Draft carries the new draft text for EventDraftMessage.
	Draft string
}
