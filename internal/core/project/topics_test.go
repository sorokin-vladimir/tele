package project_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

const forum = 50

func forumChat() domain.Chat {
	return domain.Chat{ID: forum, Title: "Dev", IsForum: true, Peer: domain.Peer{ID: forum, Type: domain.PeerSuperGroup}}
}

func topicMsgs(topic int, ids ...int) []domain.Message {
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.Message{ID: id, ChatID: forum, TopicID: topic, Date: time.Unix(int64(id), 0)})
	}
	return out
}

// A topic window shows the topic's history and the topic's own state: what is
// read, what is unread and what was left half-written there, not the forum's.
func TestBuildHistory_ATopicWindowShowsTheTopic(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{forumChat()},
		topicMsgs: map[domain.HistoryKey][]domain.Message{
			{ChatID: forum, TopicID: 12}: topicMsgs(12, 10, 12, 15),
			{ChatID: forum, TopicID: 30}: topicMsgs(30, 11, 13),
		},
		topics: map[int64][]domain.Topic{forum: {{
			ChatID: forum, ID: 12, Title: "Releases", ReadInboxMaxID: 12, ReadOutboxMaxID: 15,
			UnreadCount: 1, UnreadMentionsCount: 2, UnreadReactionsCount: 3, Draft: "half",
		}}},
	}

	got := project.BuildHistory(r, project.HistoryWindow{
		ChatID: forum, TopicID: 12, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 10,
	})

	assert.Equal(t, 12, got.TopicID)
	assert.Equal(t, "Dev", got.Title)
	assert.Equal(t, "Releases", got.TopicTitle)
	assert.Equal(t, []int{10, 12, 15}, ids(got.Messages))
	assert.Equal(t, 15, got.AnchorMsgID, "the first unread is the topic's, past its own read pointer")
	assert.Equal(t, 12, got.ReadInboxMaxID)
	assert.Equal(t, 15, got.ReadOutboxMaxID)
	assert.Equal(t, 2, got.UnreadMentions)
	assert.Equal(t, 3, got.UnreadReactions)
	assert.Equal(t, "half", got.Draft)
}

// Another topic of the same forum is another history, and nothing the client
// holds for the first applies to it.
func TestDiffHistory_SwitchingTopicIsAReset(t *testing.T) {
	prev := project.HistoryContents{ChatID: forum, TopicID: 12, Messages: topicMsgs(12, 10)}
	next := project.HistoryContents{ChatID: forum, TopicID: 30, Messages: topicMsgs(30, 10)}

	got := project.DiffHistory(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.HistoryReset, got[0].Kind)
}

func TestDiffHistory_ATopicRenamedIsAHeaderUpdate(t *testing.T) {
	prev := project.HistoryContents{ChatID: forum, TopicID: 12, TopicTitle: "Releases", Messages: topicMsgs(12, 10)}
	next := prev
	next.TopicTitle = "Releases 2"

	got := project.DiffHistory(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.HistoryHeaderUpdate, got[0].Kind)
}

// A pin belongs to the history it was taken in. Moving to another topic of the
// same forum is opening another history, and it resolves its own anchor.
func TestRegistry_APinIsNotCarriedIntoAnotherTopic(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{forumChat()},
		topicMsgs: map[domain.HistoryKey][]domain.Message{
			{ChatID: forum, TopicID: 12}: topicMsgs(12, 10, 12),
			{ChatID: forum, TopicID: 30}: topicMsgs(30, 11, 13),
		},
		topics: map[int64][]domain.Topic{forum: {
			{ChatID: forum, ID: 12, UnreadCount: 1, ReadInboxMaxID: 10},
		}},
	}
	g, _ := newRegistry(r)
	id := subscribe(g, project.HistoryWindow{ChatID: forum, TopicID: 12, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 5})

	g.MoveWindow(id, project.HistoryWindow{ChatID: forum, TopicID: 30, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 5})

	w, ok := g.Window(id)
	require.True(t, ok)
	assert.Equal(t, 13, w.(project.HistoryWindow).Anchor.MsgID, "topic 30 has nothing unread, so its newest")
}

func listedTopics() []domain.Topic {
	return []domain.Topic{
		{ChatID: forum, ID: 12, Title: "Releases", Pinned: true, UnreadCount: 2, UnreadMentionsCount: 1, UnreadReactionsCount: 3},
		{ChatID: forum, ID: 1, Title: "General", Hidden: true},
		{ChatID: forum, ID: 30, Title: "Design", Closed: true},
		{ChatID: forum, ID: 31, Title: "Ops"},
	}
}

