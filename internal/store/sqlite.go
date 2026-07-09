package store

import (
	"database/sql"
	"os"
	"path/filepath"
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
	is_archived        INTEGER NOT NULL DEFAULT 0,
	unread_reactions_count INTEGER NOT NULL DEFAULT 0
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
CREATE TABLE IF NOT EXISTS channel_access_hash (
	user_id     INTEGER NOT NULL,
	channel_id  INTEGER NOT NULL,
	access_hash INTEGER NOT NULL,
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
	// unreadReactionMsgs tracks, per chat, the message IDs observed this session
	// to carry unread reactions. Keeps ApplyUnreadReaction idempotent so repeated
	// updates for one message do not double-count. Session-only: the dialog list
	// is authoritative on restart.
	unreadReactionMsgs map[int64]map[int]struct{}
	db                 *sql.DB
	log                *zap.Logger

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
	// Pin the pool to a single connection. For ":memory:" this keeps the store
	// on one database (each fresh connection opens its own empty in-memory DB).
	// For file-backed databases it serializes writes through one connection so
	// concurrent writers — the updates.Manager's per-channel workers plus the
	// chat store — never collide on SQLITE_BUSY. WAL alone does not prevent that
	// without a busy_timeout, and the resulting failed pts/state writes break
	// channel update recovery after a long idle (#119).
	db.SetMaxOpenConns(1)
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
		chats:              make(map[int64]Chat),
		messages:           make(map[int64][]Message),
		unreadReactionMsgs: make(map[int64]map[int]struct{}),
		msgChat:            make(map[int]int64),
		dirtyPersist:       make(map[int64]struct{}),
		flushStop:          make(chan struct{}),
		flushDone:          make(chan struct{}),
		db:                 db,
		log:                log,
		orderDirty:         true, // build the sorted view lazily on first Chats() call
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
		{"unread_reactions_count", `ALTER TABLE chats ADD COLUMN unread_reactions_count INTEGER NOT NULL DEFAULT 0`},
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
