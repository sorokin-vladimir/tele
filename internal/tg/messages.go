package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// RefreshMessage re-fetches one message and returns it with fresh media refs.
func (c *GotdClient) RefreshMessage(ctx context.Context, peer store.Peer, msgID int) (store.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return store.Message{}, err
	}

	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}
	var out store.Message
	err = WithRetry(ctx, func() error {
		var (
			result tg.MessagesMessagesClass
			err    error
		)
		if isChannelPeer(peer) {
			result, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: inputChannel(peer),
				ID:      ids,
			})
		} else {
			result, err = api.MessagesGetMessages(ctx, ids)
		}
		if err != nil {
			c.log.Error("RefreshMessage failed", zap.Error(err))
			return err
		}
		msgs := parseHistory(result, peer.ID)
		m, ok := selectMessageByID(msgs, msgID)
		if !ok {
			return fmt.Errorf("refresh message %d: not found", msgID)
		}
		out = m
		return nil
	})
	return out, err
}

func (c *GotdClient) GetHistory(ctx context.Context, peer store.Peer, offsetID int, limit int) ([]store.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	c.traceLog.Debug("GetHistory", zap.Int64("peer_id", peer.ID), zap.Int("offsetID", offsetID), zap.Int("limit", limit))
	inputPeer := peerToInput(peer)
	var msgs []store.Message
	err = WithRetry(ctx, func() error {
		result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     inputPeer,
			Limit:    limit,
			OffsetID: offsetID,
		})
		if err != nil {
			c.log.Error("MessagesGetHistory failed", zap.Error(err))
			return err
		}
		msgs = parseHistory(result, peer.ID)
		// Seed the sender-name cache from the fully-resolved history so a later
		// live update that omits a sender's entity still resolves the name (#161).
		for _, m := range msgs {
			c.senderNames.put(m.SenderID, m.SenderName)
		}
		c.traceLog.Debug("GetHistory done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

func (c *GotdClient) SendMessage(ctx context.Context, peer store.Peer, text string, replyToMsgID int, entities []store.MessageEntity) (int, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return 0, err
	}

	c.traceLog.Debug("SendMessage", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	inputPeer := peerToInput(peer)
	var realID int
	err = WithRetry(ctx, func() error {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}
		randomID := int64(binary.LittleEndian.Uint64(buf[:]))

		updates, err := api.MessagesSendMessage(ctx, buildSendRequest(inputPeer, text, randomID, replyToMsgID, entities))
		if err != nil {
			c.log.Error("MessagesSendMessage failed", zap.Error(err))
			return err
		}
		realID = extractSentMessageID(updates, randomID)
		if realID != 0 {
			c.suppressMu.Lock()
			c.suppressIDs[realID] = struct{}{}
			c.suppressMu.Unlock()
		}
		c.traceLog.Debug("SendMessage ok", zap.Int64("peer_id", peer.ID), zap.Int("real_id", realID))
		return nil
	})
	return realID, err
}

// SendMediaParams carries everything SendMedia needs. Media is a ready-made
// InputMediaClass; SendMedia is type-agnostic and does not inspect it.
type SendMediaParams struct {
	Peer         store.Peer
	Media        tg.InputMediaClass
	Caption      string
	ReplyToMsgID int
}

func (c *GotdClient) SendMedia(ctx context.Context, p SendMediaParams) (int, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return 0, err
	}

	c.traceLog.Debug("SendMedia", zap.Int64("peer_id", p.Peer.ID), zap.Int("caption_len", len(p.Caption)))
	inputPeer := peerToInput(p.Peer)
	var realID int
	err = WithRetry(ctx, func() error {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}
		randomID := int64(binary.LittleEndian.Uint64(buf[:]))

		updates, err := api.MessagesSendMedia(ctx, buildSendMediaRequest(inputPeer, p.Media, p.Caption, randomID, p.ReplyToMsgID))
		if err != nil {
			c.log.Error("MessagesSendMedia failed", zap.Error(err))
			return err
		}
		realID = extractSentMessageID(updates, randomID)
		if realID != 0 {
			c.suppressMu.Lock()
			c.suppressIDs[realID] = struct{}{}
			c.suppressMu.Unlock()
		}
		c.traceLog.Debug("SendMedia ok", zap.Int64("peer_id", p.Peer.ID), zap.Int("real_id", realID))
		return nil
	})
	return realID, err
}

