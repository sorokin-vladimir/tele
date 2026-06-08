package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS metadata (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS chats (
	id                 INTEGER PRIMARY KEY,
	title              TEXT    NOT NULL DEFAULT '',
	peer_type          INTEGER NOT NULL DEFAULT 0,
	peer_access_hash   INTEGER NOT NULL DEFAULT 0,
	pinned             INTEGER NOT NULL DEFAULT 0,
	unread_count       INTEGER NOT NULL DEFAULT 0,
	read_inbox_max_id  INTEGER NOT NULL DEFAULT 0,
	read_outbox_max_id INTEGER NOT NULL DEFAULT 0,
	last_message       TEXT,
	is_contact         INTEGER NOT NULL DEFAULT 0,
	is_bot             INTEGER NOT NULL DEFAULT 0,
	is_muted           INTEGER NOT NULL DEFAULT 0,
	online             INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS update_state (
	user_id INTEGER PRIMARY KEY,
	pts     INTEGER NOT NULL DEFAULT 0,
	qts     INTEGER NOT NULL DEFAULT 0,
	date    INTEGER NOT NULL DEFAULT 0,
	seq     INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS channel_pts (
	user_id    INTEGER NOT NULL,
	channel_id INTEGER NOT NULL,
	pts        INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, channel_id)
);
CREATE TABLE IF NOT EXISTS folder_filters (
	key  TEXT PRIMARY KEY DEFAULT 'v1',
	data TEXT NOT NULL DEFAULT '[]'
);
`

// SQLiteStore is a write-through Store backed by a SQLite file.
// Reads are served from an in-memory map; every chat write also persists to disk.
// Message operations are in-memory only.
type SQLiteStore struct {
	mu       sync.RWMutex
	chats    map[int64]Chat
	messages map[int64][]Message
	db       *sql.DB
	log      *zap.Logger
}

// NewSQLite opens (or creates) the SQLite file at path and returns a ready store.
// The caller must call Close() when done.
func NewSQLite(path string, log *zap.Logger) (*SQLiteStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &SQLiteStore{
		chats:    make(map[int64]Chat),
		messages: make(map[int64][]Message),
		db:       db,
		log:      log,
	}
	if err := s.loadChats(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// DB returns the underlying *sql.DB for sharing with other storage adapters (e.g. state storage).
func (s *SQLiteStore) DB() *sql.DB { return s.db }

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) loadChats() error {
	rows, err := s.db.Query(`SELECT id, title, peer_type, peer_access_hash, pinned,
		unread_count, read_inbox_max_id, read_outbox_max_id, last_message,
		is_contact, is_bot, is_muted, online FROM chats`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var c Chat
		var lastMsgJSON []byte
		var pinned, isContact, isBot, isMuted, online int
		err := rows.Scan(
			&c.ID, &c.Title, &c.Peer.Type, &c.Peer.AccessHash,
			&pinned, &c.UnreadCount, &c.ReadInboxMaxID, &c.ReadOutboxMaxID,
			&lastMsgJSON, &isContact, &isBot, &isMuted, &online,
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
		 is_contact, is_bot, is_muted, online)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Peer.Type, c.Peer.AccessHash,
		boolInt(c.Pinned), c.UnreadCount, c.ReadInboxMaxID, c.ReadOutboxMaxID,
		lastMsgJSON,
		boolInt(c.IsContact), boolInt(c.IsBot), boolInt(c.IsMuted), boolInt(c.Online),
	)
	if err != nil {
		s.log.Error("persist chat failed", zap.Int64("chat_id", c.ID), zap.Error(err))
	}
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
	s.mu.Unlock()
	s.persistChat(chat)
}

func (s *SQLiteStore) Chats() []Chat {
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
		return sqliteLastMsgTime(out[i]).After(sqliteLastMsgTime(out[j]))
	})
	return out
}

func sqliteLastMsgTime(c Chat) time.Time {
	if c.LastMessage == nil {
		return time.Time{}
	}
	return c.LastMessage.Date
}

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
	s.messages[chatID] = cp
}

func (s *SQLiteStore) AppendMessage(msg Message) {
	s.mu.Lock()
	s.messages[msg.ChatID] = append(s.messages[msg.ChatID], msg)
	var chat Chat
	var ok bool
	if chat, ok = s.chats[msg.ChatID]; ok {
		m := msg
		chat.LastMessage = &m
		s.chats[msg.ChatID] = chat
	}
	s.mu.Unlock()
	if ok {
		s.persistChat(chat)
	}
}

func (s *SQLiteStore) UpdateMessageID(chatID int64, oldID, newID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.messages[chatID] {
		if s.messages[chatID][i].ID == oldID {
			s.messages[chatID][i].ID = newID
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
			return
		}
	}
}

func (s *SQLiteStore) RemoveMessages(chatID int64, msgIDs []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		if _, remove := toRemove[m.ID]; !remove {
			kept = append(kept, m)
		}
	}
	s.messages[chatID] = kept
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
	s.mu.Unlock()
	s.persistChat(chat)
}

func (s *SQLiteStore) UpdateChatReadMaxID(chatID int64, maxID int) {
	s.mu.Lock()
	chat, ok := s.chats[chatID]
	if !ok || maxID <= chat.ReadInboxMaxID {
		s.mu.Unlock()
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
	s.mu.Unlock()
	s.persistChat(chat)
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
	s.mu.Unlock()
	s.persistChat(chat)
}

func (s *SQLiteStore) UpdateChatOnline(userID int64, online bool) {
	s.mu.Lock()
	chat, ok := s.chats[userID]
	if !ok {
		s.mu.Unlock()
		return
	}
	chat.Online = online
	s.chats[userID] = chat
	s.mu.Unlock()
	s.persistChat(chat)
}

func (s *SQLiteStore) FolderFilters() []FolderFilter {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM folder_filters WHERE key = 'v1'`).Scan(&data)
	if err != nil {
		return nil
	}
	var filters []FolderFilter
	if err := json.Unmarshal(data, &filters); err != nil {
		return nil
	}
	return filters
}

