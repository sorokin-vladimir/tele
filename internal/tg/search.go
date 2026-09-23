package tg

import (
	"context"
	"strings"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// SearchContacts queries Telegram (contacts.search) for users matching q,
// returning matches as domain.Chat with valid peers (access hashes). Phase 1:
// users only — groups/channels are filtered out (issue #82).
func (c *GotdClient) SearchContacts(ctx context.Context, q string, limit int) ([]domain.Chat, error) {
	q = strings.TrimPrefix(strings.TrimSpace(q), "@")
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}
	found, err := api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: q, Limit: limit})
	if err != nil {
		return nil, err
	}
	return usersFromContactsFound(found, limit), nil
}

// usersFromContactsFound maps the user peers of a contacts.search response to
// domain.Chat. MyResults (exact/contact matches) are listed before global
// Results; non-user peers, self, and duplicates are dropped, and the count is
// capped at limit.
func usersFromContactsFound(found *tg.ContactsFound, limit int) []domain.Chat {
	if found == nil {
		return nil
	}
	userMap := make(map[int64]*tg.User, len(found.Users))
	for _, u := range found.Users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.ID] = user
		}
	}
	seen := make(map[int64]struct{})
	out := make([]domain.Chat, 0, limit)
	add := func(peers []tg.PeerClass) {
		for _, p := range peers {
			pu, ok := p.(*tg.PeerUser)
			if !ok {
				continue
			}
			if _, dup := seen[pu.UserID]; dup {
				continue
			}
			user := userMap[pu.UserID]
			chat, ok := convertUser(user)
			if !ok || user.Self {
				continue
			}
			seen[pu.UserID] = struct{}{}
			out = append(out, chat)
			if len(out) >= limit {
				return
			}
		}
	}
	add(found.MyResults)
	if len(out) < limit {
		add(found.Results)
	}
	return out
}
