package store

import (
	"encoding/json"
	"sort"
	"time"

	"go.uber.org/zap"
)

func (s *SQLiteStore) loadChats() error {
	rows, err := s.db.Query(`SELECT id, title, peer_type, peer_access_hash, pinned,
		unread_count, read_inbox_max_id, read_outbox_max_id, last_message,
		is_contact, is_bot, is_muted, online, unread_mark, is_archived FROM chats`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c Chat
		var lastMsgJSON []byte
		var pinned, isContact, isBot, isMuted, online, unreadMark, isArchived int
		err := rows.Scan(
			&c.ID, &c.Title, &c.Peer.Type, &c.Peer.AccessHash,
			&pinned, &c.UnreadCount, &c.ReadInboxMaxID, &c.ReadOutboxMaxID,
			&lastMsgJSON, &isContact, &isBot, &isMuted, &online,
			&unreadMark, &isArchived,
		)
		if err != nil {
			return err
		}
		c.Peer.ID = c.ID
		c.Pinned = pinned == 1
		c.IsContact = isContact == 1
		c.IsBot = isBot == 1
		c.IsMuted = isMuted == 1
		c.Online = online == 1
		c.UnreadMark = unreadMark == 1
		c.IsArchived = isArchived == 1
		if len(lastMsgJSON) > 0 {
			var m Message
			if err := json.Unmarshal(lastMsgJSON, &m); err == nil {
				c.LastMessage = &m
			}
		}
		s.chats[c.ID] = c
	}
	return rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// persistChat writes (upserts) a single chat to SQLite. Logs errors; does not return them
// because Store interface methods do not propagate errors.
func (s *SQLiteStore) persistChat(c Chat) {
	var lastMsgJSON []byte
	if c.LastMessage != nil {
		b, _ := json.Marshal(c.LastMessage)
		lastMsgJSON = b
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO chats
		(id, title, peer_type, peer_access_hash, pinned, unread_count,
		 read_inbox_max_id, read_outbox_max_id, last_message,
		 is_contact, is_bot, is_muted, online, unread_mark, is_archived)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Peer.Type, c.Peer.AccessHash,
		boolInt(c.Pinned), c.UnreadCount, c.ReadInboxMaxID, c.ReadOutboxMaxID,
		lastMsgJSON,
		boolInt(c.IsContact), boolInt(c.IsBot), boolInt(c.IsMuted), boolInt(c.Online),
		boolInt(c.UnreadMark), boolInt(c.IsArchived),
	)
	if err != nil {
		s.log.Error("persist chat failed", zap.Int64("chat_id", c.ID), zap.Error(err))
	}
}

// SetChatMuted updates the mute flag for a chat and persists it.
func (s *SQLiteStore) SetChatMuted(chatID int64, muted bool) {
	s.setChatField(chatID, func(c *Chat) { c.IsMuted = muted })
}

// SetChatUnreadMark updates the manual unread-mark flag and persists it.
func (s *SQLiteStore) SetChatUnreadMark(chatID int64, mark bool) {
	s.setChatField(chatID, func(c *Chat) { c.UnreadMark = mark })
}

// SetChatArchived updates the archived flag and persists it.
func (s *SQLiteStore) SetChatArchived(chatID int64, archived bool) {
	s.setChatField(chatID, func(c *Chat) { c.IsArchived = archived })
}

// setChatField applies mutate to a chat under the lock and write-through
// persists it. No-op when the chat is unknown. These flags do not affect
// display order, so the sorted view is not invalidated.
func (s *SQLiteStore) setChatField(chatID int64, mutate func(*Chat)) {
	s.mu.Lock()
	c, ok := s.chats[chatID]
	if !ok {
		s.mu.Unlock()
		return
	}
	mutate(&c)
	s.chats[chatID] = c
	s.mu.Unlock()
	s.persistChat(c)
}

func (s *SQLiteStore) GetChat(id int64) (Chat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.chats[id]
	return c, ok
}

func (s *SQLiteStore) SetChat(chat Chat) {
	s.mu.Lock()
	s.chats[chat.ID] = chat
	s.orderDirty = true // title/pin/last-message may change ordering
	s.mu.Unlock()
	s.persistChat(chat)
}

func (s *SQLiteStore) Chats() []Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orderDirty {
		s.rebuildSortedIDsLocked()
		s.orderDirty = false
	}
	out := make([]Chat, 0, len(s.sortedIDs))
	for _, id := range s.sortedIDs {
		if c, ok := s.chats[id]; ok {
			out = append(out, c)
		}
	}
	return out
}

// rebuildSortedIDsLocked recomputes the cached display order. Caller holds the lock.
func (s *SQLiteStore) rebuildSortedIDsLocked() {
	ids := make([]int64, 0, len(s.chats))
	for id := range s.chats {
		ids = append(ids, id)
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := s.chats[ids[i]], s.chats[ids[j]]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		return sqliteLastMsgTime(a).After(sqliteLastMsgTime(b))
	})
	s.sortedIDs = ids
}

func sqliteLastMsgTime(c Chat) time.Time {
	if c.LastMessage == nil {
		return time.Time{}
	}
	return c.LastMessage.Date
}

func (s *SQLiteStore) IncrementChatUnread(chatID int64) {
	s.mu.Lock()
	chat, ok := s.chats[chatID]
	if !ok {
		s.mu.Unlock()
		return
	}
	chat.UnreadCount++
	s.chats[chatID] = chat
	s.markDirtyLocked(chatID)
	s.mu.Unlock()
}

func (s *SQLiteStore) UpdateChatReadMaxID(chatID int64, maxID int) bool {
	s.mu.Lock()
	chat, ok := s.chats[chatID]
	if !ok || maxID <= chat.ReadInboxMaxID {
		s.mu.Unlock()
		return false
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
	s.markDirtyLocked(chatID)
	s.mu.Unlock()
	return true
}

func (s *SQLiteStore) UpdateChatOutboxReadMaxID(chatID int64, maxID int) {
	s.mu.Lock()
	chat, ok := s.chats[chatID]
	if !ok || maxID <= chat.ReadOutboxMaxID {
		s.mu.Unlock()
		return
	}
	chat.ReadOutboxMaxID = maxID
	s.chats[chatID] = chat
	s.markDirtyLocked(chatID)
	s.mu.Unlock()
}

// UpdateChatOnline updates presence in memory only. Online status is ephemeral
// and high-frequency (one update per contact per status change), so it is never
// persisted. See issue #91.
func (s *SQLiteStore) UpdateChatOnline(userID int64, online bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[userID]
	if !ok || chat.Online == online {
		return false
	}
	chat.Online = online
	s.chats[userID] = chat
	return true
}
