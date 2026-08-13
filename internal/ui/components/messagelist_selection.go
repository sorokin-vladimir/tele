package components

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// GroupMediaRef is one media part of the selected album, carrying everything the
// modal needs to open and page it: display index, kind, the photo or document
// ref, and the part's own message ID and meta.
type GroupMediaRef struct {
	Index   int
	Kind    domain.MediaKind
	Photo   *domain.PhotoRef
	Doc     *domain.DocumentRef
	MsgID   int
	Sender  string
	Date    time.Time
	DurSecs int // media duration in seconds (video), 0 when not applicable
}

// SelectedGroupMedia returns the media parts of the selected album in display
// order. Empty when the selection is not a multi-part album.
func (ml *MessageList) SelectedGroupMedia() []GroupMediaRef {
	it := ml.computeSelectedItem()
	if it == nil || len(it.parts) <= 1 {
		return nil
	}
	media := groupMediaParts(it.parts)
	out := make([]GroupMediaRef, 0, len(media))
	for _, gm := range media {
		ref := GroupMediaRef{Index: gm.Index, MsgID: gm.Msg.ID, Sender: gm.Msg.SenderName, Date: gm.Msg.Date}
		if gm.Msg.Photo != nil {
			ref.Kind = domain.MediaPhoto
			ref.Photo = gm.Msg.Photo
		} else if gm.Msg.Media != nil {
			ref.Kind = gm.Msg.Media.Kind
			ref.Doc = gm.Msg.Document
			ref.DurSecs = gm.Msg.Media.Duration
		}
		out = append(out, ref)
	}
	return out
}

// SelectedBubbleRect returns the rectangle of the selected message bubble from
// the most recent View() call, local to View()'s output. ok is false when there
// is no selected message or View() has not run yet.
func (ml *MessageList) SelectedBubbleRect() (Rect, bool) { return ml.selRect, ml.selRectOK }

func (ml *MessageList) SelectedMessageID() int {
	return ml.computeSelectedMsgID()
}

func (ml *MessageList) SelectedMessageIsOut() bool {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.IsOut
	}
	return false
}

// SelectedMessageSenderID is who wrote the selected message, 0 when there is no
// selection or the message names no sender. Outgoing messages report 0: the
// author is the account itself, and a profile of yourself is not phase one
// (#222).
func (ml *MessageList) SelectedMessageSenderID() int64 {
	if msg := ml.computeSelectedMsg(); msg != nil && !msg.IsOut {
		return msg.SenderID
	}
	return 0
}

// SelectedMessageText returns the plain text of the selected message and whether
// it carries any non-empty text. Media-only messages (no caption) report false.
func (ml *MessageList) SelectedMessageText() (string, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Text != "" {
		return msg.Text, true
	}
	return "", false
}

// SelectedMessageOpenTargets returns the openable targets (media + links) of the
// selected message, in display order. For a collapsed album it enumerates every
// media part. Empty when nothing is openable.
func (ml *MessageList) SelectedMessageOpenTargets() []OpenTarget {
	it := ml.computeSelectedItem()
	if it == nil {
		return nil
	}
	if len(it.parts) > 1 {
		return GroupOpenTargets(it.parts)
	}
	return MessageOpenTargets(it.msg)
}

// computeSelectedItem returns the selected list item (album-aware), or nil. It
// matches the selection by any of the item's part IDs so a multi-part album
// resolves to its single item.
func (ml *MessageList) computeSelectedItem() *listItem {
	id := ml.computeSelectedMsgID()
	if id == 0 {
		return nil
	}
	for i := range ml.items {
		if ml.items[i].kind != itemMessage {
			continue
		}
		for _, p := range ml.items[i].parts {
			if p.ID == id {
				return &ml.items[i]
			}
		}
	}
	return nil
}

func (ml *MessageList) SelectedMessageReplyToMsgID() int {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.ReplyToMsgID
	}
	return 0
}

func (ml *MessageList) SelectedMessagePhotoID() int64 {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Photo != nil {
		return msg.Photo.ID
	}
	return 0
}

// SelectedMessageVideo returns the document ref of the selected message when it
// is a playable video, for opening in an external player.
func (ml *MessageList) SelectedMessageVideo() (domain.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind.IsVideo() && msg.Document != nil {
		return *msg.Document, true
	}
	return domain.DocumentRef{}, false
}

