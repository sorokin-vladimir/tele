package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// serverSearch is Telegram answering message searches with found, and the
// history around a message with around, recording what it was asked.
type serverSearch struct {
	Connection
	found  internaltg.FoundMessages
	around []domain.Message

	searchedIn    *domain.Peer
	searchedTopic int
	searchedText  string
	searchedLimit int
	searched      bool

	aroundIn    domain.Peer
	aroundTopic int
	aroundID    int
	aroundLimit int
}

func (s *serverSearch) Updates() <-chan store.Event { return nil }

func (s *serverSearch) SearchMessages(_ context.Context, peer *domain.Peer, topicID int, text string, limit int) (internaltg.FoundMessages, error) {
	s.searched = true
	s.searchedIn, s.searchedTopic, s.searchedText, s.searchedLimit = peer, topicID, text, limit
	return s.found, nil
}

func (s *serverSearch) GetHistoryAround(_ context.Context, peer domain.Peer, topicID, id, limit int) ([]domain.Message, error) {
	s.aroundIn, s.aroundTopic, s.aroundID, s.aroundLimit = peer, topicID, id, limit
	return s.around, nil
}

var adaChat = domain.Chat{ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser, AccessHash: 11}}

func newSearchOwner(t *testing.T, server *serverSearch, chats ...domain.Chat) *Owner {
	t.Helper()
	o, s := newOwnerWithClient(t, server)
	for _, c := range chats {
		s.Store().SetChat(c)
	}
	return o
}

// A search of one chat asks Telegram about that chat, and each message found
// carries the chat's title (#348).
func TestSearchMessages_InAChatAsksThatChat(t *testing.T) {
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{{ID: 7, ChatID: 1, Text: "hello"}},
	}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 1}, "hello", 30)

	require.NoError(t, err)
	require.NotNil(t, server.searchedIn)
	assert.Equal(t, adaChat.Peer, *server.searchedIn)
	assert.Equal(t, 0, server.searchedTopic)
	assert.Equal(t, "hello", server.searchedText)
	assert.Equal(t, 30, server.searchedLimit)
	require.Len(t, got, 1)
	assert.Equal(t, 7, got[0].Message.ID)
	assert.Equal(t, "Ada", got[0].Chat)
}

// A search of one topic names it.
func TestSearchMessages_InATopicNamesIt(t *testing.T) {
	server := &serverSearch{}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 2, TopicID: 12}, "hello", 30)

	require.NoError(t, err)
	assert.Equal(t, 12, server.searchedTopic)
}

// A search of every chat names none, and a chat found that the owner does not
// hold is named by the title Telegram gave it and kept as an address, so a
// command can read it next by its id alone (#348).
func TestSearchMessages_EverywhereKeepsTheAddressesFound(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{{ID: 9, ChatID: 20}, {ID: 4, ChatID: 1}},
		Chats:    []domain.Chat{club, adaChat},
	}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", 30)

	require.NoError(t, err)
	assert.True(t, server.searched)
	assert.Nil(t, server.searchedIn, "every chat is asked with no chat named")
	require.Len(t, got, 2)
	assert.Equal(t, []string{"Club", "Ada"}, []string{got[0].Chat, got[1].Chat}, "newest first, as Telegram answered")
	peer, err := o.peer(20)
	require.NoError(t, err)
	assert.Equal(t, club.Peer, peer)
}

// A chat the owner cannot address is not searched at all.
func TestSearchMessages_AChatNobodyHoldsIsNotFound(t *testing.T) {
	server := &serverSearch{}
	o := newSearchOwner(t, server)

	_, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 99}, "hello", 30)

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.False(t, server.searched)
}

// The history around a message is Telegram's answer, and nothing of it is
// kept: the store would hold it with the history between missing (#348).
func TestHistoryAround_AsksTelegramAndKeepsNothing(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 48, ChatID: 1}, {ID: 50, ChatID: 1}, {ID: 52, ChatID: 1}}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.HistoryAround(context.Background(), oneChat, 50, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{48, 50, 52}, ids(got))
	assert.Equal(t, adaChat.Peer, server.aroundIn)
	assert.Equal(t, 50, server.aroundID)
	assert.Equal(t, 5, server.aroundLimit)
	assert.Empty(t, o.state.Store().Messages(oneChat))
}

// In a forum it is one topic's history.
func TestHistoryAround_InATopic(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 50, ChatID: 2, TopicID: 12}}}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.HistoryAround(context.Background(), domain.HistoryKey{ChatID: 2, TopicID: 12}, 50, 5)

	require.NoError(t, err)
	assert.Equal(t, 12, server.aroundTopic)
}

// A message Telegram does not return is not there.
func TestHistoryAround_AMessageNotThereIsNotFound(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 48, ChatID: 1}, {ID: 52, ChatID: 1}}}
	o := newSearchOwner(t, server, adaChat)

	_, err := o.HistoryAround(context.Background(), oneChat, 50, 5)

	assert.Equal(t, telerr.NotFound, telerr.Of(err))
}

// A forum's messages are its topics', as for History.
func TestHistoryAround_AForumNeedsATopic(t *testing.T) {
	server := &serverSearch{}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.HistoryAround(context.Background(), domain.HistoryKey{ChatID: 2}, 50, 5)

	assert.Equal(t, telerr.NotFound, telerr.Of(err))
}
