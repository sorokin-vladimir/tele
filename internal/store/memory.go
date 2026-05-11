package store

import "sync"

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
	return out
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
}
