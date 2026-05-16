package store

import (
	"sort"
	"sync"
	"time"
)

type memoryStore struct {
	mu       sync.RWMutex
	chats    map[int64]Chat
	messages map[int64][]Message
}

func NewMemory() Store {
	return &memoryStore{
		chats:    make(map[int64]Chat),
		messages: make(map[int64][]Message),
	}
}

func (s *memoryStore) GetChat(id int64) (Chat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.chats[id]
	return c, ok
}

func (s *memoryStore) SetChat(chat Chat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chats[chat.ID] = chat
}

func (s *memoryStore) Chats() []Chat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Chat, 0, len(s.chats))
	for _, c := range s.chats {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return lastMsgTime(out[i]).After(lastMsgTime(out[j]))
	})
	return out
}

func lastMsgTime(c Chat) time.Time {
	if c.LastMessage == nil {
		return time.Time{}
	}
	return c.LastMessage.Date
}

func (s *memoryStore) Messages(chatID int64) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages[chatID]
	if msgs == nil {
		return nil
	}
	cp := make([]Message, len(msgs))
	copy(cp, msgs)
	return cp
}

func (s *memoryStore) SetMessages(chatID int64, msgs []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Message, len(msgs))
	copy(cp, msgs)
	s.messages[chatID] = cp
}

func (s *memoryStore) AppendMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[msg.ChatID] = append(s.messages[msg.ChatID], msg)
	if chat, ok := s.chats[msg.ChatID]; ok {
		m := msg
		chat.LastMessage = &m
		s.chats[msg.ChatID] = chat
	}
}

func (s *memoryStore) UpdateMessageID(chatID int64, oldID, newID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[chatID]
	for i := range msgs {
		if msgs[i].ID == oldID {
			msgs[i].ID = newID
			return
		}
	}
}

func (s *memoryStore) RemoveMessage(chatID int64, msgID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[chatID]
	for i, m := range msgs {
		if m.ID == msgID {
			s.messages[chatID] = append(msgs[:i], msgs[i+1:]...)
			return
		}
	}
}

func (s *memoryStore) UpdateChatReadMaxID(chatID int64, maxID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok || maxID <= chat.ReadInboxMaxID {
		return
	}
	chat.ReadInboxMaxID = maxID
	unread := 0
	for _, m := range s.messages[chatID] {
		if !m.IsOut && m.ID > maxID {
			unread++
		}
	}
	chat.UnreadCount = unread
	s.chats[chatID] = chat
}
