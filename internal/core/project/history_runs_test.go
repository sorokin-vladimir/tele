package project_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

var chat1 = domain.HistoryKey{ChatID: 1}

func span(lo, hi int) []domain.Message {
	out := make([]domain.Message, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, domain.Message{ID: id, ChatID: 1, Date: time.Unix(int64(id), 0)})
	}
	return out
}

// readerWithRuns holds chat 1 as a run far back, 1..5, and the tail run,
// 100..110.
func readerWithRuns(chat domain.Chat, far domain.Run) *fakeReader {
	return &fakeReader{
		chats: []domain.Chat{chat},
		runs: map[domain.HistoryKey][]domain.Run{chat1: {
			far,
			{Messages: span(100, 110), AtTail: true},
		}},
	}
}

// Two runs have history nobody fetched between them, so a window never shows
// them as adjacent: it stays inside the run its anchor is in.
func TestBuildHistory_AWindowStaysInsideItsRun(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5)})

	got := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 4}, Before: 2, After: 5,
	})

	assert.Equal(t, []int{2, 3, 4, 5}, ids(got.Messages))
	assert.True(t, got.HasOlder)
	assert.False(t, got.HasNewer, "the run ends at 5; what follows was never fetched")
	assert.False(t, got.AtTail)
	assert.False(t, got.AtBeginning)
}

func TestBuildHistory_SaysWhichEndsTheRunReaches(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5), AtBeginning: true})

	far := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 2}, Before: 5, After: 5,
	})
	tail := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 3,
	})

	assert.True(t, far.AtBeginning)
	assert.False(t, far.AtTail)
	assert.True(t, tail.AtTail)
	assert.False(t, tail.AtBeginning)
	assert.Equal(t, []int{107, 108, 109, 110}, ids(tail.Messages))
}

// Unread messages are the newest, so the first unread is looked for on the
// tail run: a far run older than the read pointer is not where it is.
func TestBuildHistory_TheFirstUnreadIsOnTheTailRun(t *testing.T) {
	chat := domain.Chat{ID: 1, UnreadCount: 3, ReadInboxMaxID: 3}
	r := readerWithRuns(chat, domain.Run{Messages: span(1, 5)})

	got := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 1,
	})

	assert.Equal(t, 100, got.AnchorMsgID)
	assert.True(t, got.AtTail)
}

// A history opened at a far message holds no tail run: a window at the tail has
// nothing to show, and the shortfall is what asks for the newest page.
func TestBuildHistory_NoTailRunShowsNothingAtTheTail(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{{ID: 1}},
		runs:  map[domain.HistoryKey][]domain.Run{chat1: {{Messages: span(1, 5)}}},
	}

	got := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 3,
	})

	assert.Empty(t, got.Messages)
	assert.Zero(t, got.AnchorMsgID)
}

// lastNewCount is "N new" as the last history delta of a take carrying
// contents told it, or -1 when none did.
func lastNewCount(t *testing.T, ds []project.Delta) int {
	t.Helper()
	n := -1
	for _, d := range ds {
		if d.History != nil && (d.History.Kind == project.HistoryReset || d.History.Kind == project.HistoryHeaderUpdate) {
			n = d.History.Contents.NewCount
		}
	}
	return n
}

// A window that opens away from the tail is marked with where the tail was,
// and every message landing there afterwards is told as "N new".
func TestRegistry_AWindowAwayFromTheTailIsToldWhatLandsThere(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5)})
	g, rec := newRegistry(r)
	subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 3}, Before: 2, After: 2,
	})
	assert.Zero(t, lastNewCount(t, rec.take()))

	r.runs[chat1][1].Messages = span(100, 112)
	g.Refresh()

	assert.Equal(t, 2, lastNewCount(t, rec.take()), "111 and 112 landed while the window was away")
}

// A history opened far back holds no tail run. The first message to arrive
// starts one, and it is new to the window like every one after it.
func TestRegistry_WithNoTailRunTheFirstArrivalIsCounted(t *testing.T) {
	r := &fakeReader{
		chats: []domain.Chat{{ID: 1}},
		runs:  map[domain.HistoryKey][]domain.Run{chat1: {{Messages: span(1, 5), UpTo: 5}}},
	}
	g, rec := newRegistry(r)
	subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 3}, Before: 2, After: 2,
	})
	rec.take()

	r.runs[chat1] = append(r.runs[chat1], domain.Run{Messages: span(200, 200), AtTail: true, UpTo: 200})
	g.Refresh()

	assert.Equal(t, 1, lastNewCount(t, rec.take()))
}

// Widening a window away from the tail keeps counting from when it left.
func TestRegistry_MovingAWindowKeepsItsMark(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5)})
	g, rec := newRegistry(r)
	w := project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 3}, Before: 1, After: 1,
	}
	id := subscribe(g, w)
	r.runs[chat1][1].Messages = span(100, 112)
	g.Refresh()
	rec.take()

	w.Before, w.After = 2, 2
	g.MoveWindow(id, w)
	moved, _ := g.Window(id)

	assert.Equal(t, 110, moved.(project.HistoryWindow).TailMark)
}

// Once the window's run reaches the tail there is nothing new below it, and
// the mark is gone: leaving again counts from the tail as it is then.
func TestRegistry_ReachingTheTailForgetsTheMark(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5)})
	g, rec := newRegistry(r)
	id := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 3}, Before: 2, After: 2,
	})
	r.runs[chat1][1].Messages = span(100, 112)
	g.Refresh()
	rec.take()

	// The far run grows until it meets the tail run: one run, ending at the tail.
	r.runs[chat1] = []domain.Run{{Messages: append(span(1, 5), span(100, 112)...), AtTail: true}}
	g.Refresh()
	w, _ := g.Window(id)

	assert.Zero(t, lastNewCount(t, rec.take()))
	assert.Zero(t, w.(project.HistoryWindow).TailMark)
}

// "N new" counts what landed on the tail run after the window left it.
func TestBuildHistory_CountsWhatLandedSinceTheWindowLeftTheTail(t *testing.T) {
	r := readerWithRuns(domain.Chat{ID: 1}, domain.Run{Messages: span(1, 5)})

	got := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 3}, Before: 2, After: 2,
		TailMark: 107,
	})
	atTail := project.BuildHistory(r, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 2, TailMark: 107,
	})

	assert.Equal(t, 3, got.NewCount, "108, 109 and 110 landed after 107")
	assert.Zero(t, atTail.NewCount, "a window on the tail run has nothing new below it")
}