// ErrForwardRestricted is returned by ForwardMessages when the source chat has
// content protection enabled (the server replies CHAT_FORWARDS_RESTRICTED).
var ErrForwardRestricted = errors.New("forwarding restricted")

// ForwardMessages forwards messages by ID from one peer to another via
// messages.forwardMessages. No optimistic insert is performed: when the target
// is the open chat, the message arrives through the normal live-update path.
func (c *GotdClient) ForwardMessages(ctx context.Context, from store.Peer, to store.Peer, ids []int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("ForwardMessages", zap.Int64("from", from.ID), zap.Int64("to", to.ID), zap.Int("count", len(ids)))
	return WithRetry(ctx, func() error {
		randomIDs := make([]int64, len(ids))
		for i := range randomIDs {
			var buf [8]byte
			if _, err := rand.Read(buf[:]); err != nil {
				return err
			}
			randomIDs[i] = int64(binary.LittleEndian.Uint64(buf[:]))
		}
		_, err := api.MessagesForwardMessages(ctx, buildForwardRequest(peerToInput(from), peerToInput(to), ids, randomIDs))
		if err != nil {
			if isForwardRestrictedErr(err) {
				return ErrForwardRestricted
			}
			c.log.Error("MessagesForwardMessages failed", zap.Error(err))
			return err
		}
		return nil
	})
}

func buildForwardRequest(fromPeer, toPeer tg.InputPeerClass, ids []int, randomIDs []int64) *tg.MessagesForwardMessagesRequest {
	return &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer,
		ToPeer:   toPeer,
		ID:       ids,
		RandomID: randomIDs,
	}
}

func isForwardRestrictedErr(err error) bool {
	return tgerr.Is(err, "CHAT_FORWARDS_RESTRICTED")
}

func buildSendRequest(inputPeer tg.InputPeerClass, text string, randomID int64, replyToMsgID int, entities []store.MessageEntity) *tg.MessagesSendMessageRequest {
	req := &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  text,
		RandomID: randomID,
	}
	if replyToMsgID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID}
	}
	if ent := convertToTGEntities(entities); len(ent) > 0 {
		req.Entities = ent
	}
	return req
}

// convertToTGEntities maps store entities to Telegram send-side entities. Only
// name-based mentions need an explicit entity (they carry an InputUser); plain
// @username mentions are detected server-side and are skipped here.
func convertToTGEntities(es []store.MessageEntity) []tg.MessageEntityClass {
	if len(es) == 0 {
		return nil
	}
	var out []tg.MessageEntityClass
	for _, e := range es {
		if e.Type != "mention_name" {
			continue
		}
		out = append(out, &tg.InputMessageEntityMentionName{
			Offset: e.Offset,
			Length: e.Length,
			UserID: &tg.InputUser{UserID: e.UserID, AccessHash: e.AccessHash},
		})
	}
	return out
}

func buildSendMediaRequest(inputPeer tg.InputPeerClass, media tg.InputMediaClass, caption string, randomID int64, replyToMsgID int) *tg.MessagesSendMediaRequest {
	req := &tg.MessagesSendMediaRequest{
		Peer:     inputPeer,
		Media:    media,
		Message:  caption,
		RandomID: randomID,
	}
	if replyToMsgID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID}
	}
	return req
}

