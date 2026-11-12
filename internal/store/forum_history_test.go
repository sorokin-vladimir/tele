package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

const forumID = 50

func forumStore(t *testing.T) store.Store {
	t.Helper()
	s := store.NewMemory()
	s.SetChat(domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}})
	return s
}

func topicMsg(id, topic int) domain.Message {
	return domain.Message{ID: id, ChatID: forumID, TopicID: topic, Date: time.Unix(int64(id), 0)}
}

func topic(id int) domain.HistoryKey { return domain.HistoryKey{ChatID: forumID, TopicID: id} }

func ids(msgs []domain.Message) []int {
	out := make([]int, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}

func TestForum_EachTopicIsItsOwnHistory(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(10, 12))
	s.AppendMessage(topicMsg(11, 0))
	s.AppendMessage(topicMsg(12, 12))
	s.AppendMessage(topicMsg(13, 30))

	assert.Equal(t, []int{10, 12}, ids(s.Messages(topic(12))))
	assert.Equal(t, []int{13}, ids(s.Messages(topic(30))))
	assert.Equal(t, []int{11}, ids(s.Messages(topic(domain.GeneralTopicID))),
		"a message naming no topic is in General")
	assert.Empty(t, s.Messages(domain.HistoryKey{ChatID: forumID}),
		"a forum keeps no history of its own")
}

func TestForum_ABusyTopicDoesNotPushOutAQuietOne(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(1, 30))
	for id := 2; id <= store.MaxMessagesPerChat+10; id++ {
		s.AppendMessage(topicMsg(id, 12))
	}

	assert.Len(t, s.Messages(topic(12)), store.MaxMessagesPerChat)
	assert.Equal(t, []int{1}, ids(s.Messages(topic(30))))
}

func TestForum_AWindowOnOneTopicSparesOnlyThatTopic(t *testing.T) {
	s := forumStore(t)
	// The window opened on the newest message there was, and runs to the newest
	// whatever arrives, as a window opened at the first unread does.
	s.SetShown(map[domain.HistoryKey][]store.Shown{
		topic(12): {{Anchor: store.MaxMessagesPerChat + 50, Before: store.MaxMessagesPerChat + 50}},
	})
	page := make([]domain.Message, 0, store.MaxMessagesPerChat+50)
	for id := 1; id <= store.MaxMessagesPerChat+50; id++ {
		page = append(page, topicMsg(id, 12))
	}
	s.MergeOlder(topic(12), 0, page)
	first := 100000
	for id := first; id < first+store.MaxMessagesPerChat+5; id++ {
		s.AppendMessage(topicMsg(id, 30))
	}
	s.AppendMessage(topicMsg(first+store.MaxMessagesPerChat+5, 12))

	assert.Len(t, s.Messages(topic(12)), store.MaxMessagesPerChat+51,
		"the scrollback somebody loaded in one topic survives an arrival")
	assert.Len(t, s.Messages(topic(30)), store.MaxMessagesPerChat,
		"another topic is capped as usual")
}

// The tail is where a forum's gap is measured from, and a forum's gap lies
// across every topic: the newest message of any of them is the tail.
func TestForum_TheTailIsTheNewestOfAnyTopic(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(3, 12))
	s.AppendMessage(topicMsg(9, 30))
	s.AppendMessage(topicMsg(5, 0))

	assert.Equal(t, 9, s.TailMessageID(forumID))
}

// A topic nobody opened this session is held only on disk, and its newest
// message may be newer than anything in memory.
func TestForum_TheTailCountsATopicHeldOnlyOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	chat := domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(chat)
	s.AppendMessage(topicMsg(3, 12))
	s.AppendMessage(topicMsg(9, 30))
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	s2.SetChat(chat)
	s2.LoadMessages(topic(12))

	assert.Equal(t, 9, s2.TailMessageID(forumID))
}

func TestForum_TheForumFlagSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	c, ok := s2.GetChat(forumID)
	require.True(t, ok)
	assert.True(t, c.IsForum)
}

// A group turning topics on or off changes what its history is, and what was
// stored under the old shape cannot be read under the new one. It is discarded
// whole, as a tail reload would discard it, and fetched again.
func TestForum_TurningTopicsOnDiscardsWhatWasStored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	group := domain.Chat{ID: forumID, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}
	flat := domain.HistoryKey{ChatID: forumID}

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(group)
	s.SetMessages(forumID, []domain.Message{topicMsg(1, 0), topicMsg(2, 0)})
	s.MarkGap(forumID, 2)
	s.Flush()

	forum := group
	forum.IsForum = true
	s.SetChat(forum)

	assert.Empty(t, s.Messages(flat))
	s.LoadMessages(flat)
	assert.Empty(t, s.Messages(flat), "the old rows must not come back before they leave the disk")
	_, open := s.Gap(forumID)
	assert.False(t, open, "a gap is a position in the history that was discarded")
	assert.Zero(t, s.TailMessageID(forumID))
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(flat)
	assert.Empty(t, s2.Messages(flat))
}

// The dialog list states the chat as it is, and restating it unchanged is not a
// change of shape.
func TestForum_RestatingAForumKeepsItsHistory(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(1, 12))

	s.SetChat(domain.Chat{ID: forumID, IsForum: true, Title: "renamed", Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}})

	assert.Equal(t, []int{1}, ids(s.Messages(topic(12))))
}

