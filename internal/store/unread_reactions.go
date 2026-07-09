package store

// ApplyUnreadReaction idempotently adjusts a chat's unread-reaction count from a
// per-message signal. Marking a not-yet-tracked message increments the count;
// clearing a tracked message decrements it (floored at 0). Returns true when the
// count changed. No-op (false) for unknown chats.
func (s *SQLiteStore) ApplyUnreadReaction(chatID int64, msgID int, hasUnread bool) bool {
	s.mu.Lock()
	c, ok := s.chats[chatID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	tracked := s.unreadReactionMsgs[chatID]
	_, isTracked := tracked[msgID]

	changed := false
	switch {
	case hasUnread && !isTracked:
		if tracked == nil {
			tracked = make(map[int]struct{})
			s.unreadReactionMsgs[chatID] = tracked
		}
		tracked[msgID] = struct{}{}
		c.UnreadReactionsCount++
		changed = true
	case !hasUnread && isTracked:
		delete(tracked, msgID)
		if c.UnreadReactionsCount > 0 {
			c.UnreadReactionsCount--
		}
		changed = true
	}
	if changed {
		s.chats[chatID] = c
	}
	s.mu.Unlock()
	if changed {
		s.persistChat(c)
	}
	return changed
}

// SetChatReactionsRead clears a chat's unread-reaction count and its tracked
// message set, then persists. No-op for unknown chats.
func (s *SQLiteStore) SetChatReactionsRead(chatID int64) {
	s.mu.Lock()
	c, ok := s.chats[chatID]
	if !ok {
		s.mu.Unlock()
		return
	}
	delete(s.unreadReactionMsgs, chatID)
	c.UnreadReactionsCount = 0
	s.chats[chatID] = c
	s.mu.Unlock()
	s.persistChat(c)
}
