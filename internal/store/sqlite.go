package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
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
	unread_reactions_count INTEGER NOT NULL DEFAULT 0,
	unread_mentions_count INTEGER NOT NULL DEFAULT 0
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
CREATE TABLE IF NOT EXISTS messages (
	chat_id INTEGER NOT NULL,
	msg_id  INTEGER NOT NULL,
	date    INTEGER NOT NULL DEFAULT 0,
	data    TEXT    NOT NULL,
	PRIMARY KEY (chat_id, msg_id)
);
CREATE INDEX IF NOT EXISTS idx_messages_chat_date ON messages(chat_id, date);
CREATE TABLE IF NOT EXISTS chat_gap (
	chat_id      INTEGER PRIMARY KEY,
	after_msg_id INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS addresses (
	chat_id     INTEGER PRIMARY KEY,
	peer_type   INTEGER NOT NULL,
	access_hash INTEGER NOT NULL
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
// domain.Message operations are in-memory only.
type SQLiteStore struct {
	mu       sync.RWMutex
	chats    map[int64]domain.Chat
	messages map[int64][]domain.Message
	// unreadReactionMsgs tracks, per chat, the message IDs observed this session
	// to carry unread reactions. Keeps ApplyUnreadReaction idempotent so repeated
	// updates for one message do not double-count. Session-only: the dialog list
	// is authoritative on restart.
	unreadReactionMsgs map[int64]map[int]struct{}
	// unreadMentionMsgs mirrors unreadReactionMsgs for unread mentions (#155):
	// per chat, the message IDs observed this session to carry an unread mention.
	// Keeps ApplyUnreadMention idempotent; session-only, dialog list authoritative.
	unreadMentionMsgs map[int64]map[int]struct{}
	// baselineUnread holds the last server-authoritative unread count per chat:
	// the dialog-list number, or the value restored from disk on startup.
	// Locally observed unread messages are counted on top of it via unreadMsgs.
	baselineUnread map[int64]int
	// unreadMsgs tracks, per chat, the inbound message IDs observed this session
	// above the read pointer. Keeps unread counting idempotent so a replayed or
	// duplicated update cannot inflate the count. Session-only: the dialog list
	// is authoritative on restart. See issue #189.
	unreadMsgs map[int64]map[int]struct{}
	db         *sql.DB
	log        *zap.Logger

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

	// msgFloor is how many messages a chat was last filled with outright, so the
	// per-chat cap does not trim a scrollback the user deliberately loaded.
	msgFloor map[int64]int

	// dirtyPersist holds chat IDs whose row changed via a high-frequency
	// write-behind mutation (read state, last message) and awaits a coalesced
	// flush. flushStop signals the flusher goroutine to exit; flushDone is closed
	// when it has. See issue #91.
	dirtyPersist map[int64]struct{}
	flushStop    chan struct{}
	flushDone    chan struct{}
	closeOnce    sync.Once

	// loaded marks chats whose persisted message tail has been read from disk
	// into memory, so LoadMessages runs at most once per chat and distinguishes
	// "empty in DB" from "not yet loaded". See issue #139.
	loaded map[int64]bool
	// dirtyMsgs / deletedMsgs queue per-chat message upserts and deletes for the
	// next write-behind flush, mirroring dirtyPersist for chat rows. See #139.
	dirtyMsgs   map[int64]map[int]struct{}
	deletedMsgs map[int64]map[int]struct{}
	// deletingMsgs holds the deletes handed to a flush that has not committed
	// yet. The queue is drained under the lock and written without it, and for
	// that stretch the row is still on disk while nothing marks it deleted — a
	// chat opened right then would read the message back.
	deletingMsgs map[int64]map[int]struct{}
}

// sharedPtsBox reports whether a peer's messages live in the account's common
// pts update box (private chats and basic groups), where message IDs are
// globally unique. Channels and supergroups have their own per-peer ID space.
func sharedPtsBox(p domain.Peer) bool {
	return p.Type == domain.PeerUser || p.Type == domain.PeerGroup
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
		chats:              make(map[int64]domain.Chat),
		messages:           make(map[int64][]domain.Message),
		unreadReactionMsgs: make(map[int64]map[int]struct{}),
		unreadMentionMsgs:  make(map[int64]map[int]struct{}),
		baselineUnread:     make(map[int64]int),
		unreadMsgs:         make(map[int64]map[int]struct{}),
		msgChat:            make(map[int]int64),
		dirtyPersist:       make(map[int64]struct{}),
		flushStop:          make(chan struct{}),
		flushDone:          make(chan struct{}),
		loaded:             make(map[int64]bool),
		dirtyMsgs:          make(map[int64]map[int]struct{}),
		deletedMsgs:        make(map[int64]map[int]struct{}),
		deletingMsgs:       make(map[int64]map[int]struct{}),
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
	pending := make([]domain.Chat, 0, len(s.dirtyPersist))
	for id := range s.dirtyPersist {
		if c, ok := s.chats[id]; ok {
			pending = append(pending, c)
		}
	}
	s.dirtyPersist = make(map[int64]struct{})
	upserts, deletes := s.snapshotMessageWritesLocked()
	s.mu.Unlock()

	for _, c := range pending {
		s.persistChat(c)
	}
	s.flushMessageRows(upserts, deletes)
	s.clearDeletesInFlight(deletes)
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
		{"unread_mentions_count", `ALTER TABLE chats ADD COLUMN unread_mentions_count INTEGER NOT NULL DEFAULT 0`},
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
