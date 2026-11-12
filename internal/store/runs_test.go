package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var chat7 = domain.HistoryKey{ChatID: 7}

// runShape is a run as the tests read it: its message ids, oldest first, and
// which ends of the history it reaches.
type runShape struct {
	IDs         []int
	AtTail      bool
	AtBeginning bool
}

func runsOf(s store.Store, h domain.HistoryKey) []runShape {
	var out []runShape
	for _, r := range s.Runs(h) {
		ids := make([]int, 0, len(r.Messages))
		for _, m := range r.Messages {
			ids = append(ids, m.ID)
		}
		out = append(out, runShape{IDs: ids, AtTail: r.AtTail, AtBeginning: r.AtBeginning})
	}
	return out
}

func msgs(ids ...int) []domain.Message {
	out := make([]domain.Message, 0, len(ids))
	for _, id := range ids {
		out = append(out, mergeMsg(id, int64(id)))
	}
	return out
}

func TestRuns_APageAroundAFarMessageIsARunOfItsOwn(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))

	s.MergeAround(chat7, msgs(10, 11, 12))

	assert.Equal(t, []runShape{
		{IDs: []int{10, 11, 12}},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

// A page fetched older than a run's oldest message lies right under it, even
// with ids missing between them: ids skip whatever was deleted.
func TestRuns_AnOlderPageGrowsTheRunItWasFetchedUnder(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	added := s.MergeOlder(chat7, 50, msgs(40, 41, 42))

	assert.Equal(t, 3, added)
	assert.Equal(t, []runShape{
		{IDs: []int{40, 41, 42, 50, 51, 52}},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

func TestRuns_AnOlderPageThatReachesTheRunBelowJoinsIt(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.MergeOlder(chat7, 100, msgs(52, 60, 70))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52, 60, 70, 100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

func TestRuns_NothingOlderMarksTheBeginning(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(1, 2, 3))

	added := s.MergeOlder(chat7, 1, nil)

	assert.Zero(t, added)
	assert.Equal(t, []runShape{
		{IDs: []int{1, 2, 3}, AtBeginning: true},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

// With nothing held, the page older than nothing is the newest Telegram has:
// it ends at the tail.
func TestRuns_TheNewestPageEndsAtTheTail(t *testing.T) {
	s := storeWithChat(t)

	s.MergeOlder(chat7, 0, msgs(5, 6, 7))

	assert.Equal(t, []runShape{{IDs: []int{5, 6, 7}, AtTail: true}}, runsOf(s, chat7))
}

func TestRuns_ANewerPageGrowsTheRunItWasFetchedAbove(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	added := s.MergeNewer(chat7, 52, msgs(60, 61, 62))

	assert.Equal(t, 3, added)
	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52, 60, 61, 62}},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

func TestRuns_ANewerPageThatReachesTheTailRunJoinsIt(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.MergeNewer(chat7, 52, msgs(60, 70, 100))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52, 60, 70, 100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

// Nothing newer than a run's newest message means the run ends at the tail.
// Telegram says so by sliding its answer back over messages already held.
func TestRuns_NothingNewerMeansTheRunEndsAtTheTail(t *testing.T) {
	s := storeWithChat(t)
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.MergeNewer(chat7, 52, msgs(50, 51, 52))

	assert.Equal(t, []runShape{{IDs: []int{50, 51, 52}, AtTail: true}}, runsOf(s, chat7))
}

func TestRuns_ALiveMessageLandsOnTheTailRun(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.AppendMessage(mergeMsg(103, 103))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52}},
		{IDs: []int{100, 101, 102, 103}, AtTail: true},
	}, runsOf(s, chat7))
}

// A history opened at a far message holds no tail run. The first live message
// starts one, apart from the far run: nobody fetched what lies between.
func TestRuns_ALiveMessageWithNoTailRunStartsOne(t *testing.T) {
	s := storeWithChat(t)
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.AppendMessage(mergeMsg(200, 200))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52}},
		{IDs: []int{200}, AtTail: true},
	}, runsOf(s, chat7))
}

