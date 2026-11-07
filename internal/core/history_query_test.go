package core

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// serverHistory is Telegram holding one history: it answers a page of up to
// limit messages older than offsetID, the newest when offsetID is zero, oldest
// first, as the client hands messages.getHistory's answer on.
type serverHistory struct {
	Connection
	msgs []domain.Message // oldest first
	// fetchedFrom is every offset a page was asked from.
	fetchedFrom []int
	repliesOf   []int
}

func (s *serverHistory) Updates() <-chan store.Event { return nil }

func (s *serverHistory) page(offsetID, limit int) []domain.Message {
	s.fetchedFrom = append(s.fetchedFrom, offsetID)
	var out []domain.Message
	for i := len(s.msgs) - 1; i >= 0 && len(out) < limit; i-- {
		if offsetID == 0 || s.msgs[i].ID < offsetID {
			out = append(out, s.msgs[i])
		}
	}
	slices.Reverse(out)
	return out
}

func (s *serverHistory) GetHistory(_ context.Context, _ domain.Peer, offsetID, limit int) ([]domain.Message, error) {
	return s.page(offsetID, limit), nil
}

func (s *serverHistory) GetReplies(_ context.Context, _ domain.Peer, topicID, offsetID, limit int) ([]domain.Message, error) {
	s.repliesOf = append(s.repliesOf, topicID)
	return s.page(offsetID, limit), nil
}

// messagesUpTo is a history of ids 1..n in chat 1.
func messagesUpTo(n int) []domain.Message {
	out := make([]domain.Message, 0, n)
	for id := 1; id <= n; id++ {
		out = append(out, domain.Message{ID: id, ChatID: 1, Text: "m"})
	}
	return out
}

func ids(msgs []domain.Message) []int {
	out := make([]int, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}

var oneChat = domain.HistoryKey{ChatID: 1}

func newHistoryOwner(t *testing.T, server *serverHistory) *Owner {
	t.Helper()
	o, s := newOwnerWithClient(t, server)
	s.Store().SetChat(domain.Chat{ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	return o
}

// What the store holds answers, oldest first, without asking Telegram (#181).
func TestHistory_AnswersFromTheStore(t *testing.T) {
	server := &serverHistory{}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeMessages(oneChat, messagesUpTo(10))

	got, err := o.History(context.Background(), oneChat, 0, 3)

	require.NoError(t, err)
	assert.Equal(t, []int{8, 9, 10}, ids(got))
	assert.Empty(t, server.fetchedFrom)
}

// What the store lacks is fetched as a window's backfill would fetch it, page
// by page back from the oldest message held, and kept: a window opened after
// finds it there.
func TestHistory_FetchesWhatTheStoreLacksAndKeepsIt(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(50)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeMessages(oneChat, messagesUpTo(50)[45:])

	got, err := o.History(context.Background(), oneChat, 0, 30)

	require.NoError(t, err)
	assert.Equal(t, ids(messagesUpTo(50)[20:]), ids(got))
	assert.Equal(t, 46, server.fetchedFrom[0], "the first page goes back from the oldest message held")
	held := o.state.Store().Messages(oneChat)
	require.NotEmpty(t, held)
	assert.LessOrEqual(t, held[0].ID, 21, "the store did not keep what was fetched")
}

// A history shorter than asked for answers with all of it.
func TestHistory_AShortHistoryIsAllOfIt(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(5)}
	o := newHistoryOwner(t, server)

	got, err := o.History(context.Background(), oneChat, 0, 20)

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, ids(got))
}

// before answers with the messages older than it.
func TestHistory_BeforeAnswersWithTheOlderMessages(t *testing.T) {
	server := &serverHistory{}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeMessages(oneChat, messagesUpTo(10))

	got, err := o.History(context.Background(), oneChat, 6, 3)

	require.NoError(t, err)
	assert.Equal(t, []int{3, 4, 5}, ids(got))
}

// Messages older than anything held are asked of Telegram from before itself
// and not kept: kept, they would sit in the store with the stretch between
// them and what it holds missing, and a window would draw them as adjacent.
func TestHistory_FarBackIsFetchedAndNotKept(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(100)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeMessages(oneChat, messagesUpTo(100)[90:])

	got, err := o.History(context.Background(), oneChat, 30, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{25, 26, 27, 28, 29}, ids(got))
	assert.Equal(t, []int{30}, server.fetchedFrom)
	assert.Len(t, o.state.Store().Messages(oneChat), 10, "the store kept a page it cannot join")
}

// A topic's history is its own thread, fetched as replies to it.
func TestHistory_ATopicIsItsOwnThread(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(3)}
	for i := range server.msgs {
		server.msgs[i].TopicID = 17
	}
	o := newHistoryOwner(t, server)
	o.state.Store().SetChat(domain.Chat{ID: 1, Title: "Forum", IsForum: true, Peer: domain.Peer{ID: 1, Type: domain.PeerSuperGroup}})

	got, err := o.History(context.Background(), domain.HistoryKey{ChatID: 1, TopicID: 17}, 0, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, ids(got))
	require.NotEmpty(t, server.repliesOf)
	for _, topic := range server.repliesOf {
		assert.Equal(t, 17, topic)
	}
}

// A forum has no history of its own: its messages are its topics'.
func TestHistory_AForumWithoutATopicIsNone(t *testing.T) {
	o := newHistoryOwner(t, &serverHistory{})
	o.state.Store().SetChat(domain.Chat{ID: 1, Title: "Forum", IsForum: true, Peer: domain.Peer{ID: 1, Type: domain.PeerSuperGroup}})

	_, err := o.History(context.Background(), oneChat, 0, 5)

	assert.Equal(t, telerr.NotFound, telerr.Of(err), "got %v", err)
}

// A chat reached by its address alone, such as one named by a username the
// chats did not have, is read from Telegram and not stored: the account has
// no dialog with it.
func TestHistory_AChatKnownByAddressIsReadAndNotStored(t *testing.T) {
	server := &serverHistory{msgs: []domain.Message{{ID: 1, ChatID: 42}, {ID: 2, ChatID: 42}}}
	o, s := newOwnerWithClient(t, server)
	s.RememberAddress(domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 4242})
	h := domain.HistoryKey{ChatID: 42}

	got, err := o.History(context.Background(), h, 0, 5)

	require.NoError(t, err)
	assert.Equal(t, []int{1, 2}, ids(got))
	assert.Empty(t, s.Store().Messages(h))
}
