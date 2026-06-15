package store

import "time"

// Message operations are in-memory only — messages load on demand per chat open.

func (s *SQLiteStore) Messages(chatID int64) []Message {
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

func (s *SQLiteStore) SetMessages(chatID int64, msgs []Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]Message, len(msgs))
	copy(cp, msgs)
	// Re-index this chat: drop entries for the replaced messages, then add the
	// new ones if the chat lives in the shared pts box.
	for _, m := range s.messages[chatID] {
		delete(s.msgChat, m.ID)
	}
	s.messages[chatID] = cp
	s.capMessagesLocked(chatID)
	if chat, ok := s.chats[chatID]; ok && sharedPtsBox(chat.Peer) {
		for _, m := range s.messages[chatID] {
			s.msgChat[m.ID] = chatID
		}
	}
}

// capMessagesLocked trims a chat's message slice to the newest MaxMessagesPerChat,
// dropping the oldest from the front and clearing their index entries. Caller
// holds the lock. See issue #73.
func (s *SQLiteStore) capMessagesLocked(chatID int64) {
	msgs := s.messages[chatID]
	if len(msgs) <= MaxMessagesPerChat {
		return
	}
	drop := len(msgs) - MaxMessagesPerChat
	for _, m := range msgs[:drop] {
		delete(s.msgChat, m.ID)
	}
	s.messages[chatID] = msgs[drop:]
}

func (s *SQLiteStore) AppendMessage(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages[msg.ChatID] = append(s.messages[msg.ChatID], msg)
	if chat, ok := s.chats[msg.ChatID]; ok {
		m := msg
		chat.LastMessage = &m
		s.chats[msg.ChatID] = chat
		s.orderDirty = true // newer last-message moves the chat in the list
		if sharedPtsBox(chat.Peer) {
			s.msgChat[msg.ID] = msg.ChatID
		}
		s.markDirtyLocked(msg.ChatID) // write-behind: last-message persists on flush
	}
	s.capMessagesLocked(msg.ChatID)
}

func (s *SQLiteStore) UpdateMessageID(chatID int64, oldID, newID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.messages[chatID] {
		if s.messages[chatID][i].ID == oldID {
			s.messages[chatID][i].ID = newID
			if cid, ok := s.msgChat[oldID]; ok {
				delete(s.msgChat, oldID)
				s.msgChat[newID] = cid
			}
			return
		}
	}
}

func (s *SQLiteStore) UpdateMessageText(chatID int64, msgID int, text string, editDate time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.messages[chatID] {
		if s.messages[chatID][i].ID == msgID {
			s.messages[chatID][i].Text = text
			t := editDate
			s.messages[chatID][i].EditDate = &t
			return
		}
	}
}

func (s *SQLiteStore) UpdateMessageReactions(chatID int64, msgID int, reactions []Reaction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.messages[chatID] {
		if s.messages[chatID][i].ID == msgID {
			cp := make([]Reaction, len(reactions))
			copy(cp, reactions)
			s.messages[chatID][i].Reactions = cp
			return
		}
	}
}

// UpdateMessageMedia replaces the photo/document refs of a cached message. A nil
// ref leaves that field unchanged.
func (s *SQLiteStore) UpdateMessageMedia(chatID int64, msgID int, photo *PhotoRef, document *DocumentRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.messages[chatID] {
		if s.messages[chatID][i].ID == msgID {
			if photo != nil {
				s.messages[chatID][i].Photo = photo
			}
			if document != nil {
				s.messages[chatID][i].Document = document
			}
			return
		}
	}
}

func (s *SQLiteStore) RemoveMessage(chatID int64, msgID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.messages[chatID]
	for i, m := range msgs {
		if m.ID == msgID {
			s.messages[chatID] = append(msgs[:i], msgs[i+1:]...)
			delete(s.msgChat, msgID)
			return
		}
	}
}

func (s *SQLiteStore) RemoveMessages(chatID int64, msgIDs []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeMessagesLocked(chatID, msgIDs)
}

// removeMessagesLocked drops the given message IDs from one chat and the msgChat
// index. Caller holds the lock.
func (s *SQLiteStore) removeMessagesLocked(chatID int64, msgIDs []int) {
	if len(s.messages[chatID]) == 0 {
		return
	}
	toRemove := make(map[int]struct{}, len(msgIDs))
	for _, id := range msgIDs {
		toRemove[id] = struct{}{}
	}
	msgs := s.messages[chatID]
	kept := msgs[:0]
	for _, m := range msgs {
		if _, remove := toRemove[m.ID]; remove {
			delete(s.msgChat, m.ID)
			continue
		}
		kept = append(kept, m)
	}
	s.messages[chatID] = kept
}

// RemoveMessagesByID resolves each message ID to its owning chat via the index
// and removes it there, returning the affected chat IDs. Used for the Telegram
// non-channel delete that carries message IDs but no peer context (issue #72).
func (s *SQLiteStore) RemoveMessagesByID(msgIDs []int) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	byChat := make(map[int64][]int)
	for _, id := range msgIDs {
		if cid, ok := s.msgChat[id]; ok {
			byChat[cid] = append(byChat[cid], id)
		}
	}
	affected := make([]int64, 0, len(byChat))
	for cid, ids := range byChat {
		s.removeMessagesLocked(cid, ids)
		affected = append(affected, cid)
	}
	return affected
}
