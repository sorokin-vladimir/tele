package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestApplyUnreadReaction_Idempotent(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, Title: "A"})

	assert.True(t, s.ApplyUnreadReaction(1, 100, true))
	c, _ := s.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)

	// Same message again: already tracked, no change.
	assert.False(t, s.ApplyUnreadReaction(1, 100, true))
	c, _ = s.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)

	// Different message: +1.
	assert.True(t, s.ApplyUnreadReaction(1, 200, true))
	c, _ = s.GetChat(1)
	assert.Equal(t, 2, c.UnreadReactionsCount)
}

func TestApplyUnreadReaction_ClearDecrementsFloorZero(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1})
	s.ApplyUnreadReaction(1, 100, true)

	assert.True(t, s.ApplyUnreadReaction(1, 100, false))
	c, _ := s.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)

	// Clearing an untracked message: no change, no negative count.
	assert.False(t, s.ApplyUnreadReaction(1, 999, false))
	c, _ = s.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)
}

func TestApplyUnreadReaction_UnknownChatNoop(t *testing.T) {
	s := store.NewMemory()
	assert.False(t, s.ApplyUnreadReaction(42, 1, true))
}

func TestSetChatReactionsRead_ClearsCountAndSet(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, UnreadReactionsCount: 3})
	s.ApplyUnreadReaction(1, 100, true) // count 4, tracked {100}

	s.SetChatReactionsRead(1)
	c, _ := s.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)

	// Set was cleared: re-adding the same message increments from 0.
	assert.True(t, s.ApplyUnreadReaction(1, 100, true))
	c, _ = s.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)
}

func TestSetChat_ResetsReactionTracking(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1})
	s.ApplyUnreadReaction(1, 100, true)

	// Dialog refresh overwrites the chat with a fresh server count.
	s.SetChat(store.Chat{ID: 1, UnreadReactionsCount: 5})

	// Tracked set was cleared by SetChat, so re-adding 100 increments to 6.
	assert.True(t, s.ApplyUnreadReaction(1, 100, true))
	c, _ := s.GetChat(1)
	assert.Equal(t, 6, c.UnreadReactionsCount)
}
