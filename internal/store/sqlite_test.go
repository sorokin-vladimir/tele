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
	s.SetChat(store.Chat{
		ID:    42,
		Title: "Hello",
		Peer:  store.Peer{ID: 42, Type: store.PeerUser, AccessHash: 999},
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
	s.SetChat(store.Chat{
		ID:    1,
		Title: "C",
		Peer:  store.Peer{ID: 1, Type: store.PeerUser},
		LastMessage: &store.Message{
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

	filters := []store.FolderFilter{
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
	s.SetChat(store.Chat{ID: 1, Title: "A", LastMessage: &store.Message{Date: now.Add(-1 * time.Minute)}})
	s.SetChat(store.Chat{ID: 2, Title: "B", LastMessage: &store.Message{Date: now}})
	s.SetChat(store.Chat{ID: 3, Title: "Pinned", Pinned: true})

	chats := s.Chats()
	require.Len(t, chats, 3)
	assert.Equal(t, int64(3), chats[0].ID) // pinned first
	assert.Equal(t, int64(2), chats[1].ID) // newest
	assert.Equal(t, int64(1), chats[2].ID)
}

func TestSQLite_Chats_ReordersAfterAppendMessage(t *testing.T) {
	s := newTestSQLite(t)
	now := time.Now()
	s.SetChat(store.Chat{ID: 1, Title: "A", LastMessage: &store.Message{Date: now}})
	s.SetChat(store.Chat{ID: 2, Title: "B", LastMessage: &store.Message{Date: now.Add(-1 * time.Hour)}})

	// A is newest, so it leads initially.
	require.Equal(t, int64(1), s.Chats()[0].ID)

	// A newer message in B must move it to the top on the next read.
	s.AppendMessage(store.Message{ID: 9, ChatID: 2, Date: now.Add(1 * time.Hour)})
	assert.Equal(t, int64(2), s.Chats()[0].ID)
}

func TestSQLite_Chats_ReflectsFreshUnreadAndOnlineWithoutReorder(t *testing.T) {
	s := newTestSQLite(t)
	now := time.Now()
	s.SetChat(store.Chat{ID: 1, Title: "A", LastMessage: &store.Message{Date: now}})
	s.SetChat(store.Chat{ID: 2, Title: "B", LastMessage: &store.Message{Date: now.Add(-1 * time.Hour)}})

	// Prime the order cache.
	require.Equal(t, int64(1), s.Chats()[0].ID)

	// Mutations that do not affect ordering must still be reflected in the
	// cached view (the cache stores order only; field values are read fresh).
	s.IncrementChatUnread(1)
	s.UpdateChatOnline(1, true)

	chats := s.Chats()
	require.Equal(t, int64(1), chats[0].ID) // order unchanged
	assert.Equal(t, 1, chats[0].UnreadCount)
	assert.True(t, chats[0].Online)
}

func TestSQLite_RemoveMessagesByID_TargetsOwningChat(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	s.SetChat(store.Chat{ID: 2, Peer: store.Peer{ID: 2, Type: store.PeerUser}})
	s.SetMessages(1, []store.Message{{ID: 5, ChatID: 1}})
	s.SetMessages(2, []store.Message{{ID: 6, ChatID: 2}})

	affected := s.RemoveMessagesByID([]int{5})

	assert.Equal(t, []int64{1}, affected)
	assert.Empty(t, s.Messages(1))   // owning chat lost the message
	require.Len(t, s.Messages(2), 1) // unrelated chat untouched
}

func TestSQLite_RemoveMessagesByID_IgnoresChannelMessages(t *testing.T) {
	s := newTestSQLite(t)
	// Channel messages live in a per-peer ID space and are deleted with an
	// explicit ChatID, so they are never indexed for the ChatID==0 path.
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerChannel}})
	s.SetMessages(1, []store.Message{{ID: 5, ChatID: 1}})

	affected := s.RemoveMessagesByID([]int{5})

	assert.Empty(t, affected)
	require.Len(t, s.Messages(1), 1) // untouched — not addressable without ChatID
}

func TestSQLite_AppendMessage_CapsHistory(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	const total = store.MaxMessagesPerChat + 100
	for i := 1; i <= total; i++ {
		s.AppendMessage(store.Message{ID: i, ChatID: 1})
	}
	msgs := s.Messages(1)
	require.Len(t, msgs, store.MaxMessagesPerChat)
	// Oldest trimmed from the front, newest retained at the back.
	assert.Equal(t, total-store.MaxMessagesPerChat+1, msgs[0].ID)
	assert.Equal(t, total, msgs[len(msgs)-1].ID)
	// A trimmed message must no longer be resolvable via the index.
	assert.Empty(t, s.RemoveMessagesByID([]int{1}))
}

func TestSQLite_SetMessages_CapsHistory(t *testing.T) {
	s := newTestSQLite(t)
	const total = store.MaxMessagesPerChat + 100
	msgs := make([]store.Message, total)
	for i := range msgs {
		msgs[i] = store.Message{ID: i + 1, ChatID: 1}
	}
	s.SetMessages(1, msgs)
	got := s.Messages(1)
	require.Len(t, got, store.MaxMessagesPerChat)
	assert.Equal(t, total-store.MaxMessagesPerChat+1, got[0].ID) // kept the newest tail
	assert.Equal(t, total, got[len(got)-1].ID)
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
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}, ReadInboxMaxID: 5})

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
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})

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
	s.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}, ReadInboxMaxID: 5})
	require.True(t, s.UpdateChatReadMaxID(1, 42))
	require.NoError(t, s.Close()) // must flush pending write-behind state

	s2, err := store.NewSQLite(path, log)
	require.NoError(t, err)
	defer func() { _ = s2.Close() }()
	chat, ok := s2.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 42, chat.ReadInboxMaxID)
}

func TestSQLite_UpdateChatOnline_ReturnsTrueOnFlip(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Title: "Alice"})
	assert.True(t, s.UpdateChatOnline(1, true))
}

func TestSQLite_UpdateChatOnline_ReturnsFalseWhenUnchanged(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Title: "Alice", Online: true})
	assert.False(t, s.UpdateChatOnline(1, true))
}

func TestSQLite_UpdateChatOnline_ReturnsFalseWhenMissing(t *testing.T) {
	s := newTestSQLite(t)
	assert.False(t, s.UpdateChatOnline(999, true))
}

func TestSQLite_UpdateChatReadMaxID_ReturnsTrueWhenAdvanced(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Title: "Alice", ReadInboxMaxID: 5})
	assert.True(t, s.UpdateChatReadMaxID(1, 10))
}

func TestSQLite_UpdateChatReadMaxID_ReturnsFalseWhenNotAdvanced(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(store.Chat{ID: 1, Title: "Alice", ReadInboxMaxID: 10})
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
	s.SetChat(store.Chat{ID: 42, Title: "Bob", UnreadMark: true, IsArchived: true})
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
}

func TestSQLite_ChatStateMutators(t *testing.T) {
	s := store.NewMemory()
	s.SetChat(store.Chat{ID: 1, Title: "A"})

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
