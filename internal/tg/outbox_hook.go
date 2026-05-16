package tg

import (
	"context"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// outboxHook sits between the raw MTProto layer and the updates.Manager.
// It extracts UpdateReadHistoryOutbox / UpdateReadChannelOutbox directly from
// the wire update before pts-tracking sees it. updates.Manager drops these
// events when a pts gap exists (the pending buffer never flushes), so we must
// intercept them here to guarantee delivery.
type outboxHook struct {
	next   telegram.UpdateHandler
	events chan<- store.Event
	log    *zap.Logger
}

func newOutboxHook(next telegram.UpdateHandler, events chan<- store.Event, log *zap.Logger) *outboxHook {
	return &outboxHook{next: next, events: events, log: log}
}

func (h *outboxHook) Handle(ctx context.Context, u tg.UpdatesClass) error {
	h.extractOutboxReads(u)
	return h.next.Handle(ctx, u)
}

func (h *outboxHook) extractOutboxReads(u tg.UpdatesClass) {
	var upds []tg.UpdateClass
	switch u := u.(type) {
	case *tg.Updates:
		upds = u.Updates
	case *tg.UpdatesCombined:
		upds = u.Updates
	case *tg.UpdateShort:
		upds = []tg.UpdateClass{u.Update}
	default:
		return
	}
	for _, upd := range upds {
		switch upd := upd.(type) {
		case *tg.UpdateReadHistoryOutbox:
			chatID := peerIDFromPeer(upd.Peer)
			if chatID == 0 {
				continue
			}
			h.log.Debug("outboxHook: ReadHistoryOutbox", zap.Int64("chat_id", chatID), zap.Int("max_id", upd.MaxID))
			select {
			case h.events <- store.Event{Kind: store.EventReadOutbox, ChatID: chatID, ReadMaxID: upd.MaxID}:
			default:
			}
		case *tg.UpdateReadChannelOutbox:
			h.log.Debug("outboxHook: ReadChannelOutbox", zap.Int64("channel_id", upd.ChannelID), zap.Int("max_id", upd.MaxID))
			select {
			case h.events <- store.Event{Kind: store.EventReadOutbox, ChatID: upd.ChannelID, ReadMaxID: upd.MaxID}:
			default:
			}
		}
	}
}
