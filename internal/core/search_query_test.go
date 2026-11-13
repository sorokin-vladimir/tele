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
	searchedAfter internaltg.SearchPosition
	searched      bool

	aroundIn    domain.Peer
	aroundTopic int
	aroundID    int
	aroundLimit int
	aroundErr   error
	aroundAsked bool

	// chats is what FetchChat answers with, by id.
	chats map[int64]domain.Chat
}

func (s *serverSearch) Updates() <-chan store.Event { return nil }

func (s *serverSearch) FetchChat(_ context.Context, peer domain.Peer) (domain.Chat, error) {
	c, ok := s.chats[peer.ID]
	if !ok {
		return domain.Chat{}, &telerr.Error{Kind: telerr.PeerNotFound}
	}
	return c, nil
}

func (s *serverSearch) SearchMessages(_ context.Context, peer *domain.Peer, topicID int, text string, after internaltg.SearchPosition, limit int) (internaltg.FoundMessages, error) {
	s.searched = true
	s.searchedIn, s.searchedTopic, s.searchedText, s.searchedLimit = peer, topicID, text, limit
	s.searchedAfter = after
	return s.found, nil
}

func (s *serverSearch) GetHistoryAround(_ context.Context, peer domain.Peer, topicID, id, limit int) ([]domain.Message, error) {
	s.aroundAsked = true
	s.aroundIn, s.aroundTopic, s.aroundID, s.aroundLimit = peer, topicID, id, limit
	if s.aroundErr != nil {
		return nil, s.aroundErr
	}
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

	got, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 1}, "hello", project.SearchPosition{}, 30)

	require.NoError(t, err)
	require.NotNil(t, server.searchedIn)
	assert.Equal(t, adaChat.Peer, *server.searchedIn)
	assert.Equal(t, 0, server.searchedTopic)
	assert.Equal(t, "hello", server.searchedText)
	assert.Equal(t, 30, server.searchedLimit)
	require.Len(t, got.Found, 1)
	assert.Equal(t, 7, got.Found[0].Message.ID)
	assert.Equal(t, "Ada", got.Found[0].Chat)
	assert.Nil(t, got.Next, "Telegram named no next page")
}

// A search of one topic names it.
func TestSearchMessages_InATopicNamesIt(t *testing.T) {
	server := &serverSearch{}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 2, TopicID: 12}, "hello", project.SearchPosition{}, 30)

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

	got, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{}, 30)

	require.NoError(t, err)
	assert.True(t, server.searched)
	assert.Nil(t, server.searchedIn, "every chat is asked with no chat named")
	require.Len(t, got.Found, 2)
	assert.Equal(t, []string{"Club", "Ada"}, []string{got.Found[0].Chat, got.Found[1].Chat}, "newest first, as Telegram answered")
	peer, err := o.peer(20)
	require.NoError(t, err)
	assert.Equal(t, club.Peer, peer)
}

// A message found in a forum names its topic when the owner holds the topic,
// General always; one in a private chat says so, since its sender alone does
// not tell a message to Ada from one from her (#349).
func TestSearchMessages_NamesTheTopicAndAPrivateChat(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{
			{ID: 9, ChatID: 20, TopicID: 12},
			{ID: 8, ChatID: 20},
			{ID: 7, ChatID: 20, TopicID: 30},
			{ID: 4, ChatID: 1},
		},
		Chats: []domain.Chat{club, adaChat},
	}}
	o, s := newOwnerWithClient(t, server)
	s.Store().SetChat(club)
	s.Store().SetTopicsPage(20, []domain.Topic{{ChatID: 20, ID: 12, Title: "Releases"}})

	got, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{}, 30)

	require.NoError(t, err)
	require.Len(t, got.Found, 4)
	assert.Equal(t, "Releases", got.Found[0].Topic)
	assert.Equal(t, "General", got.Found[1].Topic, "General is named without being held")
	assert.Empty(t, got.Found[2].Topic, "a topic the owner does not hold is not named")
	assert.Empty(t, got.Found[3].Topic, "outside a forum there is no topic")
	assert.False(t, got.Found[0].Private)
	assert.True(t, got.Found[3].Private)
}

