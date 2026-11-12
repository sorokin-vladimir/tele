package core

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
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
	// chats is what FetchChat answers with by id; any other address is an
	// ordinary chat.
	chats map[int64]domain.Chat
}

func (s *serverHistory) Updates() <-chan store.Event { return nil }

func (s *serverHistory) FetchChat(_ context.Context, peer domain.Peer) (domain.Chat, error) {
	if c, ok := s.chats[peer.ID]; ok {
		return c, nil
	}
	return domain.Chat{ID: peer.ID, Peer: peer}, nil
}

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
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(10))

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
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(50)[45:])

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
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(10))

	got, err := o.History(context.Background(), oneChat, 6, 3)

	require.NoError(t, err)
	assert.Equal(t, []int{3, 4, 5}, ids(got))
}

// Messages older than anything held are asked of Telegram from before itself
// and kept as a run of their own: the stretch between them and what the store
// holds stays unclaimed, and the next command asking for them is answered from
// the store (#347).
func TestHistory_FarBackIsFetchedAndKeptAsARun(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(100)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(100)[90:])

	got, err := o.History(context.Background(), oneChat, 30, 5)
	again, errAgain := o.History(context.Background(), oneChat, 30, 5)

	require.NoError(t, err)
	require.NoError(t, errAgain)
	assert.Equal(t, []int{25, 26, 27, 28, 29}, ids(got))
	assert.Equal(t, ids(got), ids(again))
	assert.Equal(t, []int{30}, server.fetchedFrom, "the second answer came from the store")
	assert.Len(t, o.state.Store().Runs(oneChat), 2, "a run far back beside the tail run")
}

// historyWithin runs History with a deadline, so a command that never ends
// fails the test rather than hanging it.
func historyWithin(t *testing.T, o *Owner, before, limit int) []domain.Message {
	t.Helper()
	type answer struct {
		msgs []domain.Message
		err  error
	}
	done := make(chan answer, 1)
	go func() {
		msgs, err := o.History(context.Background(), oneChat, before, limit)
		done <- answer{msgs, err}
	}()
	select {
	case a := <-done:
		require.NoError(t, a.err)
		return a.msgs
	case <-time.After(2 * time.Second):
		t.Fatal("History never answered")
		return nil
	}
}

// fullStore holds the newest store.MaxMessagesPerChat of a 1000-message
// history, nobody looking at it.
func fullStore(t *testing.T) (*Owner, *serverHistory) {
	t.Helper()
	server := &serverHistory{msgs: messagesUpTo(1000)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(1000)[1000-store.MaxMessagesPerChat:])
	return o, server
}

// What a command is reading is not trimmed while it reads it, even with the
// store full and no window open: the cap gives up a run nobody looks at, and
// a command running is somebody reading.
func TestHistory_FarBackIsAnsweredWithTheStoreFull(t *testing.T) {
	o, server := fullStore(t)

	got := historyWithin(t, o, 30, 5)

	assert.Equal(t, []int{25, 26, 27, 28, 29}, ids(got))
	assert.Equal(t, []int{30}, server.fetchedFrom)
}

// Reading back past the cap from the tail keeps what the command has read so
// far until it has all it asked for.
func TestHistory_ReadingPastTheCapAnswers(t *testing.T) {
	o, _ := fullStore(t)

	got := historyWithin(t, o, 0, store.MaxMessagesPerChat+30)

	require.Len(t, got, store.MaxMessagesPerChat+30)
	assert.Equal(t, 1000-store.MaxMessagesPerChat-29, got[0].ID)
}

// Reading on from a run far back grows that run, not the tail run, and never
// answers across the stretch nobody fetched.
func TestHistory_ReadingOnFromARunFarBackGrowsIt(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(100)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(100)[90:])
	o.state.Store().MergeAround(oneChat, messagesUpTo(100)[24:29])

	got, err := o.History(context.Background(), oneChat, 28, 6)

	require.NoError(t, err)
	assert.Equal(t, []int{22, 23, 24, 25, 26, 27}, ids(got))
	assert.Equal(t, []int{25}, server.fetchedFrom, "the page went back from the far run's oldest")
	assert.Len(t, o.state.Store().Runs(oneChat), 2)
}

// Asking for what is older than the tail run, with a run far back below it,
// does not take the far run as adjacent.
func TestHistory_ARunFarBackIsNotTakenAsAdjacent(t *testing.T) {
	server := &serverHistory{msgs: messagesUpTo(100)}
	o := newHistoryOwner(t, server)
	o.state.Store().MergeOlder(oneChat, 0, messagesUpTo(100)[90:])
	o.state.Store().MergeAround(oneChat, messagesUpTo(100)[24:29])

	got, err := o.History(context.Background(), oneChat, 0, 12)

	require.NoError(t, err)
	assert.Equal(t, ids(messagesUpTo(100)[88:]), ids(got))
	assert.Equal(t, []int{91}, server.fetchedFrom)
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

	assert.True(t, IsNoTopic(err), "got %v", err)
}

// A forum the owner knows by its address alone has no history of its own
// either: asked whole, it says it is a forum, and its topics are not read
// mixed together as one (#348).
func TestHistory_AForumKnownByAddressWithoutATopicIsNone(t *testing.T) {
	club := domain.Chat{ID: 20, Title: "Club", IsForum: true, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup, AccessHash: 200}}
	server := &serverHistory{chats: map[int64]domain.Chat{20: club}}
	o, s := newOwnerWithClient(t, server)
	s.RememberAddress(club.Peer)

	_, err := o.History(context.Background(), domain.HistoryKey{ChatID: 20}, 0, 5)

	assert.True(t, IsNoTopic(err), "got %v", err)
	assert.Empty(t, server.fetchedFrom, "no page was asked")
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