// A topic list is a window over a forum's topics in the store's order, which
// the builder keeps rather than decides.
func TestBuildTopicList_WindowsTheForumsTopics(t *testing.T) {
	r := &fakeReader{chats: []domain.Chat{forumChat()}, topics: map[int64][]domain.Topic{forum: listedTopics()}}

	got := project.BuildTopicList(r, project.TopicListWindow{ChatID: forum, Offset: 0, Limit: 3})

	assert.Equal(t, int64(forum), got.ChatID)
	assert.Equal(t, 5, got.Total)
	require.Len(t, got.Rows, 3)
	assert.Equal(t, project.TopicRow{ID: domain.AllTopicsID, Title: "All", Unread: 2, Mentions: 1, Reactions: 3}, got.Rows[0])
	assert.Equal(t, project.TopicRow{ID: 12, Title: "Releases", Pinned: true, Unread: 2, Mentions: 1, Reactions: 3}, got.Rows[1])
	assert.True(t, got.Rows[2].Hidden)
}

// A topic row shows whether the topic is muted, which it is by its own setting
// when it has one and by the forum's when it follows it.
func TestBuildTopicList_ARowIsMutedByItsOwnSettingOrTheForums(t *testing.T) {
	f := forumChat()
	f.IsMuted = true
	r := &fakeReader{chats: []domain.Chat{f}, topics: map[int64][]domain.Topic{forum: {
		{ChatID: forum, ID: 12, Mute: domain.TopicFollowsForum},
		{ChatID: forum, ID: 13, Mute: domain.TopicUnmuted},
		{ChatID: forum, ID: 14, Mute: domain.TopicMuted},
	}}}

	got := project.BuildTopicList(r, project.TopicListWindow{ChatID: forum, Limit: 10})

	require.Len(t, got.Rows, 4)
	assert.Equal(t, domain.AllTopicsID, got.Rows[0].ID)
	assert.True(t, got.Rows[0].Muted, "All follows the muted forum")
	assert.True(t, got.Rows[1].Muted, "follows the muted forum")
	assert.False(t, got.Rows[2].Muted, "unmuted in a muted forum")
	assert.True(t, got.Rows[3].Muted)
}

func TestDiffTopicList_AChangedTopicIsOneRow(t *testing.T) {
	r := &fakeReader{chats: []domain.Chat{forumChat()}, topics: map[int64][]domain.Topic{forum: listedTopics()}}
	w := project.TopicListWindow{ChatID: forum, Limit: 10}
	prev := project.BuildTopicList(r, w)
	r.topics[forum][2].UnreadCount = 5

	got := project.DiffTopicList(prev, project.BuildTopicList(r, w))

	require.Len(t, got, 2)
	assert.Equal(t, project.TopicListRow, got[0].Kind)
	assert.Equal(t, domain.AllTopicsID, got[0].Row.ID)
	assert.Equal(t, project.TopicListRow, got[1].Kind)
	assert.Equal(t, 30, got[1].Row.ID)
}

func TestDiffTopicList_ATopicMovingIsAReset(t *testing.T) {
	r := &fakeReader{chats: []domain.Chat{forumChat()}, topics: map[int64][]domain.Topic{forum: listedTopics()}}
	w := project.TopicListWindow{ChatID: forum, Limit: 10}
	prev := project.BuildTopicList(r, w)
	ts := r.topics[forum]
	ts[2], ts[3] = ts[3], ts[2]

	got := project.DiffTopicList(prev, project.BuildTopicList(r, w))

	require.Len(t, got, 1)
	assert.Equal(t, project.TopicListReset, got[0].Kind)
}

func TestRegistry_ATopicListSubscriptionFollowsTheTopics(t *testing.T) {
	r := &fakeReader{chats: []domain.Chat{forumChat()}, topics: map[int64][]domain.Topic{forum: listedTopics()}}
	g, rec := newRegistry(r)
	id := subscribe(g, project.TopicListWindow{ChatID: forum, Limit: 10})

	r.topics[forum][3].Title = "Operations"
	g.Refresh()

	deltas := rec.take()
	require.Len(t, deltas, 2)
	assert.Equal(t, id, deltas[0].Sub)
	require.NotNil(t, deltas[0].TopicList)
	assert.Equal(t, project.TopicListReset, deltas[0].TopicList.Kind)
	assert.Equal(t, "Operations", deltas[1].TopicList.Row.Title)
}