// Telegram addresses a message by its chat and id and never by its topic: an
// edit or a delete in a forum names no topic at all.
func TestForum_AnEditFindsTheMessageInItsTopic(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(1, 12))
	s.AppendMessage(topicMsg(2, 30))

	s.UpdateMessageText(forumID, 2, "edited", nil)

	got := s.Messages(topic(30))
	require.Len(t, got, 1)
	assert.Equal(t, "edited", got[0].Text)
}

func TestForum_ADeleteReachesATopicNobodyOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	chat := domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(chat)
	s.AppendMessage(topicMsg(1, 30))
	s.AppendMessage(topicMsg(2, 30))
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s2.RemoveMessages(forumID, []int{2})
	require.NoError(t, s2.Close())

	s3, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(topic(30))
	assert.Equal(t, []int{1}, ids(s3.Messages(topic(30))))
}

func TestForum_AMessageIsFoundByChatAndID(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(1, 12))
	s.AppendMessage(topicMsg(2, 30))

	m, ok := s.Message(forumID, 2)
	require.True(t, ok)
	assert.Equal(t, 30, m.TopicID)
	_, ok = s.Message(forumID, 3)
	assert.False(t, ok)
}

// An admin can move a message to another topic; the copy that says so wins and
// the message leaves the history it was in. It joins the new one only where a
// run of it lies: anywhere else it is history nobody fetched there, and a
// backfill reaching it brings it with its neighbours.
func TestForum_AMessageMovedToAnotherTopicLeavesTheOld(t *testing.T) {
	s := forumStore(t)
	s.AppendMessage(topicMsg(1, 12))
	s.AppendMessage(topicMsg(2, 12))

	moved := s.AppendMessage(topicMsg(2, 30))

	assert.False(t, moved, "it is the same message, not a new one")
	assert.Equal(t, []int{1}, ids(s.Messages(topic(12))))
	assert.Empty(t, s.Messages(topic(30)), "topic 30 holds no run for it to join")
}

// A page fetched for one topic can carry a message another topic holds or
// should hold. It joins that topic only where a run of it lies, as a moved
// message does: a page asked for one history grows no other.
func TestForum_APageJoinsAnotherTopicOnlyWhereItsRunLies(t *testing.T) {
	s := forumStore(t)
	s.MergeAround(topic(12), []domain.Message{topicMsg(10, 12), topicMsg(20, 12)})

	s.MergeOlder(topic(30), 100, []domain.Message{topicMsg(5, 12), topicMsg(15, 12), topicMsg(90, 30)})

	assert.Equal(t, []int{10, 15, 20}, ids(s.Messages(topic(12))))
	assert.Equal(t, []int{90}, ids(s.Messages(topic(30))))
}

func TestForum_AMessageMovedIntoARunJoinsIt(t *testing.T) {
	s := forumStore(t)
	s.MergeAround(topic(30), []domain.Message{topicMsg(1, 30), topicMsg(9, 30)})
	s.AppendMessage(topicMsg(5, 12))

	s.AppendMessage(topicMsg(5, 30))

	assert.Empty(t, s.Messages(topic(12)))
	assert.Equal(t, []int{1, 5, 9}, ids(s.Messages(topic(30))))
}

// A reload that finds a message in another topic moves it; the move is not a
// delete, whichever topic the reload happens to lay out first.
func TestForum_AReloadThatMovesAMessageKeepsItOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	chat := domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(chat)
	s.SetMessages(forumID, []domain.Message{topicMsg(1, 12), topicMsg(2, 12)})
	s.Flush()
	s.SetMessages(forumID, []domain.Message{topicMsg(1, 12), topicMsg(2, 30)})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(topic(30))
	assert.Equal(t, []int{2}, ids(s2.Messages(topic(30))))
}

// Opening a topic reads that topic's rows and no other: a forum with fifty
// topics would otherwise read all of them to show one.
func TestForum_OpeningATopicLoadsOnlyThatTopic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tele.db")
	chat := domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(chat)
	s.AppendMessage(topicMsg(1, 12))
	s.AppendMessage(topicMsg(2, 30))
	s.AppendMessage(topicMsg(3, 0))
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(topic(12))

	assert.Equal(t, []int{1}, ids(s2.Messages(topic(12))))
	assert.Empty(t, s2.Messages(topic(30)))
	s2.LoadMessages(topic(domain.GeneralTopicID))
	assert.Equal(t, []int{3}, ids(s2.Messages(topic(domain.GeneralTopicID))))
}

func TestForum_ARepairLaysThePageOutAndDeepensNoTopic(t *testing.T) {
	s := forumStore(t)
	held := make([]domain.Message, 0, store.MaxMessagesPerChat)
	for id := 1; id <= store.MaxMessagesPerChat; id++ {
		held = append(held, topicMsg(id, 12))
	}
	s.SetMessages(forumID, held)

	added := s.RepairMessages(forumID, []domain.Message{
		topicMsg(1001, 12), topicMsg(1002, 30), topicMsg(1003, 0),
	})

	require.Equal(t, 3, added)
	got := s.Messages(topic(12))
	assert.Len(t, got, store.MaxMessagesPerChat, "the repaired topic is no deeper than before")
	assert.Equal(t, 1001, got[len(got)-1].ID)
	assert.Equal(t, []int{1002}, ids(s.Messages(topic(30))))
	assert.Equal(t, []int{1003}, ids(s.Messages(topic(domain.GeneralTopicID))))
}
