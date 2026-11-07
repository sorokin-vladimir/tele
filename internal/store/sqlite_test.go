package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	_ "modernc.org/sqlite"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func newTestSQLite(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.NewSQLite(filepath.Join(t.TempDir(), "state.db"), zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSQLite_SetChat_PersistsSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	log := zap.NewNop()

	s, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	s.SetChat(domain.Chat{
		ID:    42,
		Title: "Hello",
		Peer:  domain.Peer{ID: 42, Type: domain.PeerUser, AccessHash: 999},
	})
	_ = s.Close()

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	chat, ok := s2.GetChat(42)
	assert.True(t, ok)
	assert.Equal(t, "Hello", chat.Title)
	assert.Equal(t, int64(999), chat.Peer.AccessHash)
}

func TestSQLite_LastMessage_PersistsSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	log := zap.NewNop()

	now := time.Unix(1700000000, 0).UTC()
	s, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	s.SetChat(domain.Chat{
		ID:    1,
		Title: "C",
		Peer:  domain.Peer{ID: 1, Type: domain.PeerUser},
		LastMessage: &domain.Message{
			ID:     55,
			ChatID: 1,
			Text:   "hey",
			Date:   now,
		},
	})
	_ = s.Close()

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	chat, ok := s2.GetChat(1)
	assert.True(t, ok)
	require.NotNil(t, chat.LastMessage)
	assert.Equal(t, 55, chat.LastMessage.ID)
	assert.Equal(t, "hey", chat.LastMessage.Text)
	assert.True(t, chat.LastMessage.Date.Equal(now))
}

func TestSQLite_FolderFilters_PersistsSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	log := zap.NewNop()

	filters := []domain.FolderFilter{
		{ID: 1, Title: "Work", Emoji: "💼", Groups: true},
		{ID: 2, Title: "Personal", Contacts: true, ExcludeMuted: true},
	}

	s, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	s.SetFolderFilters(filters)
	_ = s.Close()

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()

	got := s2.FolderFilters()
	require.Len(t, got, 2)
	assert.Equal(t, 1, got[0].ID)
	assert.Equal(t, "Work", got[0].Title)
	assert.True(t, got[0].Groups)
	assert.Equal(t, 2, got[1].ID)
	assert.True(t, got[1].Contacts)
	assert.True(t, got[1].ExcludeMuted)
}

func TestSQLite_FolderFilters_EmptyWhenNotSet(t *testing.T) {
	s := newTestSQLite(t)
	assert.Nil(t, s.FolderFilters())
}

func TestSQLite_Chats_OrderMatchesMemory(t *testing.T) {
	s := newTestSQLite(t)
	now := time.Now()
	s.SetChat(domain.Chat{ID: 1, Title: "A", LastMessage: &domain.Message{Date: now.Add(-1 * time.Minute)}})
	s.SetChat(domain.Chat{ID: 2, Title: "B", LastMessage: &domain.Message{Date: now}})
	s.SetChat(domain.Chat{ID: 3, Title: "Pinned", Pinned: true})

	chats := s.Chats()
	require.Len(t, chats, 3)
	assert.Equal(t, int64(3), chats[0].ID) // pinned first
	assert.Equal(t, int64(2), chats[1].ID) // newest
	assert.Equal(t, int64(1), chats[2].ID)
}

func TestSQLite_Chats_ReordersAfterAppendMessage(t *testing.T) {
	s := newTestSQLite(t)
	now := time.Now()
	s.SetChat(domain.Chat{ID: 1, Title: "A", LastMessage: &domain.Message{Date: now}})
	s.SetChat(domain.Chat{ID: 2, Title: "B", LastMessage: &domain.Message{Date: now.Add(-1 * time.Hour)}})

	// A is newest, so it leads initially.
	require.Equal(t, int64(1), s.Chats()[0].ID)

	// A newer message in B must move it to the top on the next read.
	s.AppendMessage(domain.Message{ID: 9, ChatID: 2, Date: now.Add(1 * time.Hour)})
	assert.Equal(t, int64(2), s.Chats()[0].ID)
}

