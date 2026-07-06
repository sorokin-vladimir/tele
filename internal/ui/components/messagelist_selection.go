package components

import "github.com/sorokin-vladimir/tele/internal/store"

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

// SelectedMessageText returns the plain text of the selected message and whether
// it carries any non-empty text. Media-only messages (no caption) report false.
func (ml *MessageList) SelectedMessageText() (string, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Text != "" {
		return msg.Text, true
	}
	return "", false
}

// SelectedMessageOpenTargets returns the openable targets (media + links) of the
// selected message, in display order. Empty when nothing is openable.
func (ml *MessageList) SelectedMessageOpenTargets() []OpenTarget {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return MessageOpenTargets(*msg)
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
func (ml *MessageList) SelectedMessageVideo() (store.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind.IsVideo() && msg.Document != nil {
		return *msg.Document, true
	}
	return store.DocumentRef{}, false
}

// SelectedMessageVoice returns the document ref of the selected message when it
// is a voice message, for in-app playback.
func (ml *MessageList) SelectedMessageVoice() (store.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind == store.MediaVoice && msg.Document != nil {
		return *msg.Document, true
	}
	return store.DocumentRef{}, false
}

// SelectedMessageGIF returns the document ref of the selected message when it is
// an animated GIF, for inline looping playback (#105 Phase 2b).
func (ml *MessageList) SelectedMessageGIF() (store.DocumentRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Media != nil &&
		msg.Media.Kind == store.MediaGIF && msg.Document != nil {
		return *msg.Document, true
	}
	return store.DocumentRef{}, false
}

// SelectedMessagePhoto returns the full PhotoRef of the selected message when it
// is a photo, for saving to disk at full quality.
func (ml *MessageList) SelectedMessagePhoto() (store.PhotoRef, bool) {
	if msg := ml.computeSelectedMsg(); msg != nil && msg.Photo != nil {
		return *msg.Photo, true
	}
	return store.PhotoRef{}, false
}

// SelectedMessageMediaKind returns the media kind of the selected message and
// whether it carries any media. Photos report MediaPhoto (detected via the
// photo ref, independent of the Media field); document-backed media report
// their Media.Kind. Messages with no downloadable/openable media report false.
func (ml *MessageList) SelectedMessageMediaKind() (store.MediaKind, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil {
		return 0, false
	}
	if msg.Photo != nil {
		return store.MediaPhoto, true
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
func (ml *MessageList) SelectedMessageDownloadDoc() (store.DocumentRef, store.MediaKind, bool) {
	msg := ml.computeSelectedMsg()
	if msg == nil || msg.Media == nil || msg.Document == nil {
		return store.DocumentRef{}, 0, false
	}
	if msg.Media.Kind == store.MediaSticker {
		return store.DocumentRef{}, 0, false
	}
	return *msg.Document, msg.Media.Kind, true
}

func (ml *MessageList) computeSelectedMsgID() int {
	if msg := ml.computeSelectedMsg(); msg != nil {
		return msg.ID
	}
	return 0
}

func (ml *MessageList) computeSelectedMsg() *store.Message {
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
