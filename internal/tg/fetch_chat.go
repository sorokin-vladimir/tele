package tg

import (
	"context"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// FetchChat asks Telegram for the chat peer addresses: a person, a group or a
// channel the account can reach without having a dialog with it, such as one
// a message search found. One Telegram does not answer with is PeerNotFound
// (#348).
func (c *GotdClient) FetchChat(ctx context.Context, peer domain.Peer) (domain.Chat, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return domain.Chat{}, err
	}
	var chat domain.Chat
	var ok bool
	err = WithRetry(ctx, func() error {
		switch peer.Type {
		case domain.PeerUser:
			users, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{UserID: peer.ID, AccessHash: peer.AccessHash}})
			if err != nil {
				return err
			}
			chat, ok = chatFromUsers(users, peer.ID)
		case domain.PeerGroup:
			res, err := api.MessagesGetChats(ctx, []int64{peer.ID})
			if err != nil {
				return err
			}
			chat, ok = chatFromChats(res, peer.ID)
		case domain.PeerChannel, domain.PeerSuperGroup:
			res, err := api.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel(peer)})
			if err != nil {
				return err
			}
			chat, ok = chatFromChats(res, peer.ID)
		}
		return nil
	})
	if err != nil {
		c.log.Error("fetching a chat failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
		return domain.Chat{}, err
	}
	if !ok {
		return domain.Chat{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "fetch chat"}
	}
	return chat, nil
}

// chatFromUsers is the person id among users, when the answer holds them.
func chatFromUsers(users []tg.UserClass, id int64) (domain.Chat, bool) {
	for _, u := range users {
		if user, ok := u.(*tg.User); ok && user.ID == id {
			return convertUser(user)
		}
	}
	return domain.Chat{}, false
}

// chatFromChats is the group or channel id in res, when the answer holds it.
func chatFromChats(res tg.MessagesChatsClass, id int64) (domain.Chat, bool) {
	var chats []tg.ChatClass
	switch v := res.(type) {
	case *tg.MessagesChats:
		chats = v.Chats
	case *tg.MessagesChatsSlice:
		chats = v.Chats
	}
	for _, raw := range chats {
		switch ch := raw.(type) {
		case *tg.Chat:
			if ch.ID == id {
				return convertGroupChat(ch)
			}
		case *tg.Channel:
			if ch.ID == id {
				return convertChannel(ch)
			}
		}
	}
	return domain.Chat{}, false
}