func TestSQLite_Chats_ReflectsFreshUnreadAndOnlineWithoutReorder(t *testing.T) {
	s := newTestSQLite(t)
	now := time.Now()
	s.SetChat(domain.Chat{ID: 1, Title: "A", LastMessage: &domain.Message{Date: now}})
	s.SetChat(domain.Chat{ID: 2, Title: "B", LastMessage: &domain.Message{Date: now.Add(-1 * time.Hour)}})

	// Prime the order cache.
	require.Equal(t, int64(1), s.Chats()[0].ID)

	// Mutations that do not affect ordering must still be reflected in the
	// cached view (the cache stores order only; field values are read fresh).
	s.ApplyUnreadMessage(1, 100)
	s.UpdateChatOnline(1, true)

	chats := s.Chats()
	require.Equal(t, int64(1), chats[0].ID) // order unchanged
	assert.Equal(t, 1, chats[0].UnreadCount)
	assert.True(t, chats[0].Online)
}

func TestSQLite_RemoveMessagesByID_TargetsOwningChat(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	s.SetChat(domain.Chat{ID: 2, Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	s.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1}})
	s.SetMessages(2, []domain.Message{{ID: 6, ChatID: 2}})

	affected := s.RemoveMessagesByID([]int{5})

	assert.Equal(t, []int64{1}, affected)
	assert.Empty(t, s.Messages(domain.HistoryKey{ChatID: 1}))   // owning chat lost the message
	require.Len(t, s.Messages(domain.HistoryKey{ChatID: 2}), 1) // unrelated chat untouched
}

func TestSQLite_RemoveMessagesByID_IgnoresChannelMessages(t *testing.T) {
	s := newTestSQLite(t)
	// Channel messages live in a per-peer ID space and are deleted with an
	// explicit ChatID, so they are never indexed for the ChatID==0 path.
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerChannel}})
	s.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1}})

	affected := s.RemoveMessagesByID([]int{5})

	assert.Empty(t, affected)
	require.Len(t, s.Messages(domain.HistoryKey{ChatID: 1}), 1) // untouched — not addressable without ChatID
}

func TestSQLite_AppendMessage_CapsHistory(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	const total = store.MaxMessagesPerChat + 100
	for i := 1; i <= total; i++ {
		s.AppendMessage(domain.Message{ID: i, ChatID: 1})
	}
	msgs := s.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, msgs, store.MaxMessagesPerChat)
	// Oldest trimmed from the front, newest retained at the back.
	assert.Equal(t, total-store.MaxMessagesPerChat+1, msgs[0].ID)
	assert.Equal(t, total, msgs[len(msgs)-1].ID)
	// A trimmed message must no longer be resolvable via the index.
	assert.Empty(t, s.RemoveMessagesByID([]int{1}))
}

// SetMessages is the owner saying what a chat's history is, and how deep it goes
// is driven by how far the reader scrolled. Capping it would cap the scrollback:
// the view is built from what the store holds.
func TestSQLite_SetMessages_KeepsWhatItIsGiven(t *testing.T) {
	s := newTestSQLite(t)
	const total = store.MaxMessagesPerChat + 100
	msgs := make([]domain.Message, total)
	for i := range msgs {
		msgs[i] = domain.Message{ID: i + 1, ChatID: 1}
	}

	s.SetMessages(1, msgs)

	got := s.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, total)
	assert.Equal(t, 1, got[0].ID)
	assert.Equal(t, total, got[len(got)-1].ID)
}

// An arriving message must not trim a scrollback the reader deliberately loaded.
func TestSQLite_AppendMessage_DoesNotTrimALoadedScrollback(t *testing.T) {
	s := newTestSQLite(t)
	const total = store.MaxMessagesPerChat + 100
	msgs := make([]domain.Message, total)
	for i := range msgs {
		msgs[i] = domain.Message{ID: i + 1, ChatID: 1}
	}
	s.SetMessages(1, msgs)

	s.AppendMessage(domain.Message{ID: total + 1, ChatID: 1})

	got := s.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, total+1)
	assert.Equal(t, 1, got[0].ID, "the oldest loaded message is still there")
}

