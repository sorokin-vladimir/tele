package tg

import (
	"context"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// ResolveUsername asks Telegram (contacts.resolveUsername) which user, bot or
// channel holds username, given without the @. One nobody holds is
// PeerNotFound (#181).
func (c *GotdClient) ResolveUsername(ctx context.Context, username string) (domain.Chat, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return domain.Chat{}, err
	}
	res, err := api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{Username: username})
	if err != nil {
		return domain.Chat{}, err
	}
	chat, ok := chatFromResolved(res)
	if !ok {
		return domain.Chat{}, &telerr.Error{Kind: telerr.PeerNotFound, Op: "contacts.resolveUsername"}
	}
	return chat, nil
}

// chatFromResolved is the chat a contacts.resolveUsername answer names: the
// entity its peer points at, among those it carries.
func chatFromResolved(res *tg.ContactsResolvedPeer) (domain.Chat, bool) {
	switch p := res.Peer.(type) {
	case *tg.PeerUser:
		for _, u := range res.Users {
			if user, ok := u.(*tg.User); ok && user.ID == p.UserID {
				return convertUser(user)
			}
		}
	case *tg.PeerChannel:
		for _, ch := range res.Chats {
			if channel, ok := ch.(*tg.Channel); ok && channel.ID == p.ChannelID {
				return convertChannel(channel)
			}
		}
	}
	return domain.Chat{}, false
}
