package tg

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestConvertUser_ToChat(t *testing.T) {
	user := &tg.User{ID: 100, FirstName: "Alice", LastName: "B", AccessHash: 42}
	chat, ok := convertUser(user)
	require.True(t, ok)
	assert.Equal(t, int64(100), chat.ID)
	assert.Equal(t, "Alice B", chat.Title)
	assert.Equal(t, store.PeerUser, chat.Peer.Type)
	assert.Equal(t, int64(42), chat.Peer.AccessHash)
}

func TestConvertChat_ToChat(t *testing.T) {
	c := &tg.Chat{ID: 200, Title: "My Group"}
	chat, ok := convertGroupChat(c)
	require.True(t, ok)
	assert.Equal(t, int64(200), chat.ID)
	assert.Equal(t, "My Group", chat.Title)
	assert.Equal(t, store.PeerGroup, chat.Peer.Type)
}

func TestConvertChannel_ToChat(t *testing.T) {
	ch := &tg.Channel{ID: 300, Title: "News", AccessHash: 99}
	chat, ok := convertChannel(ch)
	require.True(t, ok)
	assert.Equal(t, int64(300), chat.ID)
	assert.Equal(t, store.PeerChannel, chat.Peer.Type)
	assert.Equal(t, int64(99), chat.Peer.AccessHash)
}

func TestConvertUser_Bot(t *testing.T) {
	user := &tg.User{ID: 101, FirstName: "MyBot", Bot: true, AccessHash: 55}
	chat, ok := convertUser(user)
	require.True(t, ok)
	assert.Equal(t, int64(101), chat.ID)
	assert.Equal(t, "MyBot", chat.Title)
	assert.Equal(t, store.PeerUser, chat.Peer.Type)
	assert.Equal(t, int64(55), chat.Peer.AccessHash)
}

func TestConvertUser_Self(t *testing.T) {
	user := &tg.User{ID: 1, FirstName: "Me", Self: true, AccessHash: 7}
	chat, ok := convertUser(user)
	require.True(t, ok)
	assert.Equal(t, "Saved Messages", chat.Title)
	assert.Equal(t, store.PeerUser, chat.Peer.Type)
}

func TestParseDialogs_IncludesBots(t *testing.T) {
	c := &GotdClient{peers: make(map[int64]store.Peer)}
	bot := &tg.User{ID: 42, FirstName: "CoolBot", Bot: true, AccessHash: 1}
	dialog := &tg.Dialog{
		Peer:       &tg.PeerUser{UserID: 42},
		TopMessage: 1,
	}
	msg := &tg.Message{ID: 1, Date: int(time.Now().Unix())}
	result := &tg.MessagesDialogs{
		Dialogs:  []tg.DialogClass{dialog},
		Messages: []tg.MessageClass{msg},
		Users:    []tg.UserClass{bot},
	}
	chats := c.parseDialogs(result)
	require.Len(t, chats, 1)
	assert.Equal(t, "CoolBot", chats[0].Title)
}

func TestParseDialogs_IncludesSavedMessages(t *testing.T) {
	c := &GotdClient{peers: make(map[int64]store.Peer)}
	self := &tg.User{ID: 1, FirstName: "Me", Self: true, AccessHash: 7}
	dialog := &tg.Dialog{
		Peer:       &tg.PeerUser{UserID: 1},
		TopMessage: 1,
	}
	msg := &tg.Message{ID: 1, Date: int(time.Now().Unix())}
	result := &tg.MessagesDialogs{
		Dialogs:  []tg.DialogClass{dialog},
		Messages: []tg.MessageClass{msg},
		Users:    []tg.UserClass{self},
	}
	chats := c.parseDialogs(result)
	require.Len(t, chats, 1)
	assert.Equal(t, "Saved Messages", chats[0].Title)
}

func TestParseDialogs_UnreadCount(t *testing.T) {
	c := &GotdClient{peers: make(map[int64]store.Peer)}
	user := &tg.User{ID: 7, FirstName: "Bob", AccessHash: 1}
	dialog := &tg.Dialog{
		Peer:        &tg.PeerUser{UserID: 7},
		TopMessage:  1,
		UnreadCount: 5,
	}
	msg := &tg.Message{ID: 1, Date: int(time.Now().Unix())}
	result := &tg.MessagesDialogs{
		Dialogs:  []tg.DialogClass{dialog},
		Messages: []tg.MessageClass{msg},
		Users:    []tg.UserClass{user},
	}
	chats := c.parseDialogs(result)
	require.Len(t, chats, 1)
	assert.Equal(t, 5, chats[0].UnreadCount)
}
