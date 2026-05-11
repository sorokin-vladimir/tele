package tg

import (
	"context"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func setupDispatcher(dispatcher *tg.UpdateDispatcher, events chan<- store.Event) {
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateNewMessage) error {
		peerID := extractPeerID(upd.Message)
		msg, ok := convertMessage(upd.Message, peerID)
		if !ok {
			return nil
		}
		select {
		case events <- store.Event{Kind: store.EventNewMessage, Message: msg}:
		default:
			// drop if buffer full
		}
		return nil
	})
}

func extractPeerID(raw tg.MessageClass) int64 {
	msg, ok := raw.(*tg.Message)
	if !ok {
		return 0
	}
	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	}
	return 0
}
