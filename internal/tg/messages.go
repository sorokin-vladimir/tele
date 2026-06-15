package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"

	"github.com/gotd/td/tg"
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
		c.traceLog.Debug("GetHistory done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

func (c *GotdClient) SendMessage(ctx context.Context, peer store.Peer, text string, replyToMsgID int) (int, error) {
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

		updates, err := api.MessagesSendMessage(ctx, buildSendRequest(inputPeer, text, randomID, replyToMsgID))
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

func buildSendRequest(inputPeer tg.InputPeerClass, text string, randomID int64, replyToMsgID int) *tg.MessagesSendMessageRequest {
	req := &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  text,
		RandomID: randomID,
	}
	if replyToMsgID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID}
	}
	return req
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
