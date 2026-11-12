package store_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func reopened(t *testing.T, path string, h domain.HistoryKey) *store.SQLiteStore {
	t.Helper()
	s := openStore(t, path)
	t.Cleanup(func() { _ = s.Close() })
	s.LoadMessages(h)
	return s
}

func TestRunsPersist_EveryRunSurvivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.LoadMessages(chat7)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(1, 2, 3))
	s.MergeOlder(chat7, 1, nil)
	require.NoError(t, s.Close())

	s2 := reopened(t, path, chat7)

	assert.Equal(t, []runShape{
		{IDs: []int{1, 2, 3}, AtBeginning: true},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s2, chat7))
}

// A database written before runs were recorded holds messages and no runs:
// what it kept then was one stretch ending at the tail.
func TestRunsPersist_RowsWithNoRunsAreOneTailRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.SetMessages(7, msgs(1, 2, 3))
	require.NoError(t, s.Close())
	raw := openStore(t, path)
	_, err := raw.DB().Exec(`DELETE FROM runs`)
	require.NoError(t, err)
	require.NoError(t, raw.Close())

	s2 := reopened(t, path, chat7)

	assert.Equal(t, []runShape{{IDs: []int{1, 2, 3}, AtTail: true}}, runsOf(s2, chat7))
}

// A history nobody opened this session is known here by its arrivals alone.
// They land on the tail run on disk and leave its far runs where they were.
func TestRunsPersist_ArrivalsInAHistoryNotOpenedExtendItsTailRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.LoadMessages(chat7)
	s.SetMessages(7, msgs(100, 101))
	s.MergeAround(chat7, msgs(10, 11))
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	s2.AppendMessage(mergeMsg(102, 102))
	s2.RepairMessages(7, msgs(110, 111))
	require.NoError(t, s2.Close())

	s3 := reopened(t, path, chat7)

	assert.Equal(t, []runShape{
		{IDs: []int{10, 11}},
		{IDs: []int{100, 101, 102, 110, 111}, AtTail: true},
	}, runsOf(s3, chat7))
}

// A tail reload discards every run of the chat, in a forum every topic's,
// those this session never opened as well: after a restart nothing of the old
// history comes back.
func TestRunsPersist_ATailReloadLeavesNothingOldOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	forum := domain.Chat{ID: forumID, IsForum: true, Peer: domain.Peer{ID: forumID, Type: domain.PeerSuperGroup}}
	s := openStore(t, path)
	s.SetChat(forum)
	s.LoadMessages(topic(5))
	s.LoadMessages(topic(6))
	s.SetMessages(forumID, []domain.Message{topicMsg(100, 5), topicMsg(101, 6)})
	s.MergeAround(topic(6), []domain.Message{topicMsg(10, 6), topicMsg(11, 6)})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	s2.LoadMessages(topic(5))
	s2.SetMessages(forumID, []domain.Message{topicMsg(300, 5)})
	require.NoError(t, s2.Close())

	s3 := reopened(t, path, topic(5))
	s3.LoadMessages(topic(6))

	assert.Equal(t, []runShape{{IDs: []int{300}, AtTail: true}}, runsOf(s3, topic(5)))
	assert.Empty(t, runsOf(s3, topic(6)), "topic 6 was not open, and its old history is gone too")
}

// A chat whose shape changed loses its runs on disk with its messages.
func TestRunsPersist_AReshapedChatLeavesNoRunsOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerChannel}})
	s.LoadMessages(chat7)
	s.SetMessages(7, msgs(100, 101))
	s.MergeAround(chat7, msgs(10, 11))
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	s2.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerChannel}})
	s2.SetChat(domain.Chat{ID: 7, IsForum: true, Peer: domain.Peer{ID: 7, Type: domain.PeerChannel}})
	require.NoError(t, s2.Close())

	raw := openStore(t, path)
	t.Cleanup(func() { _ = raw.Close() })
	var n int
	require.NoError(t, raw.DB().QueryRow(`SELECT count(*) FROM runs WHERE chat_id = 7`).Scan(&n))
	assert.Zero(t, n)
}

// Arrivals in a history not opened yet join its tail run the moment it opens,
// in the same session.
func TestRunsPersist_OpeningAHistoryJoinsWhatArrivedBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.LoadMessages(chat7)
	s.SetMessages(7, msgs(100, 101))
	s.MergeAround(chat7, msgs(10, 11))
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	t.Cleanup(func() { _ = s2.Close() })
	s2.AppendMessage(mergeMsg(102, 102))
	s2.LoadMessages(chat7)

	assert.Equal(t, []runShape{
		{IDs: []int{10, 11}},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s2, chat7))
}
