package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestMemory_SetGetChat(t *testing.T) {
	s := store.NewMemory()
	chat := store.Chat{ID: 42, Title: "Test"}
	s.SetChat(chat)
	got, ok := s.GetChat(42)
	assert.True(t, ok)
	assert.Equal(t, chat, got)
}

func TestMemory_GetChat_Missing(t *testing.T) {
	s := store.NewMemory()
	_, ok := s.GetChat(999)
	assert.False(t, ok)
}

func TestMemory_Chats_ReturnsAll(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, Title: "A"})
	s.SetChat(store.Chat{ID: 2, Title: "B"})
	assert.Len(t, s.Chats(), 2)
}

func TestMemory_SetGetMessages(t *testing.T) {
	s := store.NewMemory()
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 10, Text: "hello", Date: now},
		{ID: 2, ChatID: 10, Text: "world", Date: now},
	}
	s.SetMessages(10, msgs)
	assert.Len(t, s.Messages(10), 2)
	assert.Empty(t, s.Messages(999))
}

func TestMemory_AppendMessage(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "a"})
	s.AppendMessage(store.Message{ID: 2, ChatID: 5, Text: "b"})
	assert.Len(t, s.Messages(5), 2)
}
