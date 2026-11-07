package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

const forumChat = 50

// topicConn is a connection that also answers for forum topics. It records
// what was asked so a test can tell a fetch from no fetch.
type topicConn struct {
	*stubConn
	mu    sync.Mutex
	pages map[int64]internaltg.ForumTopicsPage
	byID  map[int64]internaltg.ForumTopicsPage
	asked []int64
	// askedInBackground is, for each of asked, whether it was background work.
	askedInBackground []bool
	byIDs             [][]int
	gate              chan struct{}
	// topicPages is what getReplies answers for each topic.
	topicPages map[int][]domain.Message
	replies    []repliesCall
	// nextPages is what a page past the first answers; offsets what was asked.
	nextPages map[int64]internaltg.ForumTopicsPage
	offsets   []internaltg.TopicsOffset
}

func newTopicConn() *topicConn {
	return &topicConn{
		stubConn:  &stubConn{},
		pages:     make(map[int64]internaltg.ForumTopicsPage),
		nextPages: make(map[int64]internaltg.ForumTopicsPage),
		byID:      make(map[int64]internaltg.ForumTopicsPage),
	}
}

func (c *topicConn) GetArchivedDialogs(context.Context) ([]domain.Chat, error) { return nil, nil }

func (c *topicConn) GetForumTopics(ctx context.Context, peer domain.Peer, after internaltg.TopicsOffset, _ int) (internaltg.ForumTopicsPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if after != (internaltg.TopicsOffset{}) {
		c.offsets = append(c.offsets, after)
		return c.nextPages[peer.ID], nil
	}
	c.asked = append(c.asked, peer.ID)
	c.askedInBackground = append(c.askedInBackground, priority.IsBackground(ctx))
	return c.pages[peer.ID], nil
}

func (c *topicConn) offsetsAsked() []internaltg.TopicsOffset {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]internaltg.TopicsOffset(nil), c.offsets...)
}

func (c *topicConn) GetForumTopicsByID(_ context.Context, peer domain.Peer, ids []int) (internaltg.ForumTopicsPage, error) {
	if c.gate != nil {
		<-c.gate
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byIDs = append(c.byIDs, append([]int(nil), ids...))
	return c.byID[peer.ID], nil
}

// repliesCall records one getReplies request.
type repliesCall struct {
	topic, offsetID int
}

func (c *topicConn) GetReplies(_ context.Context, _ domain.Peer, topicID, offsetID, _ int) ([]domain.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.replies = append(c.replies, repliesCall{topic: topicID, offsetID: offsetID})
	return c.topicPages[topicID], nil
}

func (c *topicConn) repliesCalls() []repliesCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]repliesCall(nil), c.replies...)
}

func (c *topicConn) askedFor() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.asked...)
}

func (c *topicConn) byIDCalls() [][]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]int(nil), c.byIDs...)
}

func forumDialog() domain.Chat {
	return domain.Chat{ID: forumChat, Title: "Dev", IsForum: true, Peer: domain.Peer{ID: forumChat, Type: domain.PeerSuperGroup}}
}

func forumOwner(t *testing.T, c *topicConn) (*Owner, *state.State) {
	t.Helper()
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(forumDialog())
	return o, s
}

func arrival(id, topic int) store.Event {
	return store.Event{Kind: store.EventNewMessage, Message: domain.Message{
		ID: id, ChatID: forumChat, TopicID: topic, Date: time.Unix(int64(id), 0), Text: "hi",
	}}
}

func TestOwner_AnArrivalInAForumLandsInItsTopic(t *testing.T) {
	c := newTopicConn()
	o, s := forumOwner(t, c)
	s.Store().SetTopicsPage(forumChat, []domain.Topic{{ChatID: forumChat, ID: 12, Title: "Releases"}})

	o.handleEvent(arrival(40, 12))

	got, _ := s.Store().Topic(forumChat, 12)
	assert.Equal(t, 1, got.UnreadCount)
	assert.Equal(t, 40, got.TopMessageID)
	assert.Len(t, s.Store().Messages(domain.HistoryKey{ChatID: forumChat, TopicID: 12}), 1)
	assert.Empty(t, c.byIDCalls(), "a topic already described is not asked about")
}

// A message for a topic nobody has described is the cue to ask Telegram what
// that topic is; until the answer comes it is a topic known by its id alone.
func TestOwner_AnArrivalInAnUnknownTopicAsksWhatItIs(t *testing.T) {
	c := newTopicConn()
	c.byID[forumChat] = internaltg.ForumTopicsPage{Topics: []domain.Topic{{ChatID: forumChat, ID: 30, Title: "Design"}}}
	o, s := forumOwner(t, c)

	o.handleEvent(arrival(40, 30))

	require.Eventually(t, func() bool {
		got, _ := s.Store().Topic(forumChat, 30)
		return got.Title == "Design"
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, [][]int{{30}}, c.byIDCalls())
}

func TestOwner_ATopicSignalReadsTheTopicAgain(t *testing.T) {
	c := newTopicConn()
	c.byID[forumChat] = internaltg.ForumTopicsPage{Topics: []domain.Topic{{ChatID: forumChat, ID: 12, Title: "Releases", ReadInboxMaxID: 90}}}
	o, s := forumOwner(t, c)

	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: forumChat, MsgIDs: []int{12}})

	require.Eventually(t, func() bool {
		got, _ := s.Store().Topic(forumChat, 12)
		return got.ReadInboxMaxID == 90
	}, time.Second, 5*time.Millisecond)
}

