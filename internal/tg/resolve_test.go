package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// A resolved username is the user, bot or channel its peer names, with the
// address that came with it (#181).
func TestChatFromResolved_TheUserOrChannelThePeerNames(t *testing.T) {
	user, ok := chatFromResolved(&tg.ContactsResolvedPeer{
		Peer:  &tg.PeerUser{UserID: 42},
		Users: []tg.UserClass{&tg.User{ID: 41, FirstName: "Other"}, &tg.User{ID: 42, FirstName: "Bob", Username: "bob", AccessHash: 4242}},
	})
	require.True(t, ok)
	assert.Equal(t, int64(42), user.ID)
	assert.Equal(t, domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242}, user.Peer)

	channel, ok := chatFromResolved(&tg.ContactsResolvedPeer{
		Peer:  &tg.PeerChannel{ChannelID: 5},
		Chats: []tg.ChatClass{&tg.Channel{ID: 5, Title: "News", Username: "news", AccessHash: 55, Broadcast: true}},
	})
	require.True(t, ok)
	assert.Equal(t, domain.Peer{ID: 5, Type: domain.PeerChannel, AccessHash: 55}, channel.Peer)
}

// A peer the answer carries no entity for resolves to nothing.
func TestChatFromResolved_APeerWithNoEntityIsNothing(t *testing.T) {
	_, ok := chatFromResolved(&tg.ContactsResolvedPeer{Peer: &tg.PeerUser{UserID: 42}})

	assert.False(t, ok)
}
