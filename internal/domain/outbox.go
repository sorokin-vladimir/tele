package domain

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// OutboxState is where a queued send has got to. There are three, not the four
// a naive reading suggests: "sent" is not a state but the moment the entry is
// deleted, because the real message is in the chat by then and a surviving
// entry would render a second bubble.
type OutboxState string

const (
	// OutboxQueued is waiting for its turn, or for NextAttemptAt to arrive.
	OutboxQueued OutboxState = "queued"
	// OutboxUploading has bytes going up. Distinct from OutboxSending because a
	// media send spends most of its life here and reads differently on screen:
	// a file being uploaded is not a request waiting for an answer.
	OutboxUploading OutboxState = "uploading"
	// OutboxSending has a request in flight. It, and OutboxUploading, are reset
	// to OutboxQueued on startup: a process that died mid-send left them behind.
	OutboxSending OutboxState = "sending"
	// OutboxFailed is terminal until the user retries or discards it. Reached
	// only through a non-retryable error kind, never through an attempt count.
	OutboxFailed OutboxState = "failed"
)

// OutboxKind tags the payload variant.
type OutboxKind string

const (
	// OutboxText is a plain text message.
	OutboxText OutboxKind = "text"
	// OutboxMedia is one album group of local files, with an optional caption.
	OutboxMedia OutboxKind = "media"
)

// OutboxMessage is the payload of an OutboxText entry.
type OutboxMessage struct {
	Text         string          `json:"text"`
	Entities     []MessageEntity `json:"entities,omitempty"`
	ReplyToMsgID int             `json:"reply_to,omitempty"`
}

// OutboxMediaSend is the payload of an OutboxMedia entry: one album group, which
// is one sendMultiMedia request and one album in the chat. A submission of more
// files than one group takes becomes several entries, because the group is what
// Telegram sends atomically (#195).
type OutboxMediaSend struct {
	Parts        []OutboxMediaPart `json:"parts"`
	Caption      string            `json:"caption,omitempty"`
	Entities     []MessageEntity   `json:"entities,omitempty"`
	ReplyToMsgID int               `json:"reply_to,omitempty"`
}

// OutboxMediaPart is one local file of a group. Name and Size are recorded at
// submission so the entry describes itself to a client that was not running when
// it was queued.
type OutboxMediaPart struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// SendAs is the user's explicit choice, as in "send this photo as a file".
	// Zero means the owner decides from the file's MIME type.
	SendAs MediaKind `json:"send_as,omitempty"`
}

// SentSend is a send that became a message, remembered by its ref after it
// left the queue: what a command waiting on it is told, and what keeps the
// same ref from being sent twice (#181).
type SentSend struct {
	Ref     string
	ChatID  int64
	TopicID int
	// MsgIDs are the messages it became: one for a text, one per part of an
	// album.
	MsgIDs []int
	At     time.Time
}

// OutboxEntry is one durable send. It crosses the owner boundary inside the
// chat projection and is JSON on the wire in v2, which is why it lives in
// domain rather than beside the queue that stores it.
type OutboxEntry struct {
	// Ref is the client's idempotency key, and the seed RandomID is derived
	// from. Resubmitting a known Ref is a no-op.
	Ref string
	// Seq is the submission order (the table's rowid). FIFO within a chat is
	// ordering by this.
	Seq    int64
	ChatID int64
	// TopicID is the forum topic the send was composed in, 0 outside a forum.
	// The chat and the topic together name the history the send is queued in,
	// and FIFO holds within a history rather than a whole forum (#275).
	TopicID int
	// RandomID is Telegram's deduplication key, derived from Ref so that a
	// resubmission in the window before anything was persisted still produces
	// the same value.
	RandomID int64
	Kind     OutboxKind
	State    OutboxState
	// Message is set when Kind is OutboxText.
	Message *OutboxMessage
	// Media is set when Kind is OutboxMedia.
	Media *OutboxMediaSend
	// Attempts counts failures that led to a backoff. It drives the backoff
	// curve and the display, and never decides terminality.
	Attempts      int
	CreatedAt     time.Time
	NextAttemptAt time.Time
	// SentMsgIDs are the IDs Telegram confirmed: one for a text message, several
	// for an album group. Set between a successful request and the moment the
	// messages land in state, which is when the entry goes. Deliberately not
	// persisted: a crash in that window must re-send rather than assume, and the
	// persisted RandomID makes that safe.
	SentMsgIDs []int
	// ErrKind, ErrReason and ErrDetail are empty unless State is OutboxFailed.
	// ErrReason is set only for a refusal, and is what the interface says to a
	// person; ErrDetail is the raw Telegram type behind it, kept for the logs
	// and for anyone filing a report.
	ErrKind   telerr.Kind
	ErrReason telerr.Reason
	ErrDetail string
}
