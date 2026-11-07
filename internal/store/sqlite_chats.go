package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"go.uber.org/zap"
)

func (s *SQLiteStore) loadChats() error {
	rows, err := s.db.Query(`SELECT id, title, peer_type, peer_access_hash, pinned,
		unread_count, read_inbox_max_id, read_outbox_max_id, last_message,
		is_contact, is_bot, is_muted, online, unread_mark, is_archived,
		unread_reactions_count, unread_mentions_count, is_forum, username FROM chats`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c domain.Chat
		var lastMsgJSON []byte
		var pinned, isContact, isBot, isMuted, online, unreadMark, isArchived, isForum int
		err := rows.Scan(
			&c.ID, &c.Title, &c.Peer.Type, &c.Peer.AccessHash,
			&pinned, &c.UnreadCount, &c.ReadInboxMaxID, &c.ReadOutboxMaxID,
			&lastMsgJSON, &isContact, &isBot, &isMuted, &online,
			&unreadMark, &isArchived, &c.UnreadReactionsCount, &c.UnreadMentionsCount,
			&isForum, &c.Username,
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
		c.IsForum = isForum == 1
		if len(lastMsgJSON) > 0 {
			var m domain.Message
			if err := json.Unmarshal(lastMsgJSON, &m); err == nil {
				c.LastMessage = &m
			}
		}
		s.chats[c.ID] = c
		// The persisted count is the baseline for this session; the tracked set
		// starts empty, so the restored value is reproduced exactly.
		s.baselineUnread[c.ID] = c.UnreadCount
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
func (s *SQLiteStore) persistChat(c domain.Chat) {
	var lastMsgJSON []byte
	if c.LastMessage != nil {
		b, _ := json.Marshal(c.LastMessage)
		lastMsgJSON = b
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO chats
		(id, title, peer_type, peer_access_hash, pinned, unread_count,
		 read_inbox_max_id, read_outbox_max_id, last_message,
		 is_contact, is_bot, is_muted, online, unread_mark, is_archived,
		 unread_reactions_count, unread_mentions_count, is_forum, username)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Peer.Type, c.Peer.AccessHash,
		boolInt(c.Pinned), c.UnreadCount, c.ReadInboxMaxID, c.ReadOutboxMaxID,
		lastMsgJSON,
		boolInt(c.IsContact), boolInt(c.IsBot), boolInt(c.IsMuted), boolInt(c.Online),
		boolInt(c.UnreadMark), boolInt(c.IsArchived), c.UnreadReactionsCount, c.UnreadMentionsCount,
		boolInt(c.IsForum), c.Username,
	)
	if err != nil {
		s.log.Error("persist chat failed", zap.Int64("chat_id", c.ID), zap.Error(err))
	}
}

// SetChatMuted updates the mute flag for a chat and persists it.
func (s *SQLiteStore) SetChatMuted(chatID int64, muted bool) {
	s.setChatField(chatID, func(c *domain.Chat) { c.IsMuted = muted })
}

// SetChatUnreadMark updates the manual unread-mark flag and persists it.
func (s *SQLiteStore) SetChatUnreadMark(chatID int64, mark bool) {
	s.setChatField(chatID, func(c *domain.Chat) { c.UnreadMark = mark })
}

// SetChatArchived updates the archived flag and persists it.
func (s *SQLiteStore) SetChatArchived(chatID int64, archived bool) {
	s.setChatField(chatID, func(c *domain.Chat) { c.IsArchived = archived })
}

// SetChatDraft updates the synced draft for a chat in memory only (#62). Drafts
// are not a persisted column — they are reloaded from the dialog list on each
// startup — so this deliberately skips persistChat. No-op for unknown chats.
func (s *SQLiteStore) SetChatDraft(chatID int64, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chats[chatID]
	if !ok {
		return
	}
	c.Draft = text
	s.chats[chatID] = c
}

// setChatField applies mutate to a chat under the lock and write-through
// persists it. No-op when the chat is unknown. These flags do not affect
// display order, so the sorted view is not invalidated.
func (s *SQLiteStore) setChatField(chatID int64, mutate func(*domain.Chat)) {
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

func (s *SQLiteStore) GetChat(id int64) (domain.Chat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.chats[id]
	return c, ok
}

// SetChat stores a chat as Telegram states it. A chat whose forum flag changed
// has a different shape of history than what is stored for it, so what is
// stored is discarded, its gap with it (#275).
func (s *SQLiteStore) SetChat(chat domain.Chat) {
	s.mu.Lock()
	prev, known := s.chats[chat.ID]
	reshaped := known && prev.IsForum != chat.IsForum
	if reshaped {
		s.discardChatLocked(chat.ID)
	}
	s.chats[chat.ID] = chat
	s.baselineUnread[chat.ID] = chat.UnreadCount // dialog-list count is authoritative
	delete(s.unreadMsgs, chat.ID)                // its messages are already inside that count
	delete(s.unreadReactionMsgs, chat.ID)        // server count is authoritative; drop stale session tracking
	delete(s.unreadMentionMsgs, chat.ID)         // same: mention count comes fresh from the dialog list
	s.orderDirty = true                          // title/pin/last-message may change ordering
	s.mu.Unlock()
	s.persistChat(chat)
	if reshaped {
		s.ClearGap(chat.ID)
	}
}

func (s *SQLiteStore) Chats() []domain.Chat {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orderDirty {
		s.rebuildSortedIDsLocked()
		s.orderDirty = false
	}
	out := make([]domain.Chat, 0, len(s.sortedIDs))
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

func sqliteLastMsgTime(c domain.Chat) time.Time {
	if c.LastMessage == nil {
		return time.Time{}
	}
	return c.LastMessage.Date
}

// UpdateChatReadMaxID advances a chat's read pointer and reconciles its unread
// count, returning false when the pointer did not move. Reconciliation prunes
// the tracked message set rather than scanning s.messages, because messages load
// lazily and a chat never opened this session has none in memory (#189).
//
// The server baseline is a bare count, so what an advancing pointer took out of
// it has to be worked out: the messages between the old and the new pointer are
// subtracted when they are known, the whole baseline is cleared when the pointer
// reaches the newest message, and only when the range is unknown does the stale
// baseline get dropped wholesale — the next dialog-list sync restores it.
// Anything observed locally above maxID survives regardless.
func (s *SQLiteStore) UpdateChatReadMaxID(chatID int64, maxID int) bool {
	s.mu.Lock()
	chat, ok := s.chats[chatID]
	if !ok || maxID <= chat.ReadInboxMaxID {
		s.mu.Unlock()
		return false
	}
	read, known := s.countBaselineReadLocked(chatID, chat.ReadInboxMaxID, maxID)
	chat.ReadInboxMaxID = maxID
	for id := range s.unreadMsgs[chatID] {
		if id <= maxID {
			delete(s.unreadMsgs[chatID], id)
		}
	}
	switch {
	case chat.LastMessage != nil && maxID >= chat.LastMessage.ID:
		s.baselineUnread[chatID] = 0 // read to the end: nothing can be left
	case !known:
		s.baselineUnread[chatID] = 0
	case read >= s.baselineUnread[chatID]:
		s.baselineUnread[chatID] = 0
	default:
		s.baselineUnread[chatID] -= read
	}
	s.recomputeUnreadLocked(&chat)
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