// BuildInputMediaUploadedPhoto wraps an uploaded InputFile into an
// InputMediaUploadedPhoto for messages.sendMedia. The server recompresses it and
// produces an inline photo. Caption/entities are carried separately by SendMedia.
func BuildInputMediaUploadedPhoto(f tg.InputFileClass) tg.InputMediaClass {
	return &tg.InputMediaUploadedPhoto{File: f}
}

// BuildInputMediaUploadedDocument wraps an uploaded InputFile into an
// InputMediaUploadedDocument for messages.sendMedia, forcing the generic
// document path (ForceFile) so Telegram does not reinterpret an image as a
// photo. The filename is attached via DocumentAttributeFilename; an empty MIME
// falls back to application/octet-stream.
func BuildInputMediaUploadedDocument(f tg.InputFileClass, fileName, mime string) tg.InputMediaClass {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return &tg.InputMediaUploadedDocument{
		File:     f,
		MimeType: mime,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: fileName},
		},
		ForceFile: true,
	}
}

// BuildInputMediaUploadedVideo wraps an uploaded InputFile into an
// InputMediaUploadedDocument carrying DocumentAttributeVideo (always
// SupportsStreaming; duration/w/h are passed through verbatim) plus
// DocumentAttributeFilename. Unlike the generic document builder it does NOT set
// ForceFile, so Telegram renders it as inline video. thumb is optional: when
// non-nil it is attached as the document Thumb so the bubble has a preview before
// the server generates its own. An empty mime falls back to video/mp4.
func BuildInputMediaUploadedVideo(f tg.InputFileClass, fileName, mime string, dur, w, h int, thumb tg.InputFileClass) tg.InputMediaClass {
	if mime == "" {
		mime = "video/mp4"
	}
	doc := &tg.InputMediaUploadedDocument{
		File:     f,
		MimeType: mime,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{
				SupportsStreaming: true,
				Duration:          float64(dur),
				W:                 w,
				H:                 h,
			},
			&tg.DocumentAttributeFilename{FileName: fileName},
		},
	}
	if thumb != nil {
		doc.SetThumb(thumb)
	}
	return doc
}

func (c *GotdClient) MarkRead(ctx context.Context, peer store.Peer, maxID int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		if isChannelPeer(peer) {
			_, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
				Channel: inputChannel(peer),
				MaxID:   maxID,
			})
			return err
		}
		_, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
			Peer:  peerToInput(peer),
			MaxID: maxID,
		})
		return err
	})
}

func (c *GotdClient) ReadReactions(ctx context.Context, peer store.Peer) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		_, err := api.MessagesReadReactions(ctx, &tg.MessagesReadReactionsRequest{
			Peer: peerToInput(peer),
		})
		return err
	})
}

func (c *GotdClient) ReadMentions(ctx context.Context, peer store.Peer) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		_, err := api.MessagesReadMentions(ctx, &tg.MessagesReadMentionsRequest{
			Peer: peerToInput(peer),
		})
		return err
	})
}

func (c *GotdClient) DeleteMessages(ctx context.Context, peer store.Peer, ids []int, revoke bool) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("DeleteMessages", zap.Int64("peer_id", peer.ID), zap.Int("count", len(ids)), zap.Bool("revoke", revoke))
	return WithRetry(ctx, func() error {
		if isChannelPeer(peer) {
			// Channel/supergroup messages are always deleted for all members; revoke is N/A.
			_, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
				Channel: inputChannel(peer),
				ID:      ids,
			})
			return err
		}
		_, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: revoke,
			ID:     ids,
		})
		return err
	})
}

func (c *GotdClient) EditMessage(ctx context.Context, peer store.Peer, msgID int, text string) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("EditMessage", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:    peerToInput(peer),
			ID:      msgID,
			Message: text,
		})
		if err != nil {
			c.log.Error("MessagesEditMessage failed", zap.Error(err))
		}
		return err
	})
}