// A channel's comment threads are read the same way a topic is, and they are
// not topics: a chat that is not a forum has nothing to read again.
func TestOwner_ATopicSignalForAChatThatIsNoForumIsIgnored(t *testing.T) {
	c := newTopicConn()
	o, s := newOwnerWithClient(t, c)
	s.Store().SetChat(domain.Chat{ID: 60, Peer: domain.Peer{ID: 60, Type: domain.PeerSuperGroup}})

	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: 60, MsgIDs: []int{12}})
	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: 60, TopicsPage: true})

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, c.byIDCalls())
	assert.Empty(t, c.askedFor())
}

func TestOwner_APinChangeReadsTheFirstPageAgain(t *testing.T) {
	c := newTopicConn()
	c.pages[forumChat] = internaltg.ForumTopicsPage{Topics: []domain.Topic{{ChatID: forumChat, ID: 12, Title: "Releases", Pinned: true}}}
	o, s := forumOwner(t, c)

	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: forumChat, TopicsPage: true})

	require.Eventually(t, func() bool {
		got, _ := s.Store().Topic(forumChat, 12)
		return got.Pinned
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, []int64{forumChat}, c.askedFor())
}

// Topic signals come in bursts. One read is in flight per forum, and whatever
// is asked for meanwhile goes out together in the next.
func TestOwner_TopicReadsAskedForMeanwhileGoOutTogether(t *testing.T) {
	c := newTopicConn()
	c.gate = make(chan struct{})
	o, _ := forumOwner(t, c)

	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: forumChat, MsgIDs: []int{30}})
	time.Sleep(20 * time.Millisecond)
	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: forumChat, MsgIDs: []int{32}})
	o.handleEvent(store.Event{Kind: store.EventTopicsChanged, ChatID: forumChat, MsgIDs: []int{31}})
	close(c.gate)

	require.Eventually(t, func() bool { return len(c.byIDCalls()) == 2 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, [][]int{{30}, {31, 32}}, c.byIDCalls())
}

func topicHistory(topic int, ids ...int) []domain.Message {
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.Message{ID: id, ChatID: forumChat, TopicID: topic, Date: time.Unix(int64(id), 0)})
	}
	return out
}