func persistedReadInboxMaxID(t *testing.T, s *store.SQLiteStore, id int64) int {
	t.Helper()
	var v int
	err := s.DB().QueryRow(`SELECT read_inbox_max_id FROM chats WHERE id = ?`, id).Scan(&v)
	require.NoError(t, err)
	return v
}

func persistedOnline(t *testing.T, s *store.SQLiteStore, id int64) int {
	t.Helper()
	var v int
	err := s.DB().QueryRow(`SELECT online FROM chats WHERE id = ?`, id).Scan(&v)
	require.NoError(t, err)
	return v
}

func TestSQLite_WriteBehind_ReadStateFlushedOnFlush(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, ReadInboxMaxID: 5})

	// Read-state advance is write-behind: in memory immediately, on disk only
	// after a flush.
	require.True(t, s.UpdateChatReadMaxID(1, 10))
	got, _ := s.GetChat(1)
	assert.Equal(t, 10, got.ReadInboxMaxID)              // in memory
	assert.Equal(t, 5, persistedReadInboxMaxID(t, s, 1)) // not yet on disk

	s.Flush()
	assert.Equal(t, 10, persistedReadInboxMaxID(t, s, 1)) // flushed
}

func TestSQLite_WriteBehind_OnlineNeverPersisted(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})

	require.True(t, s.UpdateChatOnline(1, true))
	got, _ := s.GetChat(1)
	assert.True(t, got.Online) // in memory

	s.Flush()
	assert.Equal(t, 0, persistedOnline(t, s, 1)) // presence is ephemeral, never written
}

func TestSQLite_WriteBehind_FlushesOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	log := zap.NewNop()

	s, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, ReadInboxMaxID: 5})
	require.True(t, s.UpdateChatReadMaxID(1, 42))
	require.NoError(t, s.Close()) // must flush pending write-behind state

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	chat, ok := s2.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 42, chat.ReadInboxMaxID)
}

func TestSQLite_SetChatDraft_UpdatesInMemory(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})

	s.SetChatDraft(1, "unsent draft")
	got, ok := s.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, "unsent draft", got.Draft)

	s.SetChatDraft(1, "")
	got, _ = s.GetChat(1)
	assert.Equal(t, "", got.Draft)
}

func TestSQLite_SetChatDraft_UnknownChatNoOp(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChatDraft(999, "ghost") // must not panic on unknown chat
	_, ok := s.GetChat(999)
	assert.False(t, ok)
}

func TestSQLite_UpdateChatOnline_ReturnsTrueOnFlip(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	assert.True(t, s.UpdateChatOnline(1, true))
}

func TestSQLite_UpdateChatOnline_ReturnsFalseWhenUnchanged(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Title: "Alice", Online: true})
	assert.False(t, s.UpdateChatOnline(1, true))
}

func TestSQLite_UpdateChatOnline_ReturnsFalseWhenMissing(t *testing.T) {
	s := newTestSQLite(t)
	assert.False(t, s.UpdateChatOnline(999, true))
}

func TestSQLite_UpdateChatReadMaxID_ReturnsTrueWhenAdvanced(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Title: "Alice", ReadInboxMaxID: 5})
	assert.True(t, s.UpdateChatReadMaxID(1, 10))
}

func TestSQLite_UpdateChatReadMaxID_ReturnsFalseWhenNotAdvanced(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 1, Title: "Alice", ReadInboxMaxID: 10})
	assert.False(t, s.UpdateChatReadMaxID(1, 10))
}

func TestSQLite_UpdateChatReadMaxID_ReturnsFalseWhenMissing(t *testing.T) {
	s := newTestSQLite(t)
	assert.False(t, s.UpdateChatReadMaxID(999, 10))
}

