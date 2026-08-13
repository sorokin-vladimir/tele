// Package domain holds the account's data model: the types a view renders and
// a command addresses. It is pure data — no persistence, no gotd — so a client
// can hold it without holding the store.
package domain

import (
	"strconv"
	"strings"
	"time"
)

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
	// Language is the info string of a fenced code block ("```go"). Set only for
	// Type=="pre", and only on the send side for now (#152); the receive side
	// does not populate it yet.
	Language string
	// UserID/AccessHash are set only for Type=="mention_name" (name-based
	// mention of a user without a public username).
	UserID     int64
	AccessHash int64
}

// ChatMember is a group/channel participant offered by the @mention autocomplete.
//
// It is a membership rather than a lesser User: the same person is a member of
// many chats or of none, and a role in one chat belongs here rather than on the
// person (#222).
type ChatMember struct {
	UserID      int64
	Username    string // without leading '@'; empty if the user has no public username
	DisplayName string // First + Last, trimmed
	AccessHash  int64
}

// User is a person the account has an address for. It holds facts about the
// person and none about the conversation with them: a mute, an unread count and
// a draft belong to the Chat.
//
// A User may be partial. What the owner knows locally arrives first and the
// rest follows from users.getFullUser, so an empty Bio means "not known" as
// readily as "not set" — the caller that cares tracks which (#222).
type User struct {
	ID        int64
	Username  string // without leading '@'; empty if the user has no public username
	FirstName string
	LastName  string
	// Bio is the user's "about" text. Only ever set from the full response.
	Bio string
	// Phone is set only when Telegram's privacy settings return one, which is
	// mostly for mutual contacts. Empty otherwise.
	Phone string
	// Online is the coarse presence flag the dialog list already carries. It is
	// a boolean because that is all the client reads today; the full range of
	// last-seen states is #127.
	Online          bool
	IsBot           bool
	IsContact       bool
	IsMutualContact bool
	IsDeleted       bool
}

// DisplayName is the name to draw for a user: First + Last, falling back to the
// id when the person has neither, which is what a deleted account looks like.
func (u User) DisplayName() string {
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		return "User " + strconv.FormatInt(u.ID, 10)
	}
	return name
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
	ID         int
	ChatID     int64
	SenderID   int64
	SenderName string
	Text       string
	Date       time.Time
	IsOut      bool
	Entities   []MessageEntity
	Media      *MediaRef    // nil if message has no media
	Photo      *PhotoRef    // nil if message has no photo
	Document   *DocumentRef // nil if message has no document-backed media
	// GroupedID is Telegram's album key: album parts share the same non-zero
	// grouped_id. 0 means the message is not part of an album.
	GroupedID    int64
	ReplyToMsgID int        // 0 if not a reply
	EditDate     *time.Time // nil if not edited
	Reactions    []Reaction
	// HasUnreadReactions is true when the raw message carried at least one recent
	// reaction flagged unread (a not-yet-viewed reaction on one of our messages).
	HasUnreadReactions bool
	// Mentioned is the raw message `mentioned` flag: the message mentions us
	// (@username, or a reply to one of our messages). Drives the chat-list
	// unread-mention indicator when the message is incoming and unread (#155).
	Mentioned bool
	Forward   *ForwardInfo // nil if not forwarded
	// LocalMedia describes the files of a queued media send. Set only on the
	// bubble a client draws for an outbox entry; nil for real messages.
	LocalMedia *LocalMedia
}

// LocalMedia describes the files of a queued media send so the pending bubble
// can name them and show upload progress.
//
// It is built from the outbox entry when the bubble is drawn and never stored:
// what is in flight lives in the queue, not in a client's memory, so every
// attached client sees the same send rather than only the one that started it
// (#195).
type LocalMedia struct {
	Kind MediaKind
	// FileName names a lone file. Empty for a group, which is named by its count.
	FileName string
	Size     int64
	// Part is the 1-based file being uploaded now, Parts the total in the send.
	// Part is zero until the upload starts.
	Part  int
	Parts int
	// UploadProgress is the fraction uploaded (0..1) across the whole send.
	UploadProgress float64
}

// ForwardInfo describes the origin of a forwarded message.
type ForwardInfo struct {
	From string // display name of the original sender; empty if hidden
}

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
