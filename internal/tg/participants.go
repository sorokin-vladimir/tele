package tg

import (
	"context"
	"strings"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

const participantsLimit = 200

// buildChatMembers maps Telegram users to mention candidates, skipping deleted
// users and bots. DisplayName is the trimmed First+Last.
func buildChatMembers(users []tg.UserClass) []store.ChatMember {
	out := make([]store.ChatMember, 0, len(users))
	for _, u := range users {
		user, ok := u.(*tg.User)
		if !ok || user.Deleted || user.Bot {
			continue
		}
		name := strings.TrimSpace(user.FirstName + " " + user.LastName)
		out = append(out, store.ChatMember{
			UserID:      user.ID,
			Username:    user.Username,
			DisplayName: name,
			AccessHash:  user.AccessHash,
		})
	}
	return out
}

// GetParticipants returns mention candidates for a group or channel. Basic
// groups use messages.getFullChat; channels/supergroups use
// channels.getParticipants (first page, capped). Private chats return nil.
func (c *GotdClient) GetParticipants(ctx context.Context, peer store.Peer) ([]store.ChatMember, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}
	var members []store.ChatMember
	err = WithRetry(ctx, func() error {
		if isChannelPeer(peer) {
			res, err := api.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
				Channel: inputChannel(peer),
				Filter:  &tg.ChannelParticipantsRecent{},
				Limit:   participantsLimit,
			})
			if err != nil {
				c.log.Error("ChannelsGetParticipants failed", zap.Error(err))
				return err
			}
			if p, ok := res.(*tg.ChannelsChannelParticipants); ok {
				members = buildChatMembers(p.Users)
			}
			return nil
		}
		// Basic group (chat_id): getFullChat carries the user list.
		full, err := api.MessagesGetFullChat(ctx, peer.ID)
		if err != nil {
			c.log.Error("MessagesGetFullChat failed", zap.Error(err))
			return err
		}
		members = buildChatMembers(full.Users)
		return nil
	})
	return members, err
}