func (s *SQLiteStore) SetFolderFilters(filters []FolderFilter) {
	data, err := json.Marshal(filters)
	if err != nil {
		s.log.Error("marshal folder filters failed", zap.Error(err))
		return
	}
	_, err = s.db.Exec(`INSERT OR REPLACE INTO folder_filters (key, data) VALUES ('v1', ?)`, data)
	if err != nil {
		s.log.Error("persist folder filters failed", zap.Error(err))
	}
}

// ClearForNewAccount clears all account-specific data when ownerID differs from the stored one.
// If no owner is recorded yet (first launch with this version), it just records ownerID.
func (s *SQLiteStore) ClearForNewAccount(ownerID int64) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = 'owner_id'`).Scan(&raw)
	if err == sql.ErrNoRows {
		_, _ = s.db.Exec(`INSERT INTO metadata (key, value) VALUES ('owner_id', ?)`, fmt.Sprint(ownerID))
		return
	}
	if err != nil {
		s.log.Error("read owner_id failed", zap.Error(err))
		return
	}
	var storedID int64
	_, _ = fmt.Sscan(raw, &storedID)
	if storedID == ownerID {
		return
	}

	s.log.Info("account changed, clearing store", zap.Int64("old", storedID), zap.Int64("new", ownerID))

	s.mu.Lock()
	s.chats = make(map[int64]Chat)
	s.messages = make(map[int64][]Message)
	s.mu.Unlock()

	if _, err = s.db.Exec(`DELETE FROM chats`); err != nil {
		s.log.Error("clear chats failed", zap.Error(err))
	}
	if _, err = s.db.Exec(`DELETE FROM folder_filters`); err != nil {
		s.log.Error("clear folder_filters failed", zap.Error(err))
	}
	if _, err = s.db.Exec(`UPDATE metadata SET value = ? WHERE key = 'owner_id'`, fmt.Sprint(ownerID)); err != nil {
		s.log.Error("update owner_id failed", zap.Error(err))
	}
}
