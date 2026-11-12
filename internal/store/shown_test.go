package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// A person who scrolled back past the cap is looking at what they scrolled to:
// the window asks for it, and the cap leaves what a window asks for alone.
func TestShown_TheCapSparesWhatAWindowAsksFor(t *testing.T) {
	s := fullTailRun(t)
	s.SetShown(map[domain.HistoryKey][]store.Shown{chat7: {{Before: store.MaxMessagesPerChat + 50}}})

	s.MergeOlder(chat7, 1001, msgs(951, 960, 970, 980, 990, 1000))

	runs := runsOf(s, chat7)
	if assert.Len(t, runs, 1) {
		assert.Equal(t, 951, runs[0].IDs[0], "the page under the window is kept")
	}
}

// A reader anchored on a message nobody kept, older than which a run reaches,
// is shown that run: it is reading what lies just under the message.
func TestShown_AnAnchorNotHeldIsPlacedInTheRunReachingIt(t *testing.T) {
	s := fullTailRun(t)
	s.SetShown(map[domain.HistoryKey][]store.Shown{chat7: {{Anchor: 30, Before: 3}}})

	s.MergeOlder(chat7, 30, msgs(27, 28, 29))

	runs := runsOf(s, chat7)
	if assert.Len(t, runs, 2) {
		assert.Equal(t, []int{27, 28, 29}, runs[0].IDs, "the run under the anchor is kept")
		assert.Equal(t, 1004, runs[1].IDs[0], "the tail run made room instead")
	}
}

func TestShown_WithNobodyLookingTheCapApplies(t *testing.T) {
	s := fullTailRun(t)

	s.MergeOlder(chat7, 1001, msgs(990, 1000))

	assert.Len(t, runsOf(s, chat7)[0].IDs, store.MaxMessagesPerChat)
}
