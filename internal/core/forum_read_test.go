package core

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

func topic12() domain.HistoryKey { return domain.HistoryKey{ChatID: forumChat, TopicID: 12} }

func forumWithTopics(t *testing.T, c *stubClient, topics ...domain.Topic) *Owner {
	t.Helper()
	o := forumCmdOwner(t, c)
	o.state.Store().SetTopicsPage(forumChat, topics)
	return o
}

// Reading a topic is read in that topic alone: Telegram is told with
// readDiscussion, and the topic's own pointer and count move.
func TestMarkRead_ATopicReadsThatTopic(t *testing.T) {
	c := &stubClient{}
	o := forumWithTopics(t, c,
		domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases", UnreadCount: 3, TopMessageID: 40, ReadInboxMaxID: 30},
		domain.Topic{ChatID: forumChat, ID: 30, Title: "Design", UnreadCount: 2, TopMessageID: 41},
	)

	require.NoError(t, o.MarkRead(context.Background(), topic12(), 40))

	assert.Equal(t, []discussionRead{{topic: 12, maxID: 40}}, c.discussions)
	got, _ := o.state.Store().Topic(forumChat, 12)
	assert.Equal(t, 40, got.ReadInboxMaxID)
	assert.Zero(t, got.UnreadCount, "read up to its newest message, nothing is left")
	other, _ := o.state.Store().Topic(forumChat, 30)
	assert.Equal(t, 2, other.UnreadCount, "another topic is untouched")
}

// Marking a forum read is marking every topic in it read, General included.
func TestMarkRead_AForumReadsEveryUnreadTopic(t *testing.T) {
	c := &stubClient{}
	o := forumWithTopics(t, c,
		domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases", UnreadCount: 3, TopMessageID: 40},
		domain.Topic{ChatID: forumChat, ID: 1, Title: "General", UnreadCount: 1, TopMessageID: 42},
		domain.Topic{ChatID: forumChat, ID: 30, Title: "Design", TopMessageID: 41},
	)

	require.NoError(t, o.MarkRead(context.Background(), domain.HistoryKey{ChatID: forumChat}, 0))

	assert.ElementsMatch(t, []discussionRead{{topic: 12, maxID: 40}, {topic: 1, maxID: 42}}, c.discussions)
	for _, tp := range o.state.Store().Topics(forumChat) {
		assert.Zero(t, tp.UnreadCount, "topic %d", tp.ID)
	}
}

// Muting a topic is the topic's own setting; the forum's stays as it was.
func TestSetMuted_ATopicIsThatTopicsSetting(t *testing.T) {
	c := &stubClient{}
	o := forumWithTopics(t, c, domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases"})

	require.NoError(t, o.SetMuted(context.Background(), topic12(), true))

	got, _ := o.state.Store().Topic(forumChat, 12)
	assert.Equal(t, domain.TopicMuted, got.Mute)
	chat, _ := o.state.Store().GetChat(forumChat)
	assert.False(t, chat.IsMuted)
	assert.Equal(t, map[int]bool{12: true}, c.topicMutedWith)
}

func TestSetMuted_ARefusedTopicMuteIsUndone(t *testing.T) {
	c := &stubClient{err: errRefused}
	o := forumWithTopics(t, c, domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases"})

	require.Error(t, o.SetMuted(context.Background(), topic12(), true))

	got, _ := o.state.Store().Topic(forumChat, 12)
	assert.Equal(t, domain.TopicFollowsForum, got.Mute, "the setting it had before")
}

func TestReadMentionsAndReactions_InATopicAreThatTopics(t *testing.T) {
	c := &stubClient{}
	o := forumWithTopics(t, c,
		domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases", UnreadMentionsCount: 2, UnreadReactionsCount: 3},
		domain.Topic{ChatID: forumChat, ID: 30, Title: "Design", UnreadMentionsCount: 1},
	)

	require.NoError(t, o.ReadMentions(context.Background(), topic12()))
	require.NoError(t, o.ReadReactions(context.Background(), topic12()))

	assert.Equal(t, []int{12}, c.mentionsReadIn)
	assert.Equal(t, []int{12}, c.reactionsReadIn)
	got, _ := o.state.Store().Topic(forumChat, 12)
	assert.Zero(t, got.UnreadMentionsCount)
	assert.Zero(t, got.UnreadReactionsCount)
	other, _ := o.state.Store().Topic(forumChat, 30)
	assert.Equal(t, 1, other.UnreadMentionsCount)
}

func TestMarkReadAndMute_AllTopicsReadsTopicsAndMutesForum(t *testing.T) {
	c := &stubClient{}
	o := forumWithTopics(t, c,
		domain.Topic{ChatID: forumChat, ID: 12, Title: "Releases", UnreadCount: 3, TopMessageID: 40, UnreadMentionsCount: 1},
		domain.Topic{ChatID: forumChat, ID: 30, Title: "Design", UnreadCount: 2, TopMessageID: 41},
	)
	allKey := domain.HistoryKey{ChatID: forumChat, TopicID: domain.AllTopicsID}

	require.NoError(t, o.MarkRead(context.Background(), allKey, 0))
	assert.ElementsMatch(t, []discussionRead{{topic: 12, maxID: 40}, {topic: 30, maxID: 41}}, c.discussions)

	require.NoError(t, o.SetMuted(context.Background(), allKey, true))
	chat, _ := o.state.Store().GetChat(forumChat)
	assert.True(t, chat.IsMuted, "muting All mutes the chat rather than a topic")

	require.NoError(t, o.ReadMentions(context.Background(), allKey))
	assert.Equal(t, []int{0}, c.mentionsReadIn)
}
