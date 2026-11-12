package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func mergeMsg(id int, sec int64) domain.Message {
	return domain.Message{ID: id, ChatID: 7, Date: time.Unix(sec, 0)}
}

func storeWithChat(t *testing.T) store.Store {
	t.Helper()
	s := store.NewMemory()
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	return s
}

func TestMergeOlder_ReportsWhatThePageAdded(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, []domain.Message{mergeMsg(3, 3), mergeMsg(4, 4)})

	added := s.MergeOlder(domain.HistoryKey{ChatID: 7}, 3, []domain.Message{mergeMsg(1, 1), mergeMsg(2, 2), mergeMsg(3, 3)})

	assert.Equal(t, 2, added, "the page overlapped the store by one message")
	assert.Equal(t, []int{1, 2, 3, 4}, storedIDs(s, 7))
}

func TestMergeOlder_APageOfWhatIsHeldAddsNothing(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, []domain.Message{mergeMsg(1, 1), mergeMsg(2, 2)})

	added := s.MergeOlder(domain.HistoryKey{ChatID: 7}, 1, []domain.Message{mergeMsg(1, 1), mergeMsg(2, 2)})

	assert.Zero(t, added)
	assert.Equal(t, []int{1, 2}, storedIDs(s, 7))
}

// The reason the merge lives in the store at all. A fetch reads the held
// history, spends a round trip on the network and comes back with a page: a
// message that arrived in the meantime is in the store but not in what the
// caller read, and a plain SetMessages would write it back out of existence.
func TestMergeOlder_KeepsAMessageThatArrivedDuringTheFetch(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, []domain.Message{mergeMsg(5, 5)})

	// What the caller read before it went to the network.
	_ = s.Messages(domain.HistoryKey{ChatID: 7})
	// What arrived while it was there.
	s.AppendMessage(mergeMsg(6, 6))
	// What it came back with.
	added := s.MergeOlder(domain.HistoryKey{ChatID: 7}, 5, []domain.Message{mergeMsg(3, 3), mergeMsg(4, 4)})

	require.Equal(t, 2, added)
	assert.Equal(t, []int{3, 4, 5, 6}, storedIDs(s, 7), "the arrival must survive the page")
}

func TestMergeOlder_TakesTheFetchedCopyOfAMessageEditedInTheGap(t *testing.T) {
	s := storeWithChat(t)
	stale := mergeMsg(1, 1)
	stale.Text = "before"
	s.SetMessages(7, []domain.Message{stale})

	fresh := mergeMsg(1, 1)
	fresh.Text = "after"
	added := s.MergeOlder(domain.HistoryKey{ChatID: 7}, 0, []domain.Message{fresh})

	assert.Zero(t, added, "an edit changes no count")
	held := s.Messages(domain.HistoryKey{ChatID: 7})
	require.Len(t, held, 1)
	assert.Equal(t, "after", held[0].Text)
}

func fullChat(t *testing.T) store.Store {
	t.Helper()
	s := storeWithChat(t)
	held := make([]domain.Message, 0, store.MaxMessagesPerChat)
	for i := 1; i <= store.MaxMessagesPerChat; i++ {
		held = append(held, mergeMsg(i, int64(i)))
	}
	s.SetMessages(7, held)
	return s
}

// Scrolling back is somebody asking for more history, so the chat is allowed to
// grow past the cap and hold what their window asks for.
func TestMergeOlder_DeepensAChatThatIsAlreadyFull(t *testing.T) {
	s := storeWithChat(t)
	held := make([]domain.Message, 0, store.MaxMessagesPerChat)
	for i := 3; i < 3+store.MaxMessagesPerChat; i++ {
		held = append(held, mergeMsg(i, int64(i)))
	}
	s.SetMessages(7, held)
	s.SetShown(map[domain.HistoryKey][]store.Shown{{ChatID: 7}: {{Before: store.MaxMessagesPerChat + 50}}})

	added := s.MergeOlder(domain.HistoryKey{ChatID: 7}, 3, []domain.Message{mergeMsg(1, 1), mergeMsg(2, 2)})

	assert.Equal(t, 2, added)
	assert.Len(t, s.Messages(domain.HistoryKey{ChatID: 7}), store.MaxMessagesPerChat+2)
}

// A repair is nobody's request. It must not leave the chat costing more to hold
// than it did before the hole opened: with nobody looking, the oldest make room.
func TestRepairMessages_LeavesTheChatNoDeeper(t *testing.T) {
	s := fullChat(t)

	added := s.RepairMessages(7, []domain.Message{mergeMsg(1001, 1001), mergeMsg(1002, 1002)})

	assert.Equal(t, 2, added, "the page is counted before the cap trims the other end")
	got := s.Messages(domain.HistoryKey{ChatID: 7})
	require.Len(t, got, store.MaxMessagesPerChat)
	assert.Equal(t, 1002, got[len(got)-1].ID, "the repaired range is held")
	assert.Equal(t, 3, got[0].ID, "the two oldest made room for it")
}

func storedIDs(s store.Store, chatID int64) []int {
	msgs := s.Messages(domain.HistoryKey{ChatID: chatID})
	out := make([]int, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.ID)
	}
	return out
}
