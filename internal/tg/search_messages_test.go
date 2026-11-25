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

	req := buildSearchRequest(peer, 0, "hello", SearchPosition{}, 30)

	assert.Equal(t, peer, req.Peer)
	assert.Zero(t, req.OffsetID, "the first page starts at the newest message")
	assert.Equal(t, "hello", req.Q)
	assert.Equal(t, 30, req.Limit)
	assert.IsType(t, &tg.InputMessagesFilterEmpty{}, req.Filter)
	_, named := req.GetTopMsgID()
	assert.False(t, named, "a chat's search names no topic")
}

// A search of one topic names it, General included.
func TestBuildSearchRequest_InATopic(t *testing.T) {
	for _, topic := range []int{12, domain.GeneralTopicID} {
		req := buildSearchRequest(&tg.InputPeerChannel{ChannelID: 7}, topic, "hello", SearchPosition{}, 30)

		got, named := req.GetTopMsgID()
		assert.True(t, named)
		assert.Equal(t, topic, got)
	}
}

// The next page of one chat's search starts below the last message found:
// Telegram pages a chat's search by message id alone.
func TestBuildSearchRequest_NextPageStartsBelowTheLastFound(t *testing.T) {
	after := SearchPosition{ID: 512, Peer: domain.Peer{ID: 7, Type: domain.PeerSuperGroup, AccessHash: 70}, Rate: 3}

	req := buildSearchRequest(&tg.InputPeerChannel{ChannelID: 7, AccessHash: 70}, 0, "hello", after, 30)

	assert.Equal(t, 512, req.OffsetID)
	assert.Zero(t, req.AddOffset)
}

// A search of every chat looks in the main list: the archive is left out
// until it is asked for (#350).
func TestBuildSearchGlobalRequest_LooksInTheMainList(t *testing.T) {
	req := buildSearchGlobalRequest("hello", SearchPosition{}, 30)

	assert.Equal(t, "hello", req.Q)
	assert.Equal(t, 30, req.Limit)
	assert.IsType(t, &tg.InputMessagesFilterEmpty{}, req.Filter)
	assert.IsType(t, &tg.InputPeerEmpty{}, req.OffsetPeer, "the first page starts at the newest message")
	assert.Zero(t, req.OffsetRate)
	assert.Zero(t, req.OffsetID)
	folder, named := req.GetFolderID()
	assert.True(t, named, "the folder is named, or Telegram searches the archive too")
	assert.Equal(t, 0, folder)
}

// The next page of a search of every chat starts below the last message found,
// named by its rate, its chat and its id: ids are counted per chat there.
func TestBuildSearchGlobalRequest_NextPageStartsBelowTheLastFound(t *testing.T) {
	after := SearchPosition{Rate: 1731500000, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}, ID: 9}

	req := buildSearchGlobalRequest("hello", after, 30)

	assert.Equal(t, 1731500000, req.OffsetRate)
	assert.Equal(t, &tg.InputPeerChannel{ChannelID: 20, AccessHash: 200}, req.OffsetPeer)
	assert.Equal(t, 9, req.OffsetID)
	folder, named := req.GetFolderID()
	assert.True(t, named, "the next page stays in the main list")
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

	got := parseFound(res, true)

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

	got := parseFound(res, true)

	require.Len(t, got.Chats, 1)
	assert.Equal(t, int64(20), got.Chats[0].ID)
}

// A page of a search of every chat names where the next one starts: the rate
// Telegram gave, and the last message found, by its chat's address and its id.
func TestParseFound_EverywhereNamesTheNextPage(t *testing.T) {
	res := &tg.MessagesMessagesSlice{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello"},
			&tg.Message{ID: 9, PeerID: &tg.PeerChannel{ChannelID: 20}, FromID: &tg.PeerUser{UserID: 10}, Message: "hello there"},
		},
		Users: []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
		Chats: []tg.ChatClass{&tg.Channel{ID: 20, AccessHash: 200, Title: "Club", Megagroup: true}},
	}
	res.SetNextRate(1731500000)

	got := parseFound(res, true)

	require.NotNil(t, got.Next)
	assert.Equal(t, SearchPosition{
		Rate: 1731500000,
		Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200},
		ID:   9,
	}, *got.Next)
}

// A page of a search of every chat with no rate for the next one is the last:
// Telegram gives one only while there is more.
func TestParseFound_EverywhereWithNoNextRateIsTheLastPage(t *testing.T) {
	res := &tg.MessagesMessagesSlice{
		Messages: []tg.MessageClass{&tg.Message{ID: 40, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello"}},
		Users:    []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
	}

	got := parseFound(res, true)

	assert.Nil(t, got.Next)
}

// A page of one chat's search names the next by the last message's id: a
// supergroup or a channel answers with its own kind of page, a private chat or
// a group with a slice, and neither carries a rate.
func TestParseFound_InAChatNamesTheNextPageByTheLastID(t *testing.T) {
	pages := map[string]tg.MessagesMessagesClass{
		"channel": &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{
				&tg.Message{ID: 90, PeerID: &tg.PeerChannel{ChannelID: 20}, FromID: &tg.PeerUser{UserID: 10}, Message: "hello"},
				&tg.Message{ID: 61, PeerID: &tg.PeerChannel{ChannelID: 20}, FromID: &tg.PeerUser{UserID: 10}, Message: "hello again"},
			},
			Users: []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
			Chats: []tg.ChatClass{&tg.Channel{ID: 20, AccessHash: 200, Title: "Club", Megagroup: true}},
		},
		"slice": &tg.MessagesMessagesSlice{
			Messages: []tg.MessageClass{
				&tg.Message{ID: 90, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello"},
				&tg.Message{ID: 61, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello again"},
			},
			Users: []tg.UserClass{&tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}},
		},
	}
	for name, res := range pages {
		t.Run(name, func(t *testing.T) {
			got := parseFound(res, false)

			require.NotNil(t, got.Next)
			assert.Equal(t, 61, got.Next.ID)
			assert.Zero(t, got.Next.Rate)
		})
	}
}

// An answer that is not a page holds everything found, and an empty page is
// past the last: neither has a next page.
func TestParseFound_NoNextPage(t *testing.T) {
	found := &tg.Message{ID: 90, PeerID: &tg.PeerUser{UserID: 10}, Message: "hello"}
	ann := &tg.User{ID: 10, AccessHash: 100, FirstName: "Ann"}
	empty := &tg.MessagesMessagesSlice{}
	empty.SetNextRate(1731500000)
	answers := map[string]tg.MessagesMessagesClass{
		"whole":         &tg.MessagesMessages{Messages: []tg.MessageClass{found}, Users: []tg.UserClass{ann}},
		"empty slice":   empty,
		"empty channel": &tg.MessagesChannelMessages{},
	}
	for name, res := range answers {
		t.Run(name, func(t *testing.T) {
			for _, everywhere := range []bool{false, true} {
				assert.Nil(t, parseFound(res, everywhere).Next, "everywhere=%v", everywhere)
			}
		})
	}
}
