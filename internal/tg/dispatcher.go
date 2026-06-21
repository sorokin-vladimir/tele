package tg

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// shouldSuppress is called for every incoming message; it must be non-blocking.
func setupDispatcher(
	dispatcher *tg.UpdateDispatcher,
	mustDeliver chan<- store.Event,
	droppable chan<- store.Event,
	log *zap.Logger,
	shouldSuppress func(id int) bool,
) {
	var drops atomic.Int64

	logDrop := func(kind string) {
		n := drops.Add(1)
		log.Warn("droppable event dropped", zap.String("kind", kind), zap.Int64("total_drops", n))
	}

	// handleNewMessage converts a raw incoming message and, unless suppressed,
	// emits a store.EventNewMessage. It is shared by the UpdateNewMessage
	// (users/basic groups) and UpdateNewChannelMessage (channels/supergroups)
	// handlers, which carry an identically shaped Message field.
	handleNewMessage := func(ctx context.Context, e tg.Entities, raw tg.MessageClass) error {
		peerID := extractPeerID(raw)
		msg, ok := convertMessage(raw, peerID)
		if !ok {
			return nil
		}
		if user, ok := e.Users[msg.SenderID]; ok {
			name := strings.TrimSpace(user.FirstName + " " + user.LastName)
			if name == "" {
				name = fmt.Sprintf("User %d", user.ID)
			}
			msg.SenderName = name
		} else if msg.SenderID == 0 && !msg.IsOut {
			// nil FromID: sender is the chat peer — try user first (private chat),
			// then channel or group (anonymous post)
			if user, ok := e.Users[peerID]; ok {
				name := strings.TrimSpace(user.FirstName + " " + user.LastName)
				if name == "" {
					name = fmt.Sprintf("User %d", user.ID)
				}
				msg.SenderName = name
			} else if ch, ok := e.Channels[peerID]; ok {
				msg.SenderName = channelTitle(ch)
			} else if chat, ok := e.Chats[peerID]; ok {
				msg.SenderName = groupTitle(chat)
			}
		}
		if m, ok := raw.(*tg.Message); ok {
			if fwd, ok := m.GetFwdFrom(); ok {
				msg.Forward = forwardInfo(fwd, func(id int64) string {
					if user, ok := e.Users[id]; ok {
						name := strings.TrimSpace(user.FirstName + " " + user.LastName)
						if name != "" {
							return name
						}
					}
					if ch, ok := e.Channels[id]; ok {
						return channelTitle(ch)
					}
					if chat, ok := e.Chats[id]; ok {
						return groupTitle(chat)
					}
					return ""
				})
			}
		}
		if shouldSuppress(msg.ID) {
			return nil
		}
		// Bottom boundary of the update pipeline: a new message reached our
		// dispatcher and is about to be delivered. Pairs with the envelope log in
		// outboxHook to localize a long-idle stall (#119) — if envelopes arrive but
		// this never fires, the message is stuck inside updates.Manager.
		log.Debug("dispatcher: new message",
			zap.Int64("chat_id", msg.ChatID), zap.Int("msg_id", msg.ID), zap.Bool("out", msg.IsOut))
		select {
		case mustDeliver <- store.Event{Kind: store.EventNewMessage, Message: msg}:
		case <-ctx.Done():
		}
		return nil
	}

	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateNewMessage) error {
		return handleNewMessage(ctx, e, upd.Message)
	})

	// UpdateNewChannelMessage covers channels and supergroups; UpdateNewMessage
	// does not. Without this the chat list never bumps or increments unread for
	// supergroup/channel messages during live updates (issue #116).
	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateNewChannelMessage) error {
		return handleNewMessage(ctx, e, upd.Message)
	})

	// handleEditMessage converts an edited message and emits EventEditMessage.
	// Shared by UpdateEditMessage (users/basic groups) and UpdateEditChannelMessage
	// (channels/supergroups). No sender-name enrichment: an edit only changes the
	// message body, and the chat view re-renders from the already-stored sender.
	handleEditMessage := func(ctx context.Context, raw tg.MessageClass) error {
		peerID := extractPeerID(raw)
		msg, ok := convertMessage(raw, peerID)
		if !ok {
			return nil
		}
		log.Debug("dispatcher: edit message",
			zap.Int64("chat_id", msg.ChatID), zap.Int("msg_id", msg.ID))
		select {
		case mustDeliver <- store.Event{Kind: store.EventEditMessage, Message: msg}:
		case <-ctx.Done():
		}
		return nil
	}

	dispatcher.OnEditMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateEditMessage) error {
		return handleEditMessage(ctx, upd.Message)
	})

	dispatcher.OnEditChannelMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateEditChannelMessage) error {
		return handleEditMessage(ctx, upd.Message)
	})

	dispatcher.OnReadHistoryInbox(func(ctx context.Context, e tg.Entities, upd *tg.UpdateReadHistoryInbox) error {
		chatID := peerIDFromPeer(upd.Peer)
		if chatID == 0 {
			return nil
		}
		select {
		case mustDeliver <- store.Event{Kind: store.EventReadInbox, ChatID: chatID, ReadMaxID: upd.MaxID}:
		case <-ctx.Done():
		}
		return nil
	})

	dispatcher.OnReadChannelInbox(func(ctx context.Context, e tg.Entities, upd *tg.UpdateReadChannelInbox) error {
		select {
		case mustDeliver <- store.Event{Kind: store.EventReadInbox, ChatID: upd.ChannelID, ReadMaxID: upd.MaxID}:
		case <-ctx.Done():
		}
		return nil
	})

	dispatcher.OnMessageReactions(func(ctx context.Context, e tg.Entities, upd *tg.UpdateMessageReactions) error {
		chatID := peerIDFromPeer(upd.Peer)
		if chatID == 0 {
			return nil
		}
		reactions := convertReactions(upd.Reactions)
		select {
		case mustDeliver <- store.Event{
			Kind:      store.EventReactionsUpdate,
			ChatID:    chatID,
			MsgID:     upd.MsgID,
			Reactions: reactions,
		}:
		case <-ctx.Done():
		}
		return nil
	})

	dispatcher.OnDeleteMessages(func(ctx context.Context, e tg.Entities, upd *tg.UpdateDeleteMessages) error {
		select {
		case mustDeliver <- store.Event{Kind: store.EventDeleteMessages, MsgIDs: upd.Messages}:
		case <-ctx.Done():
		}
		return nil
	})

	dispatcher.OnDeleteChannelMessages(func(ctx context.Context, e tg.Entities, upd *tg.UpdateDeleteChannelMessages) error {
		select {
		case mustDeliver <- store.Event{Kind: store.EventDeleteMessages, ChatID: upd.ChannelID, MsgIDs: upd.Messages}:
		case <-ctx.Done():
		}
		return nil
	})

	dispatcher.OnUserStatus(func(ctx context.Context, e tg.Entities, upd *tg.UpdateUserStatus) error {
		_, online := upd.Status.(*tg.UserStatusOnline)
		select {
		case droppable <- store.Event{Kind: store.EventUserPresence, ChatID: upd.UserID, Online: online}:
		default:
			logDrop("user_presence")
		}
		return nil
	})

	dispatcher.OnUserTyping(func(ctx context.Context, e tg.Entities, upd *tg.UpdateUserTyping) error {
		action := convertTypingAction(upd.Action)
		select {
		case droppable <- store.Event{Kind: store.EventTyping, ChatID: upd.UserID, TypingAction: action}:
		default:
			logDrop("user_typing")
		}
		return nil
	})

	dispatcher.OnChatUserTyping(func(ctx context.Context, e tg.Entities, upd *tg.UpdateChatUserTyping) error {
		action := convertTypingAction(upd.Action)
		select {
		case droppable <- store.Event{Kind: store.EventTyping, ChatID: upd.ChatID, TypingAction: action}:
		default:
			logDrop("chat_typing")
		}
		return nil
	})

	dispatcher.OnChannelUserTyping(func(ctx context.Context, e tg.Entities, upd *tg.UpdateChannelUserTyping) error {
		action := convertTypingAction(upd.Action)
		select {
		case droppable <- store.Event{Kind: store.EventTyping, ChatID: upd.ChannelID, TypingAction: action}:
		default:
			logDrop("channel_typing")
		}
		return nil
	})

	// Mute/unmute performed on another device. Keeps the store's per-chat mute
	// flag in sync at runtime so notifications and the chat list stay correct
	// without a restart. NotifySettings is a generic (non-pts) update, so it is
	// safe to handle via the dispatcher.
	dispatcher.OnNotifySettings(func(ctx context.Context, e tg.Entities, upd *tg.UpdateNotifySettings) error {
		// We only track per-chat mute state; ignore global category defaults
		// (NotifyUsers/NotifyChats/NotifyBroadcasts) and forum-topic settings.
		np, ok := upd.Peer.(*tg.NotifyPeer)
		if !ok {
			return nil
		}
		chatID := peerIDFromPeer(np.Peer)
		if chatID == 0 {
			return nil
		}
		select {
		case mustDeliver <- store.Event{
			Kind:   store.EventMuteUpdate,
			ChatID: chatID,
			Muted:  mutedFromSettings(upd.NotifySettings),
		}:
		case <-ctx.Done():
		}
		return nil
	})

	// Draft changed on another device (or cleared server-side on send). Like
	// NotifySettings, updateDraftMessage is a generic (non-pts) update, so it is
	// safe to handle via the dispatcher without the outbox raw-hook workaround (#62).
	dispatcher.OnDraftMessage(func(ctx context.Context, e tg.Entities, upd *tg.UpdateDraftMessage) error {
		chatID := peerIDFromPeer(upd.Peer)
		if chatID == 0 {
			return nil
		}
		select {
		case mustDeliver <- store.Event{
			Kind:   store.EventDraftMessage,
			ChatID: chatID,
			Draft:  draftText(upd.Draft),
		}:
		case <-ctx.Done():
		}
		return nil
	})

	// OnReadHistoryOutbox / OnReadChannelOutbox are NOT registered here.
	// They are intercepted before pts-tracking in outboxHook (see client.go),
	// because pts gaps cause updates.Manager to silently drop these events.
}

func convertTypingAction(a tg.SendMessageActionClass) store.TypingAction {
	switch a.(type) {
	case *tg.SendMessageTypingAction:
		return store.TypingActionTyping
	case *tg.SendMessageRecordAudioAction:
		return store.TypingActionRecordAudio
	case *tg.SendMessageUploadAudioAction:
		return store.TypingActionUploadAudio
	case *tg.SendMessageRecordVideoAction:
		return store.TypingActionRecordVideo
	case *tg.SendMessageUploadVideoAction:
		return store.TypingActionUploadVideo
	case *tg.SendMessageUploadPhotoAction:
		return store.TypingActionUploadPhoto
	case *tg.SendMessageUploadDocumentAction:
		return store.TypingActionUploadDocument
	case *tg.SendMessageChooseStickerAction:
		return store.TypingActionChooseSticker
	case *tg.SendMessageRecordRoundAction:
		return store.TypingActionRecordRound
	case *tg.SendMessageCancelAction:
		return store.TypingActionCancel
	default:
		return store.TypingActionUnknown
	}
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