// A page names where the next one starts by the chat's id alone: the address
// Telegram needs for it stays with the owner (#349).
func TestSearchMessages_NamesTheNextPageWithNoAddress(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{{ID: 9, ChatID: 20}},
		Chats:    []domain.Chat{club},
		Next:     &internaltg.SearchPosition{Rate: 1731500000, Peer: club.Peer, ID: 9},
	}}
	o := newSearchOwner(t, server)

	got, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{}, 30)

	require.NoError(t, err)
	require.NotNil(t, got.Next)
	assert.Equal(t, project.SearchPosition{Rate: 1731500000, ChatID: 20, ID: 9}, *got.Next)
}

// The next page is asked from where the last one ended, with the address of
// the chat it names put back: the one the page before was found in (#349).
func TestSearchMessages_TheNextPageIsAskedWithTheChatsAddress(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{{ID: 9, ChatID: 20}},
		Chats:    []domain.Chat{club},
		Next:     &internaltg.SearchPosition{Rate: 1731500000, Peer: club.Peer, ID: 9},
	}}
	o := newSearchOwner(t, server)
	first, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{}, 30)
	require.NoError(t, err)
	require.NotNil(t, first.Next)

	_, err = o.SearchMessages(context.Background(), project.SearchScope{}, "hello", *first.Next, 30)

	require.NoError(t, err)
	assert.Equal(t, internaltg.SearchPosition{Rate: 1731500000, Peer: club.Peer, ID: 9}, server.searchedAfter)
}

// A next page naming a chat the owner cannot address is not asked: asked with
// no address, Telegram would answer the first page again.
func TestSearchMessages_ANextPageInAChatNobodyHoldsIsNotFound(t *testing.T) {
	server := &serverSearch{}
	o := newSearchOwner(t, server)

	_, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{Rate: 5, ChatID: 99, ID: 9}, 30)

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.False(t, server.searched)
}

// A chat the owner cannot address is not searched at all.
func TestSearchMessages_AChatNobodyHoldsIsNotFound(t *testing.T) {
	server := &serverSearch{}
	o := newSearchOwner(t, server)

	_, err := o.SearchMessages(context.Background(), project.SearchScope{ChatID: 99}, "hello", project.SearchPosition{}, 30)

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.False(t, server.searched)
}

// The history around a message the store does not hold is Telegram's answer,
// kept as a run of its own: the history between it and the rest stays
// unclaimed (#347).
func TestHistoryAround_AsksTelegramAndKeepsTheRun(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 48, ChatID: 1}, {ID: 50, ChatID: 1}, {ID: 52, ChatID: 1}}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.HistoryAround(context.Background(), oneChat, 50, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{48, 50, 52}, ids(got))
	assert.Equal(t, adaChat.Peer, server.aroundIn)
	assert.Equal(t, 50, server.aroundID)
	assert.Equal(t, 5, server.aroundLimit)
	assert.Equal(t, []int{48, 50, 52}, ids(o.state.Store().Messages(oneChat)))
}

// What the store holds around a message, enough of it either side, answers
// without asking Telegram, in the shape Telegram answers: half the rest newer,
// the others older.
func TestHistoryAround_AnswersFromTheStore(t *testing.T) {
	server := &serverSearch{}
	o := newSearchOwner(t, server, adaChat)
	o.state.Store().MergeAround(oneChat, messagesUpTo(100)[39:60])

	got, err := o.HistoryAround(context.Background(), oneChat, 50, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{48, 49, 50, 51, 52}, ids(got))
	assert.False(t, server.aroundAsked)
}

// A run that ends close to the message answers only when it ends at the tail,
// where Telegram has nothing more to give either.
func TestHistoryAround_ARunTooShortAroundAsksTelegram(t *testing.T) {
	server := &serverSearch{around: messagesUpTo(100)[47:52]}
	o := newSearchOwner(t, server, adaChat)
	o.state.Store().MergeAround(oneChat, messagesUpTo(100)[39:50])

	_, err := o.HistoryAround(context.Background(), oneChat, 50, 5)

	require.NoError(t, err)
	assert.True(t, server.aroundAsked, "the run ends at 50 and is not the tail run")
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

	assert.True(t, IsNoMessage(err), "got %v", err)
	assert.False(t, IsNoTopic(err))
}

