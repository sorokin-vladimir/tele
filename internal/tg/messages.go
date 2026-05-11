package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) GetHistory(ctx context.Context, peer store.Peer, limit int) ([]store.Message, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	inputPeer := peerToInput(peer)
	var msgs []store.Message
	err := WithRetry(ctx, func() error {
		result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:  inputPeer,
			Limit: limit,
		})
		if err != nil {
			return err
		}
		msgs = parseHistory(result, peer.ID)
		return nil
	})
	return msgs, err
}

func (c *GotdClient) SendMessage(ctx context.Context, peer store.Peer, text string) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}

	inputPeer := peerToInput(peer)
	return WithRetry(ctx, func() error {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}
		randomID := int64(binary.LittleEndian.Uint64(buf[:]))

		_, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
			Peer:     inputPeer,
			Message:  text,
			RandomID: randomID,
		})
		return err
	})
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

func parseHistory(result tg.MessagesMessagesClass, chatID int64) []store.Message {
	var rawMsgs []tg.MessageClass

	switch v := result.(type) {
	case *tg.MessagesMessages:
		rawMsgs = v.Messages
	case *tg.MessagesMessagesSlice:
		rawMsgs = v.Messages
	case *tg.MessagesChannelMessages:
		rawMsgs = v.Messages
	default:
		return nil
	}

	out := make([]store.Message, 0, len(rawMsgs))
	for _, raw := range rawMsgs {
		if msg, ok := convertMessage(raw, chatID); ok {
			out = append(out, msg)
		}
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
	}, true
}
