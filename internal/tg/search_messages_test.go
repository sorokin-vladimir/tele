package tg

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// A search of one chat asks that chat for the text, with no filter, and names
// no topic.
func TestBuildSearchRequest_InAChat(t *testing.T) {
	peer := &tg.InputPeerChannel{ChannelID: 7, AccessHash: 70}

	req := buildSearchRequest(peer, 0, "hello", 30)

	assert.Equal(t, peer, req.Peer)
	assert.Equal(t, "hello", req.Q)
	assert.Equal(t, 30, req.Limit)
	assert.IsType(t, &tg.InputMessagesFilterEmpty{}, req.Filter)
	_, named := req.GetTopMsgID()
	assert.False(t, named, "a chat's search names no topic")
}

// A search of one topic names it, General included.
func TestBuildSearchRequest_InATopic(t *testing.T) {
	for _, topic := range []int{12, domain.GeneralTopicID} {
		req := buildSearchRequest(&tg.InputPeerChannel{ChannelID: 7}, topic, "hello", 30)

		got, named := req.GetTopMsgID()
		assert.True(t, named)
		assert.Equal(t, topic, got)
	}
}

// A search of every chat looks in the main list: the archive is left out
// until it is asked for (#350).
func TestBuildSearchGlobalRequest_LooksInTheMainList(t *testing.T) {
	req := buildSearchGlobalRequest("hello", 30)

	assert.Equal(t, "hello", req.Q)
	assert.Equal(t, 30, req.Limit)
	assert.IsType(t, &tg.InputMessagesFilterEmpty{}, req.Filter)
	assert.IsType(t, &tg.InputPeerEmpty{}, req.OffsetPeer)
	folder, named := req.GetFolderID()
	assert.True(t, named, "the folder is named, or Telegram searches the archive too")
	assert.Equal(t, 0, folder)
}

// The history around a message holds it, up to half the rest newer than it
// and the other half older: Telegram pages back from offset_id, and a negative
// add_offset moves that many of the page past it.
func TestBuildAroundRequest_PutsTheMessageInTheMiddle(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 5}

	req := buildAroundRequest(peer, 100, 21)

	assert.Equal(t, peer, req.Peer)
	assert.Equal(t, 100, req.OffsetID)
	assert.Equal(t, -11, req.AddOffset, "ten newer and the message itself")
	assert.Equal(t, 21, req.Limit)
}

// The same holds in one topic of a forum.
func TestBuildAroundRepliesRequest_PutsTheMessageInTheMiddleOfItsTopic(t *testing.T) {
	peer := &tg.InputPeerChannel{ChannelID: 7}

	req := buildAroundRepliesRequest(peer, 12, 100, 20)

	assert.Equal(t, 12, req.MsgID)
	assert.Equal(t, 100, req.OffsetID)
	assert.Equal(t, -10, req.AddOffset)
	assert.Equal(t, 20, req.Limit)
}

// A search of every chat answers with messages from many chats: each is put in
// the chat it was sent in, newest first as Telegram sends them, and the chats
// come back with their addresses, so the owner can name them afterwards.
func TestParseFound_EachMessageInItsOwnChat(t *testing.T) {
	date := int(time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC).Unix())
	res := &tg.MessagesMessagesSlice{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 9, PeerID: &tg.PeerChannel{ChannelID: 20}, FromID: &tg.PeerUser{UserID: 10}, Message: "hello there", Date: date + 60},
			&tg.Message{ID: 4, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello", Date: date},
		},
		Users: []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
		Chats: []tg.ChatClass{&tg.Channel{ID: 20, AccessHash: 200, Title: "Club", Megagroup: true}},
	}

	got := parseFound(res)

	require.Len(t, got.Messages, 2)
	assert.Equal(t, 9, got.Messages[0].ID, "newest first")
	assert.Equal(t, int64(20), got.Messages[0].ChatID)
	assert.Equal(t, "Ann", got.Messages[0].SenderName)
	assert.Equal(t, int64(10), got.Messages[1].ChatID)
	assert.Equal(t, "Ann", got.Messages[1].SenderName, "a private chat's message is from the person")

	byID := map[int64]domain.Chat{}
	for _, c := range got.Chats {
		byID[c.ID] = c
	}
	require.Len(t, byID, 2)
	assert.Equal(t, domain.Peer{ID: 10, Type: domain.PeerUser, AccessHash: 100}, byID[10].Peer)
	assert.Equal(t, "Ann", byID[10].Title)
	assert.Equal(t, domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}, byID[20].Peer)
	assert.Equal(t, "Club", byID[20].Title)
}

// A chat the answer names but no message is in is not one of the found
// chats: a sender's user is not a chat of the answer.
func TestParseFound_OnlyTheChatsMessagesAreIn(t *testing.T) {
	res := &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 9, PeerID: &tg.PeerChannel{ChannelID: 20}, FromID: &tg.PeerUser{UserID: 10}, Message: "hi"},
		},
		Users: []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
		Chats: []tg.ChatClass{&tg.Channel{ID: 20, AccessHash: 200, Title: "Club"}},
	}

	got := parseFound(res)

	require.Len(t, got.Chats, 1)
	assert.Equal(t, int64(20), got.Chats[0].ID)
}
