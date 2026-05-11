package tg

import (
	"context"
	"fmt"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func (c *GotdClient) GetDialogs(ctx context.Context) ([]store.Chat, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	var chats []store.Chat
	err := WithRetry(ctx, func() error {
		result, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			Limit:      100,
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
		})
		if err != nil {
			return err
		}
		chats = c.parseDialogs(result)
		return nil
	})
	return chats, err
}

func (c *GotdClient) parseDialogs(result tg.MessagesDialogsClass) []store.Chat {
	var users []tg.UserClass
	var chatsRaw []tg.ChatClass

	switch v := result.(type) {
	case *tg.MessagesDialogs:
		users = v.Users
		chatsRaw = v.Chats
	case *tg.MessagesDialogsSlice:
		users = v.Users
		chatsRaw = v.Chats
	default:
		return nil
	}

	out := make([]store.Chat, 0, len(users)+len(chatsRaw))

	for _, u := range users {
		user, ok := u.(*tg.User)
		if !ok || user.Bot || user.Self {
			continue
		}
		if chat, ok := convertUser(user); ok {
			c.cachePeer(chat.Peer)
			out = append(out, chat)
		}
	}

	for _, raw := range chatsRaw {
		switch v := raw.(type) {
		case *tg.Chat:
			if chat, ok := convertGroupChat(v); ok {
				c.cachePeer(chat.Peer)
				out = append(out, chat)
			}
		case *tg.Channel:
			if chat, ok := convertChannel(v); ok {
				c.cachePeer(chat.Peer)
				out = append(out, chat)
			}
		}
	}

	return out
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