func TestSQLite_UnreadMarkAndArchived_Persist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 42, Title: "Bob", UnreadMark: true, IsArchived: true})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	c, ok := s2.GetChat(42)
	require.True(t, ok)
	assert.True(t, c.UnreadMark)
	assert.True(t, c.IsArchived)
}

func TestSQLite_MigratesMissingChatColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	// Create a legacy DB whose chats table lacks the new columns.
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE chats (
		id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '',
		peer_type INTEGER NOT NULL DEFAULT 0, peer_access_hash INTEGER NOT NULL DEFAULT 0,
		pinned INTEGER NOT NULL DEFAULT 0, unread_count INTEGER NOT NULL DEFAULT 0,
		read_inbox_max_id INTEGER NOT NULL DEFAULT 0, read_outbox_max_id INTEGER NOT NULL DEFAULT 0,
		last_message TEXT, is_contact INTEGER NOT NULL DEFAULT 0,
		is_bot INTEGER NOT NULL DEFAULT 0, is_muted INTEGER NOT NULL DEFAULT 0,
		online INTEGER NOT NULL DEFAULT 0)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO chats (id, title) VALUES (7, 'Legacy')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// Opening through NewSQLite must migrate and load without error.
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	c, ok := s.GetChat(7)
	require.True(t, ok)
	assert.False(t, c.UnreadMark)
	assert.False(t, c.IsArchived)
	assert.Equal(t, 0, c.UnreadReactionsCount)
	assert.Empty(t, c.Username)
}

// A database written before forums were told apart holds every chat's
// messages with no topic. They must read back as that chat's one history,
// exactly as they did before (#275).
func TestSQLite_LegacyMessagesReadAsTheChatsHistory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE messages (
		chat_id INTEGER NOT NULL, msg_id INTEGER NOT NULL,
		date INTEGER NOT NULL DEFAULT 0, data TEXT NOT NULL,
		PRIMARY KEY (chat_id, msg_id))`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO messages (chat_id, msg_id, date, data) VALUES
		(7, 1, 100, '{"ID":1,"ChatID":7,"Text":"old"}'),
		(7, 2, 200, '{"ID":2,"ChatID":7,"Text":"older build"}')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	s.SetChat(domain.Chat{ID: 7, Title: "Legacy", Peer: domain.Peer{ID: 7, Type: domain.PeerSuperGroup}})
	h := domain.HistoryKey{ChatID: 7}
	s.LoadMessages(h)
	got := s.Messages(h)
	require.Len(t, got, 2)
	assert.Equal(t, "old", got[0].Text)
	assert.Equal(t, 2, s.TailMessageID(7))
}

// A chat's username outlives a restart: a command names a chat by it before
// the dialog list is loaded again (#181).
func TestSQLite_Username_Persists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 42, Title: "Bob", Username: "bob"})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	c, ok := s2.GetChat(42)
	require.True(t, ok)
	assert.Equal(t, "bob", c.Username)
}

func TestSQLite_UnreadReactionsCount_Persist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 42, Title: "Bob", UnreadReactionsCount: 3})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	c, ok := s2.GetChat(42)
	require.True(t, ok)
	assert.Equal(t, 3, c.UnreadReactionsCount)
}

func TestSQLite_UnreadMentionsCount_Persist(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tele.db")

	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 42, Title: "Bob", UnreadMentionsCount: 3})
	require.NoError(t, s.Close())

	s2, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	c, ok := s2.GetChat(42)
	require.True(t, ok)
	assert.Equal(t, 3, c.UnreadMentionsCount)
}

func TestSQLite_ChatStateMutators(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(domain.Chat{ID: 1, Title: "A"})

	s.SetChatMuted(1, true)
	s.SetChatUnreadMark(1, true)
	s.SetChatArchived(1, true)

	c, ok := s.GetChat(1)
	require.True(t, ok)
	assert.True(t, c.IsMuted)
	assert.True(t, c.UnreadMark)
	assert.True(t, c.IsArchived)

	// Missing chat is a no-op, not a panic.
	s.SetChatMuted(999, true)
}