// SaveDraft persists (or clears, when text is empty) the message draft for a
// peer via messages.saveDraft (#62). Telegram broadcasts the change to the
// account's other clients as updateDraftMessage.
func (c *GotdClient) SaveDraft(ctx context.Context, peer store.Peer, text string) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("SaveDraft", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesSaveDraft(ctx, buildSaveDraftRequest(peerToInput(peer), text))
		if err != nil {
			c.log.Error("MessagesSaveDraft failed", zap.Error(err))
		}
		return err
	})
}

func buildSaveDraftRequest(inputPeer tg.InputPeerClass, text string) *tg.MessagesSaveDraftRequest {
	return &tg.MessagesSaveDraftRequest{
		Peer:    inputPeer,
		Message: text,
	}
}

func (c *GotdClient) SendReaction(ctx context.Context, peer store.Peer, msgID int, emoji string) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("SendReaction", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID), zap.String("emoji", emoji))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
			Peer:     peerToInput(peer),
			MsgID:    msgID,
			Reaction: buildReactionArg(emoji),
		})
		if err != nil {
			c.log.Error("MessagesSendReaction failed", zap.Error(err))
		}
		return err
	})
}

func buildReactionArg(emoji string) []tg.ReactionClass {
	if emoji == "" {
		return []tg.ReactionClass{} // empty vector = remove reaction
	}
	return []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
}

// isChannelPeer reports whether a peer is addressed via the channels.* API
// (channels and supergroups), as opposed to the messages.* API.
func isChannelPeer(p store.Peer) bool {
	return p.Type == store.PeerChannel || p.Type == store.PeerSuperGroup
}

// inputChannel builds the InputChannel for a channel/supergroup peer.
func inputChannel(p store.Peer) *tg.InputChannel {
	return &tg.InputChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
}

func peerToInput(p store.Peer) tg.InputPeerClass {
	switch p.Type {
	case store.PeerUser:
		return &tg.InputPeerUser{UserID: p.ID, AccessHash: p.AccessHash}
	case store.PeerGroup:
		return &tg.InputPeerChat{ChatID: p.ID}
	case store.PeerChannel, store.PeerSuperGroup:
		return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func extractSentMessageID(updates tg.UpdatesClass, randomID int64) int {
	if short, ok := updates.(*tg.UpdateShortSentMessage); ok {
		return short.ID
	}
	upds, ok := updates.(*tg.Updates)
	if !ok {
		return 0
	}
	for _, u := range upds.Updates {
		if mid, ok := u.(*tg.UpdateMessageID); ok && mid.RandomID == randomID {
			return mid.ID
		}
	}
	return 0
}

func typingActionToTG(a store.TypingAction) tg.SendMessageActionClass {
	switch a {
	case store.TypingActionTyping:
		return &tg.SendMessageTypingAction{}
	case store.TypingActionRecordAudio:
		return &tg.SendMessageRecordAudioAction{}
	case store.TypingActionUploadAudio:
		return &tg.SendMessageUploadAudioAction{}
	case store.TypingActionRecordVideo:
		return &tg.SendMessageRecordVideoAction{}
	case store.TypingActionUploadVideo:
		return &tg.SendMessageUploadVideoAction{}
	case store.TypingActionUploadPhoto:
		return &tg.SendMessageUploadPhotoAction{}
	case store.TypingActionUploadDocument:
		return &tg.SendMessageUploadDocumentAction{}
	case store.TypingActionChooseSticker:
		return &tg.SendMessageChooseStickerAction{}
	case store.TypingActionRecordRound:
		return &tg.SendMessageRecordRoundAction{}
	default:
		return &tg.SendMessageCancelAction{}
	}
}

func (c *GotdClient) SetTyping(ctx context.Context, peer store.Peer, action store.TypingAction) error {
	api, err := c.acquireAPI()
	if err != nil {
		return nil // typing is best-effort; ignore when not connected
	}
	_, err = api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{
		Peer:   peerToInput(peer),
		Action: typingActionToTG(action),
	})
	if err != nil {
		c.traceLog.Debug("SetTyping failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
	}
	return err
}
