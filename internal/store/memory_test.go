package store_test

import (
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemory_UpdateMessageMedia(t *testing.T) {
	s := store.NewMemory()
	s.SetMessages(7, []store.Message{
		{ID: 100, ChatID: 7, Photo: &store.PhotoRef{ID: 1, FileReference: []byte("old")}},
	})
	fresh := &store.PhotoRef{ID: 1, FileReference: []byte("new")}
	s.UpdateMessageMedia(7, 100, fresh, nil)
	got := s.Messages(7)
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Photo)
	assert.Equal(t, []byte("new"), got[0].Photo.FileReference)
}

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

func TestMemory_AppendMessage_UpdatesLastMessage(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 5, Title: "Chat"})
	msg := store.Message{ID: 10, ChatID: 5, Text: "hello"}
	s.AppendMessage(msg)
	chat, ok := s.GetChat(5)
	assert.True(t, ok)
	assert.NotNil(t, chat.LastMessage)
	assert.Equal(t, 10, chat.LastMessage.ID)
	assert.Equal(t, "hello", chat.LastMessage.Text)
	// Second append must replace LastMessage
	msg2 := store.Message{ID: 11, ChatID: 5, Text: "world"}
	s.AppendMessage(msg2)
	chat, ok = s.GetChat(5)
	assert.True(t, ok)
	assert.Equal(t, 11, chat.LastMessage.ID)
	assert.Equal(t, "world", chat.LastMessage.Text)
}

func TestMemory_AppendMessage_SkipsLastMessageWhenChatMissing(t *testing.T) {
	s := store.NewMemory()
	// chat 99 is not in store — must not panic
	assert.NotPanics(t, func() {
		s.AppendMessage(store.Message{ID: 1, ChatID: 99, Text: "orphan"})
	})
}

func TestMemory_UpdateMessageID(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: -1, ChatID: 5, Text: "pending"})
	s.UpdateMessageID(5, -1, 100)
	msgs := s.Messages(5)
	require.Len(t, msgs, 1)
	assert.Equal(t, 100, msgs[0].ID)
}

func TestMemory_UpdateMessageID_NoopWhenMissing(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "msg"})
	assert.NotPanics(t, func() {
		s.UpdateMessageID(5, 999, 100)
	})
	msgs := s.Messages(5)
	assert.Equal(t, 1, msgs[0].ID)
}

func TestMemory_UpdateMessageText(t *testing.T) {
	s := store.NewMemory()
	now := time.Now()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "original"})
	s.UpdateMessageText(5, 1, "edited", now)
	msgs := s.Messages(5)
	require.Len(t, msgs, 1)
	assert.Equal(t, "edited", msgs[0].Text)
	require.NotNil(t, msgs[0].EditDate)
}

func TestMemory_UpdateMessageText_NoopWhenMissing(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "msg"})
	assert.NotPanics(t, func() {
		s.UpdateMessageText(5, 999, "x", time.Now())
	})
	msgs := s.Messages(5)
	assert.Equal(t, "msg", msgs[0].Text)
}

func TestMemory_RemoveMessage(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: -1, ChatID: 5, Text: "sentinel"})
	s.AppendMessage(store.Message{ID: 2, ChatID: 5, Text: "other"})
	s.RemoveMessage(5, -1)
	msgs := s.Messages(5)
	require.Len(t, msgs, 1)
	assert.Equal(t, 2, msgs[0].ID)
}

func TestMemory_RemoveMessage_NoopWhenMissing(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "msg"})
	assert.NotPanics(t, func() {
		s.RemoveMessage(5, 999)
	})
	assert.Len(t, s.Messages(5), 1)
}

func TestMemory_RemoveMessages_RemovesMatchingIDs(t *testing.T) {
	s := store.NewMemory()
	now := time.Now()
	s.SetMessages(10, []store.Message{
		{ID: 1, ChatID: 10, Text: "a", Date: now},
		{ID: 2, ChatID: 10, Text: "b", Date: now},
		{ID: 3, ChatID: 10, Text: "c", Date: now},
	})
	s.RemoveMessages(10, []int{1, 3})
	msgs := s.Messages(10)
	require.Len(t, msgs, 1)
	assert.Equal(t, 2, msgs[0].ID)
}

func TestMemory_RemoveMessages_NoopWhenEmpty(t *testing.T) {
	s := store.NewMemory()
	s.RemoveMessages(99, []int{1, 2, 3})
	assert.Empty(t, s.Messages(99))
}

func TestMemory_UpdateMessageReactions_SetsReactions(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "hi"})
	reactions := []store.Reaction{
		{Emoji: "❤️", Count: 3, IsChosen: true},
		{Emoji: "👍", Count: 1, IsChosen: false},
	}
	s.UpdateMessageReactions(5, 1, reactions)
	msgs := s.Messages(5)
	require.Len(t, msgs, 1)
	assert.Equal(t, reactions, msgs[0].Reactions)
}

func TestMemory_UpdateMessageReactions_NoopWhenMissing(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "hi"})
	assert.NotPanics(t, func() {
		s.UpdateMessageReactions(5, 999, []store.Reaction{{Emoji: "👍", Count: 1}})
	})
	msgs := s.Messages(5)
	assert.Empty(t, msgs[0].Reactions)
}

func TestMemory_UpdateChatOnline_SetsOnline(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, Title: "Alice"})
	s.UpdateChatOnline(1, true)
	chat, ok := s.GetChat(1)
	assert.True(t, ok)
	assert.True(t, chat.Online)
}

func TestMemory_UpdateChatOnline_SetsOffline(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, Title: "Alice", Online: true})
	s.UpdateChatOnline(1, false)
	chat, ok := s.GetChat(1)
	assert.True(t, ok)
	assert.False(t, chat.Online)
}

func TestMemory_UpdateChatOnline_NoopWhenMissing(t *testing.T) {
	s := store.NewMemory()
	assert.NotPanics(t, func() {
		s.UpdateChatOnline(999, true)
	})
}

func TestMemory_UpdateMessageReactions_ReplacesExisting(t *testing.T) {
	s := store.NewMemory()
	s.AppendMessage(store.Message{ID: 1, ChatID: 5, Text: "hi",
		Reactions: []store.Reaction{{Emoji: "👍", Count: 2}},
	})
	s.UpdateMessageReactions(5, 1, []store.Reaction{{Emoji: "❤️", Count: 1}})
	msgs := s.Messages(5)
	require.Len(t, msgs[0].Reactions, 1)
	assert.Equal(t, "❤️", msgs[0].Reactions[0].Emoji)
}

func TestMemory_FolderFilters_EmptyInitially(t *testing.T) {
	s := store.NewMemory()
	assert.Nil(t, s.FolderFilters())
}

func TestMemory_FolderFilters_SetAndGet(t *testing.T) {
	s := store.NewMemory()
	filters := []store.FolderFilter{
		{ID: 1, Title: "Work", Groups: true},
		{ID: 2, Title: "Personal", Contacts: true},
	}
	s.SetFolderFilters(filters)
	got := s.FolderFilters()
	require.Len(t, got, 2)
	assert.Equal(t, "Work", got[0].Title)
	assert.Equal(t, "Personal", got[1].Title)
}

func TestMemory_FolderFilters_Replace(t *testing.T) {
	s := store.NewMemory()
	s.SetFolderFilters([]store.FolderFilter{{ID: 1, Title: "Old"}})
	s.SetFolderFilters([]store.FolderFilter{{ID: 2, Title: "New"}})
	got := s.FolderFilters()
	require.Len(t, got, 1)
	assert.Equal(t, "New", got[0].Title)
}