// A tail reload admits the stored history cannot be reconciled: every run goes,
// and the fresh page is the one run left.
func TestRuns_ATailReloadLeavesOneRun(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))
	s.MergeOlder(chat7, 50, nil)

	s.SetMessages(7, msgs(300, 301))

	assert.Equal(t, []runShape{{IDs: []int{300, 301}, AtTail: true}}, runsOf(s, chat7))
}

func TestRuns_ATailReloadDiscardsEveryTopicsRuns(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(domain.Chat{ID: 9, IsForum: true, Peer: domain.Peer{ID: 9, Type: domain.PeerChannel}})
	inTopic := func(topic int, ids ...int) []domain.Message {
		out := make([]domain.Message, 0, len(ids))
		for _, id := range ids {
			out = append(out, domain.Message{ID: id, ChatID: 9, TopicID: topic, Date: mergeMsg(id, int64(id)).Date})
		}
		return out
	}
	t5 := domain.HistoryKey{ChatID: 9, TopicID: 5}
	t6 := domain.HistoryKey{ChatID: 9, TopicID: 6}
	s.MergeAround(t5, inTopic(5, 10, 11))
	s.MergeAround(t6, inTopic(6, 20, 21))

	s.SetMessages(9, inTopic(5, 300))

	assert.Equal(t, []runShape{{IDs: []int{300}, AtTail: true}}, runsOf(s, t5))
	assert.Empty(t, runsOf(s, t6))
}

// A chat that became a forum has a different shape of history: its runs go
// with its messages, and a page fetched afterwards does not meet them.
func TestRuns_AReshapedChatKeepsNoRuns(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.SetChat(domain.Chat{ID: 7, IsForum: true, Peer: domain.Peer{ID: 7, Type: domain.PeerChannel}})
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.MergeAround(chat7, msgs(60, 61))

	assert.Equal(t, []runShape{{IDs: []int{60, 61}}}, runsOf(s, chat7))
}

// A run holds its place by its ends, so deleting the message at one end leaves
// the run where it was; a run whose every message was deleted shows nothing.
func TestRuns_DeletesLeaveARunWhereItWas(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))
	s.MergeAround(chat7, msgs(10, 11))

	s.RemoveMessages(7, []int{50, 10, 11})
	s.MergeAround(chat7, msgs(52, 53))

	assert.Equal(t, []runShape{
		{IDs: []int{51, 52, 53}},
		{IDs: []int{100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}

// Repair fetches forward from the gap, which opens after the tail: what it
// brings is the tail run's, and a history with no tail run starts one.
func TestRuns_ARepairPageLandsOnTheTailRun(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.RepairMessages(7, msgs(110, 111))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52}},
		{IDs: []int{100, 101, 102, 110, 111}, AtTail: true},
	}, runsOf(s, chat7))
}

func TestRuns_ARepairPageWithNoTailRunStartsOne(t *testing.T) {
	s := storeWithChat(t)
	s.MergeAround(chat7, msgs(50, 51, 52))

	s.RepairMessages(7, msgs(110, 111))

	assert.Equal(t, []runShape{
		{IDs: []int{50, 51, 52}},
		{IDs: []int{110, 111}, AtTail: true},
	}, runsOf(s, chat7))
}

func fullTailRun(t *testing.T) store.Store {
	t.Helper()
	s := storeWithChat(t)
	ids := make([]int, 0, store.MaxMessagesPerChat)
	for i := 1001; i <= 1000+store.MaxMessagesPerChat; i++ {
		ids = append(ids, i)
	}
	s.SetMessages(7, msgs(ids...))
	return s
}