// A forum's row counts its topics, the way Telegram Desktop badges a forum:
// how many topics have unread, and every topic's mentions and reactions. The
// forum's own dialog counts say nothing the topics do not.
func TestBuildChatList_AForumRowCountsItsTopics(t *testing.T) {
	f := forumChat()
	f.UnreadCount = 40
	r := &fakeReader{
		chats: []domain.Chat{f},
		topics: map[int64][]domain.Topic{forum: {
			{ChatID: forum, ID: 12, UnreadCount: 30, UnreadMentionsCount: 1, UnreadReactionsCount: 2},
			{ChatID: forum, ID: 30, UnreadCount: 10, UnreadMentionsCount: 2},
			{ChatID: forum, ID: 31},
		}},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})

	require.Len(t, got.Rows, 1)
	row := got.Rows[0]
	assert.True(t, row.IsForum)
	assert.Equal(t, 2, row.Unread, "two topics have unread")
	assert.Equal(t, 3, row.Mentions)
	assert.Equal(t, 2, row.Reactions)
}

// A folder counts chats with unread, and a forum is one chat however many of
// its topics are unread - and none while none of them is.
func TestBuildChatList_AFolderCountsAForumByItsTopics(t *testing.T) {
	read := forumChat()
	read.UnreadCount = 7
	unread := domain.Chat{ID: 51, IsForum: true, Peer: domain.Peer{ID: 51, Type: domain.PeerSuperGroup}}
	r := &fakeReader{
		chats:   []domain.Chat{read, unread},
		filters: []domain.FolderFilter{{ID: 2, Title: "Work", Groups: true}},
		topics: map[int64][]domain.Topic{
			forum: {{ChatID: forum, ID: 12}},
			51:    {{ChatID: 51, ID: 12, UnreadCount: 1}, {ChatID: 51, ID: 13, UnreadCount: 4}},
		},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})

	assert.Equal(t, 1, got.Folders.Unread[2])
}

// A topic named before anything about it arrived is still a window onto its
// messages, under a title the client supplies.
func TestBuildHistory_ATopicNotYetDescribedStillShowsItsMessages(t *testing.T) {
	r := &fakeReader{
		chats:     []domain.Chat{forumChat()},
		topicMsgs: map[domain.HistoryKey][]domain.Message{{ChatID: forum, TopicID: 30}: topicMsgs(30, 11)},
	}

	got := project.BuildHistory(r, project.HistoryWindow{ChatID: forum, TopicID: 30, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10})

	require.Len(t, got.Messages, 1)
	assert.Empty(t, got.TopicTitle)
}

func TestBuildChatList_BotForumExpandsTopicsInlineAndDiffsReorders(t *testing.T) {
	const botID = int64(77)
	bot := domain.Chat{
		ID: botID, Title: "MyBot", IsBot: true, IsForum: true, UnreadCount: 3,
		Peer: domain.Peer{ID: botID, Type: domain.PeerUser},
	}
	r := &fakeReader{
		chats: []domain.Chat{bot},
		topics: map[int64][]domain.Topic{
			botID: {
				{ChatID: botID, ID: 12, Title: "Releases", UnreadCount: 2},
				{ChatID: botID, ID: 30, Title: "Design", Closed: true},
			},
		},
	}

	got := project.BuildChatList(r, project.ChatListWindow{Limit: 10})
	require.Equal(t, 3, got.Total)
	require.Len(t, got.Rows, 3)
	assert.Equal(t, botID, got.Rows[0].ID)
	assert.False(t, got.Rows[0].IsBotTopic)
	assert.Equal(t, 1, got.Rows[0].Unread)

	assert.Equal(t, botID, got.Rows[1].ID)
	assert.Equal(t, 12, got.Rows[1].TopicID)
	assert.Equal(t, "MyBot", got.Rows[1].BotTitle)
	assert.True(t, got.Rows[1].IsBotTopic)
	assert.Equal(t, 2, got.Rows[1].Unread)

	assert.Equal(t, 30, got.Rows[2].TopicID)
	assert.True(t, got.Rows[2].TopicClosed)

	// Reordering topics emits a Reset even though chat IDs are unchanged.
	r.topics[botID][0], r.topics[botID][1] = r.topics[botID][1], r.topics[botID][0]
	deltas := project.DiffChatList(got, project.BuildChatList(r, project.ChatListWindow{Limit: 10}))
	require.Len(t, deltas, 1)
	assert.Equal(t, project.ChatListReset, deltas[0].Kind)
}
