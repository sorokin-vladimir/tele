package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) GetHistory(ctx context.Context, peer store.Peer, offsetID int, limit int) ([]store.Message, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	c.log.Debug("GetHistory", zap.Int64("peer_id", peer.ID), zap.Int("offsetID", offsetID), zap.Int("limit", limit))
	inputPeer := peerToInput(peer)
	var msgs []store.Message
	err := WithRetry(ctx, func() error {
		result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     inputPeer,
			Limit:    limit,
			OffsetID: offsetID,
		})
		if err != nil {
			c.log.Error("MessagesGetHistory failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
			return err
		}
		msgs = parseHistory(result, peer.ID)
		c.log.Debug("GetHistory done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

func (c *GotdClient) SendMessage(ctx context.Context, peer store.Peer, text string) (int, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return 0, fmt.Errorf("not connected")
	}

	c.log.Debug("SendMessage", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	inputPeer := peerToInput(peer)
	var realID int
	err := WithRetry(ctx, func() error {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}
		randomID := int64(binary.LittleEndian.Uint64(buf[:]))

		updates, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     inputPeer,
			Message:  text,
			RandomID: randomID,
		})
		if err != nil {
			c.log.Error("MessagesSendMessage failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
			return err
		}
		realID = extractSentMessageID(updates, randomID)
		if realID != 0 {
			c.suppressMu.Lock()
			c.suppressIDs[realID] = struct{}{}
			c.suppressMu.Unlock()
		}
		c.log.Debug("SendMessage ok", zap.Int64("peer_id", peer.ID), zap.Int("real_id", realID))
		return nil
	})
	return realID, err
}

func peerToInput(p store.Peer) tg.InputPeerClass {
	switch p.Type {
	case store.PeerUser:
		return &tg.InputPeerUser{UserID: p.ID, AccessHash: p.AccessHash}
	case store.PeerGroup:
		return &tg.InputPeerChat{ChatID: p.ID}
	case store.PeerChannel:
		return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

func buildNameMap(users []tg.UserClass) map[int64]string {
	m := make(map[int64]string, len(users))
	for _, u := range users {
		user, ok := u.(*tg.User)
		if !ok {
			continue
		}
		name := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if name == "" {
			name = fmt.Sprintf("User %d", user.ID)
		}
		m[user.ID] = name
	}
	return m
}

func parseHistory(result tg.MessagesMessagesClass, chatID int64) []store.Message {
	var rawMsgs []tg.MessageClass
	var rawUsers []tg.UserClass

	switch v := result.(type) {
	case *tg.MessagesMessages:
		rawMsgs, rawUsers = v.Messages, v.Users
	case *tg.MessagesMessagesSlice:
		rawMsgs, rawUsers = v.Messages, v.Users
	case *tg.MessagesChannelMessages:
		rawMsgs, rawUsers = v.Messages, v.Users
	default:
		return nil
	}

	nameMap := buildNameMap(rawUsers)

	out := make([]store.Message, 0, len(rawMsgs))
	for _, raw := range rawMsgs {
		if msg, ok := convertMessage(raw, chatID); ok {
			msg.SenderName = nameMap[msg.SenderID]
			out = append(out, msg)
		}
	}
	// Telegram API returns newest-first; reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func convertMessage(raw tg.MessageClass, chatID int64) (store.Message, bool) {
	msg, ok := raw.(*tg.Message)
	if !ok {
		return store.Message{}, false
	}
	senderID := int64(0)
	if from, ok := msg.FromID.(*tg.PeerUser); ok {
		senderID = from.UserID
	}
	return store.Message{
		ID:       msg.ID,
		ChatID:   chatID,
		SenderID: senderID,
		Text:     msg.Message,
		Date:     time.Unix(int64(msg.Date), 0),
		IsOut:    msg.Out,
		Entities: convertEntities(msg.Entities),
	}, true
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

func convertEntities(entities []tg.MessageEntityClass) []store.MessageEntity {
	if len(entities) == 0 {
		return nil
	}
	out := make([]store.MessageEntity, 0, len(entities))
	for _, e := range entities {
		switch v := e.(type) {
		case *tg.MessageEntityBold:
			out = append(out, store.MessageEntity{Type: "bold", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityItalic:
			out = append(out, store.MessageEntity{Type: "italic", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityCode:
			out = append(out, store.MessageEntity{Type: "code", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityPre:
			out = append(out, store.MessageEntity{Type: "pre", Offset: v.Offset, Length: v.Length})
		}
	}
	return out
}
