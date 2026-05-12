package tg

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) GetDialogs(ctx context.Context) ([]store.Chat, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	c.log.Debug("GetDialogs start")
	var chats []store.Chat
	err := WithRetry(ctx, func() error {
		result, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      100,
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
		})
		if err != nil {
			c.log.Error("MessagesGetDialogs failed", zap.Error(err))
			return err
		}
		chats = c.parseDialogs(result)
		c.log.Debug("GetDialogs done", zap.Int("count", len(chats)))
		return nil
	})
	return chats, err
}

type dialogMeta struct {
	pinned    bool
	lastMsgAt time.Time
}

func (c *GotdClient) parseDialogs(result tg.MessagesDialogsClass) []store.Chat {
	var dialogs []tg.DialogClass
	var msgs []tg.MessageClass
	var users []tg.UserClass
	var chatsRaw []tg.ChatClass

	switch v := result.(type) {
	case *tg.MessagesDialogs:
		dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
	case *tg.MessagesDialogsSlice:
		dialogs, msgs, users, chatsRaw = v.Dialogs, v.Messages, v.Users, v.Chats
	default:
		return nil
	}

	// Build message date index: msgID → date
	msgDate := make(map[int]time.Time, len(msgs))
	for _, raw := range msgs {
		if m, ok := raw.(*tg.Message); ok {
			msgDate[m.ID] = time.Unix(int64(m.Date), 0)
		}
	}

	// Build dialog meta index: peerID → meta
	meta := make(map[int64]dialogMeta, len(dialogs))
	for _, d := range dialogs {
		dlg, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		id := peerIDFromPeer(dlg.Peer)
		if id == 0 {
			continue
		}
		meta[id] = dialogMeta{
			pinned:    dlg.Pinned,
			lastMsgAt: msgDate[dlg.TopMessage],
		}
	}

	// Build peer lookup maps
	userMap := make(map[int64]*tg.User, len(users))
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	chatMap := make(map[int64]tg.ChatClass, len(chatsRaw))
	for _, ch := range chatsRaw {
		switch v := ch.(type) {
		case *tg.Chat:
			chatMap[v.ID] = v
		case *tg.Channel:
			chatMap[v.ID] = v
		}
	}

	// Preserve dialog order from server, apply meta
	out := make([]store.Chat, 0, len(dialogs))
	for _, d := range dialogs {
		dlg, ok := d.(*tg.Dialog)
		if !ok {
			continue
		}
		id := peerIDFromPeer(dlg.Peer)
		if id == 0 {
			continue
		}
		m := meta[id]
		var chat store.Chat
		var converted bool
		switch dlg.Peer.(type) {
		case *tg.PeerUser:
			if u, ok := userMap[id]; ok && !u.Bot && !u.Self {
				chat, converted = convertUser(u)
			}
		case *tg.PeerChat:
			if ch, ok := chatMap[id].(*tg.Chat); ok {
				chat, converted = convertGroupChat(ch)
			}
		case *tg.PeerChannel:
			if ch, ok := chatMap[id].(*tg.Channel); ok {
				chat, converted = convertChannel(ch)
			}
		}
		if !converted {
			continue
		}
		chat.Pinned = m.pinned
		chat.UnreadCount = dlg.UnreadCount
		chat.LastMessage = &store.Message{Date: m.lastMsgAt}
		c.cachePeer(chat.Peer)
		out = append(out, chat)
	}

	// Sort: pinned first (preserving server order within group), then by last message date desc
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].LastMessage.Date.After(out[j].LastMessage.Date)
	})

	return out
}

func peerIDFromPeer(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return p.ChatID
	case *tg.PeerChannel:
		return p.ChannelID
	}
	return 0
}

func (c *GotdClient) cachePeer(p store.Peer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.peers[p.ID] = p
}

func convertUser(u *tg.User) (store.Chat, bool) {
	if u == nil {
		return store.Chat{}, false
	}
	name := strings.TrimSpace(u.FirstName + " " + u.LastName)
	if name == "" {
		name = fmt.Sprintf("User %d", u.ID)
	}
	return store.Chat{
		ID:    u.ID,
		Title: name,
		Peer:  store.Peer{ID: u.ID, Type: store.PeerUser, AccessHash: u.AccessHash},
	}, true
}

func convertGroupChat(ch *tg.Chat) (store.Chat, bool) {
	if ch == nil {
		return store.Chat{}, false
	}
	return store.Chat{
		ID:    ch.ID,
		Title: ch.Title,
		Peer:  store.Peer{ID: ch.ID, Type: store.PeerGroup},
	}, true
}

func convertChannel(ch *tg.Channel) (store.Chat, bool) {
	if ch == nil {
		return store.Chat{}, false
	}
	return store.Chat{
		ID:    ch.ID,
		Title: ch.Title,
		Peer:  store.Peer{ID: ch.ID, Type: store.PeerChannel, AccessHash: ch.AccessHash},
	}, true
}