// SelectedMessageVoice returns the document ref of the selected message when it
// is a voice message, for in-app playback.
func (ml *MessageList) SelectedMessageVoice() (domain.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind == domain.MediaVoice && msg.Document != nil {
		return *msg.Document, true
	}
	return domain.DocumentRef{}, false
}

// SelectedMessageGIF returns the document ref of the selected message when it is
// an animated GIF, for inline looping playback (#105 Phase 2b).
func (ml *MessageList) SelectedMessageGIF() (domain.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind == domain.MediaGIF && msg.Document != nil {
		return *msg.Document, true
	}
	return domain.DocumentRef{}, false
}

// SelectedMessagePhoto returns the full PhotoRef of the selected message when it
// is a photo, for saving to disk at full quality.
func (ml *MessageList) SelectedMessagePhoto() (domain.PhotoRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Photo != nil {
		return *msg.Photo, true
	}
	return domain.PhotoRef{}, false
}

// SelectedMessageMediaKind returns the media kind of the selected message and
// whether it carries any media. Photos report MediaPhoto (detected via the
// photo ref, independent of the Media field); document-backed media report
// their Media.Kind. Messages with no downloadable/openable media report false.
func (ml *MessageList) SelectedMessageMediaKind() (domain.MediaKind, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil {
		return 0, false
	}
	if msg.Photo != nil {
		return domain.MediaPhoto, true
	}
	if msg.Media != nil && msg.Document != nil {
		return msg.Media.Kind, true
	}
	return 0, false
}

// SelectedMessageDownloadDoc returns the document ref and media kind of the
// selected message when it is any downloadable document-backed media (video,
// round note, voice, audio, GIF, generic file). Stickers are excluded (saving
// them to disk is not offered); photos are handled by SelectedMessagePhoto.
func (ml *MessageList) SelectedMessageDownloadDoc() (domain.DocumentRef, domain.MediaKind, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil || msg.Media == nil || msg.Document == nil {
		return domain.DocumentRef{}, 0, false
	}
	if msg.Media.Kind == domain.MediaSticker {
		return domain.DocumentRef{}, 0, false
	}
	return *msg.Document, msg.Media.Kind, true
}

func (ml *MessageList) computeSelectedMsgID() int {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.ID
	}
	return 0
}

// SelectedOutboxRef returns the ref of the selected queued send, or "" when the
// selection is a message or there is none.
func (ml *MessageList) SelectedOutboxRef() string {
	if ml.cursorOutboxRef == "" {
		return ""
	}
	for _, e := range ml.outbox {
		if e.Ref == ml.cursorOutboxRef {
			return e.Ref
		}
	}
	return ""
}

// SelectedOutboxEntry returns the selected queued send. ok is false when the
// selection is a message.
func (ml *MessageList) SelectedOutboxEntry() (domain.OutboxEntry, bool) {
	if ml.cursorOutboxRef == "" {
		return domain.OutboxEntry{}, false
	}
	for _, e := range ml.outbox {
		if e.Ref == ml.cursorOutboxRef {
			return e, true
		}
	}
	return domain.OutboxEntry{}, false
}

func (ml *MessageList) computeSelectedMsg() *domain.Message {
	// The two halves of the cursor are mutually exclusive. An entry has no
	// message ID, so every action keyed on one must miss rather than hit a
	// neighbouring message (#193).
	if ml.cursorOutboxRef != "" {
		return nil
	}
	if len(ml.items) == 0 {
		return nil
	}
	// The explicit cursor, when set and still present, is the selection. It
	// falls back to the newest visible message below until initialized.
	if ml.cursorMsgID != 0 {
		if msg := ml.findMessage(ml.cursorMsgID); msg != nil {
			return msg
		}
	}
	selectedIdx := -1
	linesUsed := 0
	for i := ml.viewStart; i < len(ml.items); i++ {
		skipped := 0
		if i == ml.viewStart {
			skipped = ml.lineOffset
		}
		h := ml.itemHeight(i)
		if ml.items[i].kind == itemMessage {
			firstContentVP := linesUsed + (1 - skipped)
			if firstContentVP >= 0 && firstContentVP < ml.viewHeight {
				selectedIdx = i
			}
		}
		visible := h - skipped
		if visible < 0 {
			visible = 0
		}
		linesUsed += visible
		if linesUsed >= ml.viewHeight {
			break
		}
	}
	if selectedIdx < 0 {
		return nil
	}
	return &ml.items[selectedIdx].msg
}