// The cap gives up a run far back whole before it trims the tail run: half a
// far run is a smaller far run nobody asked for.
func TestRuns_TheCapDropsAFarRunWholeFirst(t *testing.T) {
	s := fullTailRun(t)

	s.MergeAround(chat7, msgs(10, 11, 12))

	runs := runsOf(s, chat7)
	if assert.Len(t, runs, 1) {
		assert.True(t, runs[0].AtTail)
		assert.Len(t, runs[0].IDs, store.MaxMessagesPerChat, "only the far run made room")
		assert.Equal(t, 1001, runs[0].IDs[0], "the tail run was not trimmed")
	}
}

// A far run a window shows is kept; the tail run's old end makes room instead.
func TestShown_AFarRunOnScreenIsKept(t *testing.T) {
	s := fullTailRun(t)
	s.SetShown(map[domain.HistoryKey][]store.Shown{chat7: {{Anchor: 11, Before: 5}}})

	s.MergeAround(chat7, msgs(10, 11, 12))

	runs := runsOf(s, chat7)
	if assert.Len(t, runs, 2) {
		assert.Equal(t, []int{10, 11, 12}, runs[0].IDs)
		assert.Equal(t, 1004, runs[1].IDs[0], "the tail run's three oldest made room")
	}
}

// Trimming stops at the first message a window shows, so the tail run never
// has a hole cut into it.
func TestShown_TheTailRunIsTrimmedOnlyBelowWhatIsShown(t *testing.T) {
	s := fullTailRun(t)
	s.SetShown(map[domain.HistoryKey][]store.Shown{chat7: {{Anchor: 1002}}})

	s.AppendMessage(mergeMsg(2000, 2000))
	s.AppendMessage(mergeMsg(2001, 2001))

	ids := runsOf(s, chat7)[0].IDs
	assert.Equal(t, 1002, ids[0], "only the one message below the shown one went")
	assert.Len(t, ids, store.MaxMessagesPerChat+1)
}

// What is shown is what the owner said last: a window that closed holds
// nothing, and the next write caps as usual.
func TestShown_AClosedWindowHoldsNothing(t *testing.T) {
	s := fullTailRun(t)
	s.SetShown(map[domain.HistoryKey][]store.Shown{chat7: {{Before: store.MaxMessagesPerChat + 50}}})
	s.MergeOlder(chat7, 1001, msgs(990, 1000))

	s.SetShown(nil)
	s.AppendMessage(mergeMsg(2000, 2000))

	ids := runsOf(s, chat7)[0].IDs
	assert.Len(t, ids, store.MaxMessagesPerChat)
	assert.Equal(t, 2000, ids[len(ids)-1])
}

// Trimming the tail run's old end moves where the run starts, so it no longer
// reaches the beginning.
func TestRuns_TrimmingTheTailRunLosesTheBeginning(t *testing.T) {
	s := fullTailRun(t)
	s.MergeOlder(chat7, 1001, nil)

	s.RepairMessages(7, msgs(2000))

	runs := runsOf(s, chat7)
	if assert.Len(t, runs, 1) {
		assert.False(t, runs[0].AtBeginning)
		assert.Equal(t, 1002, runs[0].IDs[0])
	}
}

// A page fetched older than a message reaches up to it, held or not: nothing
// lies between them.
func TestRuns_AnOlderPageReachesUpToTheMessageItWasFetchedUnder(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101))

	s.MergeOlder(chat7, 30, msgs(25, 26, 29))

	runs := s.Runs(chat7)
	if assert.Len(t, runs, 2) {
		assert.Equal(t, 30, runs[0].UpTo)
		assert.Equal(t, 101, runs[1].UpTo)
	}
}

func TestRuns_APageAroundThatMeetsARunJoinsIt(t *testing.T) {
	s := storeWithChat(t)
	s.SetMessages(7, msgs(100, 101, 102))
	s.MergeAround(chat7, msgs(10, 11, 12))

	s.MergeAround(chat7, msgs(12, 13, 14))
	s.MergeAround(chat7, msgs(98, 99, 100))

	assert.Equal(t, []runShape{
		{IDs: []int{10, 11, 12, 13, 14}},
		{IDs: []int{98, 99, 100, 101, 102}, AtTail: true},
	}, runsOf(s, chat7))
}