// A topic's scrollback is the topic's own: it is fetched with getReplies from
// the oldest message the topic holds, never with the forum's history, which
// would bring every other topic's messages instead.
func TestOwner_ScrollingATopicFetchesThatTopicsHistory(t *testing.T) {
	c := newTopicConn()
	c.topicPages = map[int][]domain.Message{12: topicHistory(12, 3, 5, 8)}
	o, s := forumOwner(t, c)
	s.Store().SetTopicsPage(forumChat, []domain.Topic{{ChatID: forumChat, ID: 12, Title: "Releases"}})
	s.Store().AppendMessage(topicHistory(12, 20)[0])
	h := domain.HistoryKey{ChatID: forumChat, TopicID: 12}

	testClient(o).Subscribe(project.HistoryWindow{ChatID: forumChat, TopicID: 12, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10})

	require.Eventually(t, func() bool { return len(s.Store().Messages(h)) == 4 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []repliesCall{{topic: 12, offsetID: 20}}, c.repliesCalls())
	assert.Zero(t, c.calls.Load(), "the forum's own history is never asked for")
}

func TestOwner_GeneralIsFetchedAsATopicToo(t *testing.T) {
	c := newTopicConn()
	c.topicPages = map[int][]domain.Message{domain.GeneralTopicID: topicHistory(0, 3)}
	o, s := forumOwner(t, c)
	h := domain.HistoryKey{ChatID: forumChat, TopicID: domain.GeneralTopicID}

	testClient(o).Subscribe(project.HistoryWindow{ChatID: forumChat, TopicID: domain.GeneralTopicID, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10})

	require.Eventually(t, func() bool { return len(s.Store().Messages(h)) == 1 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []repliesCall{{topic: domain.GeneralTopicID, offsetID: 0}}, c.repliesCalls())
}

// A forum's gap lies across every topic, and is closed by the forum's own
// history laid out among them: opening one topic repairs all of them.
func TestOwner_ClosingAForumsGapRepairsEveryTopic(t *testing.T) {
	c := newTopicConn()
	c.server = append(append(topicHistory(12, 1, 2), topicHistory(30, 3)...), append(topicHistory(30, 4), topicHistory(12, 5)...)...)
	o, s := forumOwner(t, c)
	chat := forumDialog()
	chat.TopMessageID = 5
	s.Store().SetChat(chat)
	s.Store().SetMessages(forumChat, append(topicHistory(12, 1, 2), topicHistory(30, 3)...))
	s.Store().MarkGap(forumChat, 3)

	testClient(o).Subscribe(project.HistoryWindow{ChatID: forumChat, TopicID: 12, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 1})

	require.Eventually(t, func() bool {
		_, open := s.Store().Gap(forumChat)
		return !open
	}, time.Second, 5*time.Millisecond)
	assert.Equal(t, []int{1, 2, 5}, msgIDs(s.Store().Messages(domain.HistoryKey{ChatID: forumChat, TopicID: 12})))
	assert.Equal(t, []int{3, 4}, msgIDs(s.Store().Messages(domain.HistoryKey{ChatID: forumChat, TopicID: 30})),
		"the topic nobody opened was repaired with it")
}

func msgIDs(ms []domain.Message) []int {
	out := make([]int, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func activeTopic(id int, sec int64) domain.Topic {
	return domain.Topic{ChatID: forumChat, ID: id, Title: "t", TopMessageID: id * 10,
		LastMessage: &domain.Message{ID: id * 10, ChatID: forumChat, Date: time.Unix(sec, 0)}}
}

// A topic list scrolled past what the first page brought asks for the next
// page, after the last topic it has; one that has all of them asks nothing.
func TestOwner_ScrollingTheTopicListLoadsTheNextPage(t *testing.T) {
	c := newTopicConn()
	c.dialogs = []domain.Chat{forumDialog()}
	c.pages[forumChat] = internaltg.ForumTopicsPage{Total: 3, Topics: []domain.Topic{activeTopic(12, 300), activeTopic(30, 200)}}
	c.nextPages[forumChat] = internaltg.ForumTopicsPage{Total: 3, Topics: []domain.Topic{activeTopic(31, 100)}}
	o, s := newOwnerWithClient(t, c)
	require.NoError(t, o.Bootstrap(context.Background()))
	require.Eventually(t, func() bool { return len(s.Store().Topics(forumChat)) == 2 }, time.Second, 5*time.Millisecond)

	id := testClient(o).Subscribe(project.TopicListWindow{ChatID: forumChat, Limit: 10})

	require.Eventually(t, func() bool { return len(s.Store().Topics(forumChat)) == 3 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []internaltg.TopicsOffset{{Date: time.Unix(200, 0), MsgID: 300, TopicID: 30}}, c.offsetsAsked())

	testClient(o).MoveWindow(id, project.TopicListWindow{ChatID: forumChat, Limit: 20})
	time.Sleep(30 * time.Millisecond)
	assert.Len(t, c.offsetsAsked(), 1, "every topic is held, so there is nothing more to ask for")
}

// A modal choosing a topic - the forward picker - asks for a forum's topics
// once, the way it asks for the chat list.
func TestOwner_TopicsAnswersWithAForumsTopicRows(t *testing.T) {
	c := newTopicConn()
	o, s := forumOwner(t, c)
	s.Store().SetTopicsPage(forumChat, []domain.Topic{
		{ChatID: forumChat, ID: 12, Title: "Releases", UnreadCount: 2},
		{ChatID: forumChat, ID: 30, Title: "Design", Mute: domain.TopicMuted},
	})

	rows, err := o.Topics(context.Background(), forumChat)

	require.NoError(t, err)
	byID := make(map[int]project.TopicRow, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	require.Len(t, byID, 2)
	assert.Equal(t, project.TopicRow{ID: 12, Title: "Releases", Unread: 2}, byID[12])
	assert.True(t, byID[30].Muted)
}

func TestOwner_BootstrapLoadsEachForumsTopics(t *testing.T) {
	c := newTopicConn()
	c.dialogs = []domain.Chat{
		forumDialog(),
		{ID: 60, Title: "Plain", Peer: domain.Peer{ID: 60, Type: domain.PeerSuperGroup}},
	}
	c.pages[forumChat] = internaltg.ForumTopicsPage{Topics: []domain.Topic{
		{ChatID: forumChat, ID: 12, Title: "Releases"},
		{ChatID: forumChat, ID: 1, Title: "General"},
	}}
	o, s := newOwnerWithClient(t, c)

	require.NoError(t, o.Bootstrap(context.Background()))

	require.Eventually(t, func() bool { return len(s.Store().Topics(forumChat)) == 2 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []int64{forumChat}, c.askedFor(), "an ordinary group has no topics to ask about")
}

// Nobody waits on the topics read at connect: the main screen comes up on the
// dialog list, and a forum's badge can follow. So they yield to what someone
// is waiting on (#212).
func TestOwner_TheTopicsReadAtConnectAreBackgroundWork(t *testing.T) {
	c := newTopicConn()
	c.dialogs = []domain.Chat{forumDialog()}
	o, _ := newOwnerWithClient(t, c)

	require.NoError(t, o.Bootstrap(context.Background()))

	require.Eventually(t, func() bool { return len(c.askedFor()) == 1 }, time.Second, 5*time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.Equal(t, []bool{true}, c.askedInBackground)
}