// A topic Telegram does not know is no topic, not a missing message:
// messages.getReplies answers a bad topic with MSG_ID_INVALID, which reads as
// NotFound (#348).
func TestHistoryAround_ATopicNotThereIsNoTopic(t *testing.T) {
	server := &serverSearch{aroundErr: &telerr.Error{Kind: telerr.NotFound, Detail: "MSG_ID_INVALID"}}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.HistoryAround(context.Background(), domain.HistoryKey{ChatID: 2, TopicID: 999}, 50, 5)

	assert.True(t, IsNoTopic(err), "got %v", err)
	assert.False(t, IsNoMessage(err))
}

// A forum's messages are its topics', as for History.
func TestHistoryAround_AForumNeedsATopic(t *testing.T) {
	server := &serverSearch{}
	forum := domain.Chat{ID: 2, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 2, Type: domain.PeerSuperGroup}}
	o := newSearchOwner(t, server, forum)

	_, err := o.HistoryAround(context.Background(), domain.HistoryKey{ChatID: 2}, 50, 5)

	assert.True(t, IsNoTopic(err), "got %v", err)
	assert.False(t, server.aroundAsked)
}

// A forum the owner knows by its address alone needs a topic all the same:
// asked whole, it says it is a forum, and its messages are not mixed from
// every topic (#348).
func TestHistoryAround_AForumKnownByAddressNeedsATopic(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{chats: map[int64]domain.Chat{20: club}}
	o := newSearchOwner(t, server)
	o.state.RememberAddress(club.Peer)

	_, err := o.HistoryAround(context.Background(), domain.HistoryKey{ChatID: 20}, 50, 5)

	assert.True(t, IsNoTopic(err), "got %v", err)
	assert.False(t, server.aroundAsked)
}

// At the newest message there is nothing newer: what Telegram gives is the
// answer.
func TestHistoryAround_AtTheNewestMessage(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 8, ChatID: 1}, {ID: 9, ChatID: 1}, {ID: 10, ChatID: 1}}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.HistoryAround(context.Background(), oneChat, 10, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{8, 9, 10}, ids(got))
}

// A message far older than anything stored is asked of Telegram, and the page
// is a run of its own beside the tail run, the stretch between still
// unclaimed.
func TestHistoryAround_FarBackIsARunOfItsOwn(t *testing.T) {
	server := &serverSearch{around: []domain.Message{{ID: 4, ChatID: 1}, {ID: 5, ChatID: 1}, {ID: 6, ChatID: 1}}}
	o := newSearchOwner(t, server, adaChat)
	stored := messagesUpTo(100)[90:]
	o.state.Store().MergeOlder(oneChat, 0, stored)

	got, err := o.HistoryAround(context.Background(), oneChat, 5, 3)

	require.NoError(t, err)
	assert.Equal(t, []int{4, 5, 6}, ids(got))
	runs := o.state.Store().Runs(oneChat)
	require.Len(t, runs, 2)
	assert.Equal(t, []int{4, 5, 6}, ids(runs[0].Messages))
	assert.Equal(t, ids(stored), ids(runs[1].Messages))
}

// A message found in a forum's General carries no topic, as Telegram names
// General by naming none: the answer names General, so a command can read it
// next with --topic (#348).
func TestSearchMessages_AMessageInGeneralIsInGeneral(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverSearch{found: internaltg.FoundMessages{
		Messages: []domain.Message{{ID: 9, ChatID: 20}, {ID: 8, ChatID: 20, TopicID: 12}, {ID: 4, ChatID: 1}},
		Chats:    []domain.Chat{club, adaChat},
	}}
	o := newSearchOwner(t, server, adaChat)

	got, err := o.SearchMessages(context.Background(), project.SearchScope{}, "hello", project.SearchPosition{}, 30)

	require.NoError(t, err)
	require.Len(t, got.Found, 3)
	assert.Equal(t, domain.GeneralTopicID, got.Found[0].Message.TopicID)
	assert.Equal(t, 12, got.Found[1].Message.TopicID)
	assert.Equal(t, 0, got.Found[2].Message.TopicID, "outside a forum there is no topic")
}
