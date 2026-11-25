package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// A person known by address alone comes back from users.getUsers whole: the
// name and the address with it (#348).
func TestChatFromUsers_ThePersonAsked(t *testing.T) {
	users := []tg.UserClass{
		&tg.User{ID: 9, FirstName: "Someone else"},
		&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann", Username: "ann"},
	}

	got, ok := chatFromUsers(users, 10)

	require.True(t, ok)
	assert.Equal(t, "Ann", got.Title)
	assert.Equal(t, "ann", got.Username)
	assert.Equal(t, domain.Peer{ID: 10, Type: domain.PeerUser, AccessHash: 100}, got.Peer)
}

// A group or channel comes back from channels.getChannels or messages.getChats
// with its title, and a forum says it is one, so a command can ask for a topic.
func TestChatFromChats_TheChatAskedAndWhetherItIsAForum(t *testing.T) {
	res := &tg.MessagesChats{Chats: []tg.ChatClass{
		&tg.Chat{ID: 5, Title: "Old group"},
		&tg.Channel{ID: 20, AccessHash: 200, Title: "Club", Megagroup: true, Forum: true},
	}}

	club, ok := chatFromChats(res, 20)
	require.True(t, ok)
	assert.Equal(t, "Club", club.Title)
	assert.True(t, club.IsForum)
	assert.Equal(t, domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}, club.Peer)

	group, ok := chatFromChats(&tg.MessagesChatsSlice{Chats: res.Chats}, 5)
	require.True(t, ok)
	assert.Equal(t, "Old group", group.Title)
	assert.Equal(t, domain.PeerGroup, group.Peer.Type)
}

// An answer that does not hold the chat asked is no chat: one the account may
// no longer see is not mistaken for another.
func TestChatFrom_NotInTheAnswer(t *testing.T) {
	_, ok := chatFromUsers([]tg.UserClass{&tg.UserEmpty{ID: 10}}, 10)
	assert.False(t, ok)

	_, ok = chatFromChats(&tg.MessagesChats{Chats: []tg.ChatClass{&tg.ChannelForbidden{ID: 20}}}, 20)
	assert.False(t, ok)
}
