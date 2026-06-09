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
	online             INTEGER NOT NULL DEFAULT 0,
	unread_mark        INTEGER NOT NULL DEFAULT 0,
	is_archived        INTEGER NOT NULL DEFAULT 0
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

// MaxMessagesPerChat bounds how many recent messages are kept in memory per
// chat. The store is a bounded cache of the recent tail; older history is
// re-fetched from the server on demand. See issue #73.
const MaxMessagesPerChat = 500

// persistFlushInterval is how often write-behind chat-row changes (read state,
// last message) are coalesced and flushed to disk. See issue #91.
const persistFlushInterval = 2 * time.Second

// SQLiteStore is a write-through Store backed by a SQLite file.
// Reads are served from an in-memory map; every chat write also persists to disk.
// Message operations are in-memory only.
type SQLiteStore struct {
	mu       sync.RWMutex
	chats    map[int64]Chat
	messages map[int64][]Message
	db       *sql.DB
	log      *zap.Logger

	// sortedIDs caches chat IDs in display order; orderDirty marks it stale.
	// Only the order is cached — field values are always read fresh from the
	// chats map, so non-ordering mutations (online, unread, read state) need no
	// re-sort. See issue #71.
	sortedIDs  []int64
	orderDirty bool

	// msgChat maps a message ID to its owning chat for the delete-without-chatID
	// path. Only private chats and basic groups are indexed (the shared pts box,
	// where message IDs are globally unique); channels/supergroups have their own
	// ID space and are deleted with an explicit ChatID. See issue #72.
	msgChat map[int]int64

	// dirtyPersist holds chat IDs whose row changed via a high-frequency
	// write-behind mutation (read state, last message) and awaits a coalesced
	// flush. flushStop signals the flusher goroutine to exit; flushDone is closed
	// when it has. See issue #91.
	dirtyPersist map[int64]struct{}
	flushStop    chan struct{}
	flushDone    chan struct{}
	closeOnce    sync.Once
}

// sharedPtsBox reports whether a peer's messages live in the account's common
// pts update box (private chats and basic groups), where message IDs are
// globally unique. Channels and supergroups have their own per-peer ID space.
func sharedPtsBox(p Peer) bool {
	return p.Type == PeerUser || p.Type == PeerGroup
}

// NewSQLite opens (or creates) the SQLite file at path and returns a ready store.
// The caller must call Close() when done.
func NewSQLite(path string, log *zap.Logger) (*SQLiteStore, error) {
	inMemory := path == ":memory:"
	if !inMemory {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if inMemory {
		// database/sql keeps a connection pool; each fresh connection to
		// ":memory:" opens its own empty database. Pin the pool to a single
		// connection so the in-memory store stays consistent for its lifetime.
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureChatColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &SQLiteStore{
		chats:        make(map[int64]Chat),
		messages:     make(map[int64][]Message),
		msgChat:      make(map[int]int64),
		dirtyPersist: make(map[int64]struct{}),
		flushStop:    make(chan struct{}),
		flushDone:    make(chan struct{}),
		db:           db,
		log:          log,
		orderDirty:   true, // build the sorted view lazily on first Chats() call
	}
	if err := s.loadChats(); err != nil {
		_ = db.Close()
		return nil, err
	}
	go s.runFlusher()
	return s, nil
}

// runFlusher periodically flushes coalesced write-behind chat-row changes until
// Close signals it to stop. See issue #91.
func (s *SQLiteStore) runFlusher() {
	defer close(s.flushDone)
	ticker := time.NewTicker(persistFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.Flush()
		case <-s.flushStop:
			return
		}
	}
}

// Flush persists every chat marked dirty by write-behind mutations. Snapshots
// are taken under the lock; the disk writes run without it.
func (s *SQLiteStore) Flush() {
	s.mu.Lock()
	if len(s.dirtyPersist) == 0 {
		s.mu.Unlock()
		return
	}
	pending := make([]Chat, 0, len(s.dirtyPersist))
	for id := range s.dirtyPersist {
		if c, ok := s.chats[id]; ok {
			pending = append(pending, c)
		}
	}
	s.dirtyPersist = make(map[int64]struct{})
	s.mu.Unlock()

	for _, c := range pending {
		s.persistChat(c)
	}
}

// markDirtyLocked queues a chat for the next write-behind flush. Caller holds the lock.
func (s *SQLiteStore) markDirtyLocked(chatID int64) {
	s.dirtyPersist[chatID] = struct{}{}
}

// DB returns the underlying *sql.DB for sharing with other storage adapters (e.g. state storage).
func (s *SQLiteStore) DB() *sql.DB { return s.db }

// Close stops the write-behind flusher, persists any pending chat state, and
// closes the underlying database connection. It is idempotent.
func (s *SQLiteStore) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.flushStop)
		<-s.flushDone // wait for the flusher to exit before the final flush
		s.Flush()
		err = s.db.Close()
	})
	return err
}

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

// ensureChatColumns adds chat columns introduced after the original
// schema to pre-existing databases. CREATE TABLE IF NOT EXISTS never
// alters an existing table, so new columns need an explicit ALTER. Each
// ALTER runs only when PRAGMA table_info shows the column is absent, so
// the migration is idempotent.
func ensureChatColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(chats)`)
	if err != nil {
		return err
	}
	existing := make(map[string]struct{})
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return err
		}
		existing[name] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	migrations := []struct{ col, ddl string }{
		{"unread_mark", `ALTER TABLE chats ADD COLUMN unread_mark INTEGER NOT NULL DEFAULT 0`},
		{"is_archived", `ALTER TABLE chats ADD COLUMN is_archived INTEGER NOT NULL DEFAULT 0`},
	}
	for _, m := range migrations {
		if _, ok := existing[m.col]; ok {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return err
		}
	}
	return nil
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
	s.msgChat = make(map[int]int64)
	s.dirtyPersist = make(map[int64]struct{})
	s.sortedIDs = nil
	s.orderDirty = true
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
