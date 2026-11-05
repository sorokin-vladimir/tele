package store_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func openStore(t *testing.T, path string) *store.SQLiteStore {
	t.Helper()
	s, err := store.NewSQLite(path, zap.NewNop())
	require.NoError(t, err)
	return s
}

func TestSQLite_Messages_PersistAndReloadAfterReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 7, Peer: domain.Peer{ID: 7, Type: domain.PeerUser}})
	s.SetMessages(7, []domain.Message{
		{ID: 1, ChatID: 7, Text: "hello", Date: time.Unix(1000, 0)},
		{ID: 2, ChatID: 7, Text: "world", Date: time.Unix(2000, 0)},
	})
	require.NoError(t, s.Close()) // Close flushes pending write-behind

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 7})
	got := s2.Messages(domain.HistoryKey{ChatID: 7})

	require.Len(t, got, 2)
	assert.Equal(t, "hello", got[0].Text)
	assert.Equal(t, "world", got[1].Text)
}

func TestSQLite_AppendMessage_PersistsSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})
	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "appended", Date: time.Unix(3000, 0)})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 9})
	got := s2.Messages(domain.HistoryKey{ChatID: 9})

	require.Len(t, got, 1)
	assert.Equal(t, "appended", got[0].Text)
}

func TestSQLite_MessageWithRich_PersistsSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	rich := &domain.RichMessage{Blocks: []domain.RichBlock{
		{Kind: domain.RichParagraph, Text: domain.RichText{Kind: domain.RichTextPlain, Text: "structured"}},
		{Kind: domain.RichTable, Bordered: true, Rows: [][]domain.RichCell{
			{{Text: domain.RichText{Kind: domain.RichTextPlain, Text: "Name"}}},
		}},
	}}

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 11, Peer: domain.Peer{ID: 11, Type: domain.PeerUser}})
	s.SetMessages(11, []domain.Message{{
		ID: 5, ChatID: 11, Text: "fallback", Date: time.Unix(3000, 0), Rich: rich,
	}})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 11})
	got := s2.Messages(domain.HistoryKey{ChatID: 11})

	require.Len(t, got, 1)
	assert.Equal(t, rich, got[0].Rich, "the structured blocks survive a save and a load")
}

func TestSQLite_CapTrim_DeletesOldestOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 3, Peer: domain.Peer{ID: 3, Type: domain.PeerUser}})
	// One past the cap so the oldest (id 1) is trimmed.
	msgs := make([]domain.Message, 0, store.MaxMessagesPerChat+1)
	for i := 1; i <= store.MaxMessagesPerChat+1; i++ {
		msgs = append(msgs, domain.Message{ID: i, ChatID: 3, Date: time.Unix(int64(i), 0)})
	}
	s.SetMessages(3, msgs)
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 3})
	got := s2.Messages(domain.HistoryKey{ChatID: 3})

	require.Len(t, got, store.MaxMessagesPerChat)
	assert.Equal(t, 2, got[0].ID, "oldest (id 1) should be trimmed and absent on disk")
}

func TestSQLite_MessageEdit_PersistsSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	// Seed and persist the original message.
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 4, Peer: domain.Peer{ID: 4, Type: domain.PeerUser}})
	s.SetMessages(4, []domain.Message{{ID: 1, ChatID: 4, Text: "before", Date: time.Unix(10, 0)}})
	require.NoError(t, s.Close())

	// Reopen so the message is disk-loaded and clean (not already dirty), then
	// edit it — this only persists if the edit ops mark the message dirty.
	s2 := openStore(t, path)
	s2.SetChat(domain.Chat{ID: 4, Peer: domain.Peer{ID: 4, Type: domain.PeerUser}})
	s2.LoadMessages(domain.HistoryKey{ChatID: 4})
	s2.UpdateMessageText(4, 1, "after", nil)
	s2.MarkMessageEdited(4, 1, time.Unix(20, 0), true)
	s2.UpdateMessageReactions(4, 1, []domain.Reaction{{Emoji: "👍", Count: 2}}, "reactions update")
	require.NoError(t, s2.Close())

	s3 := openStore(t, path)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(domain.HistoryKey{ChatID: 4})
	got := s3.Messages(domain.HistoryKey{ChatID: 4})

	require.Len(t, got, 1)
	assert.Equal(t, "after", got[0].Text)
	require.Len(t, got[0].Reactions, 1)
	assert.Equal(t, "👍", got[0].Reactions[0].Emoji)
	require.NotNil(t, got[0].EditDate)
	assert.False(t, got[0].ShowsEdited(), "the hidden label survives a restart too")
}

func TestSQLite_RemoveMessage_DeletesOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	// Seed two messages and persist them.
	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 8, Peer: domain.Peer{ID: 8, Type: domain.PeerUser}})
	s.SetMessages(8, []domain.Message{
		{ID: 1, ChatID: 8, Text: "keep", Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 8, Text: "drop", Date: time.Unix(2, 0)},
	})
	require.NoError(t, s.Close())

	// Reopen so both are disk-loaded and clean, then remove one.
	s2 := openStore(t, path)
	s2.SetChat(domain.Chat{ID: 8, Peer: domain.Peer{ID: 8, Type: domain.PeerUser}})
	s2.LoadMessages(domain.HistoryKey{ChatID: 8})
	s2.RemoveMessage(8, 2)
	require.NoError(t, s2.Close())

	s3 := openStore(t, path)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(domain.HistoryKey{ChatID: 8})
	got := s3.Messages(domain.HistoryKey{ChatID: 8})

	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].ID)
}

// A message deleted from another device must be gone from disk too, or it comes
// back the next time the chat is opened.
func TestSQLite_RemoveMessagesByID_SurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})
	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "kept", Date: time.Unix(3000, 0)})
	s.AppendMessage(domain.Message{ID: 6, ChatID: 9, Text: "deleted", Date: time.Unix(3001, 0)})
	affected := s.RemoveMessagesByID([]int{6})
	require.Equal(t, []int64{9}, affected)
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 9})

	got := s2.Messages(domain.HistoryKey{ChatID: 9})
	require.Len(t, got, 1, "the deleted message must not come back from disk")
	assert.Equal(t, 5, got[0].ID)
}

// The delete update carries no chat context, so the chat is resolved from an
// index that only covers chats loaded this session. A chat that was never opened
// must still lose the message on disk, or it comes back on the next open.
func TestSQLite_RemoveMessagesByID_ChatNeverOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})
	s.SetMessages(9, []domain.Message{
		{ID: 5, ChatID: 9, Text: "kept", Date: time.Unix(3000, 0)},
		{ID: 6, ChatID: 9, Text: "deleted", Date: time.Unix(3001, 0)},
	})
	require.NoError(t, s.Close())

	// Reopen and delete without ever opening the chat: no LoadMessages, so the
	// chat holds nothing in memory and nothing in the index.
	s2 := openStore(t, path)
	assert.Equal(t, []int64{9}, s2.RemoveMessagesByID([]int{6}), "the owning chat must be resolved from disk")
	require.NoError(t, s2.Close())

	s3 := openStore(t, path)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(domain.HistoryKey{ChatID: 9})

	got := s3.Messages(domain.HistoryKey{ChatID: 9})
	require.Len(t, got, 1, "the deleted message must not come back from disk")
	assert.Equal(t, 5, got[0].ID)
}

// Opening the chat right after the delete, before the write-behind flush ran,
// must not read the deleted row back off disk.
func TestSQLite_RemoveMessagesByID_ThenOpenBeforeFlush(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})
	s.SetMessages(9, []domain.Message{
		{ID: 5, ChatID: 9, Text: "kept", Date: time.Unix(3000, 0)},
		{ID: 6, ChatID: 9, Text: "deleted", Date: time.Unix(3001, 0)},
	})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.RemoveMessagesByID([]int{6})
	s2.LoadMessages(domain.HistoryKey{ChatID: 9})

	got := s2.Messages(domain.HistoryKey{ChatID: 9})
	require.Len(t, got, 1, "the pending delete must win over the row still on disk")
	assert.Equal(t, 5, got[0].ID)
}

// A channel delete carries an explicit chat, but the chat may still be closed,
// with nothing of it in memory. The row must go from disk anyway.
func TestSQLite_RemoveMessages_ChannelNeverOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 50, Peer: domain.Peer{ID: 50, Type: domain.PeerChannel}})
	s.SetMessages(50, []domain.Message{
		{ID: 1, ChatID: 50, Text: "kept", Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 50, Text: "deleted", Date: time.Unix(2, 0)},
	})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	s2.RemoveMessages(50, []int{2}) // no LoadMessages: the channel was never opened
	require.NoError(t, s2.Close())

	s3 := openStore(t, path)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(domain.HistoryKey{ChatID: 50})

	got := s3.Messages(domain.HistoryKey{ChatID: 50})
	require.Len(t, got, 1, "the deleted message must not come back from disk")
	assert.Equal(t, 1, got[0].ID)
}

// Message IDs are only globally unique in the shared pts box (private chats and
// basic groups). A channel message that happens to carry the same number must
// not be swept up by a shared-box delete.
func TestSQLite_RemoveMessagesByID_LeavesChannelsAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 50, Peer: domain.Peer{ID: 50, Type: domain.PeerChannel}})
	s.SetMessages(50, []domain.Message{{ID: 6, ChatID: 50, Text: "channel", Date: time.Unix(3001, 0)}})
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	assert.Empty(t, s2.RemoveMessagesByID([]int{6}), "a channel message must not resolve as a shared-box delete")
	require.NoError(t, s2.Close())

	s3 := openStore(t, path)
	defer func() { _ = s3.Close() }()
	s3.LoadMessages(domain.HistoryKey{ChatID: 50})
	assert.Len(t, s3.Messages(domain.HistoryKey{ChatID: 50}), 1, "the channel message must survive")
}

// The same for a message this client sent: it was stored under an optimistic
// sentinel id and renumbered when the server confirmed it.
func TestSQLite_RemoveOfARenumberedMessage_SurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")

	s := openStore(t, path)
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})
	s.AppendMessage(domain.Message{ID: 42, ChatID: 9, Text: "mine", IsOut: true, Date: time.Unix(3000, 0)})
	require.Equal(t, []int64{9}, s.RemoveMessagesByID([]int{42}))
	require.NoError(t, s.Close())

	s2 := openStore(t, path)
	defer func() { _ = s2.Close() }()
	s2.LoadMessages(domain.HistoryKey{ChatID: 9})

	assert.Empty(t, s2.Messages(domain.HistoryKey{ChatID: 9}), "the deleted message must not come back from disk")
}

// The same message can arrive twice: once from the reply to the RPC that created
// it and once from a later getDifference. Appending must be idempotent by id, or
// the chat shows the message twice.
func TestSQLite_AppendMessage_SameIDTwiceDoesNotDuplicate(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer func() { _ = s.Close() }()
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})

	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "forwarded", Date: time.Unix(1, 0)})
	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "forwarded", Date: time.Unix(1, 0)})

	got := s.Messages(domain.HistoryKey{ChatID: 9})
	require.Len(t, got, 1, "the second copy must not add a row")
	assert.Equal(t, "forwarded", got[0].Text)
}

// A re-delivery may carry more than the first copy did (a sender name resolved,
// media refs filled in), so the newer version wins in place.
func TestSQLite_AppendMessage_SameIDAdoptsTheNewerCopy(t *testing.T) {
	s := openStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer func() { _ = s.Close() }()
	s.SetChat(domain.Chat{ID: 9, Peer: domain.Peer{ID: 9, Type: domain.PeerUser}})

	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "hi", Date: time.Unix(1, 0)})
	s.AppendMessage(domain.Message{ID: 5, ChatID: 9, Text: "hi", SenderName: "Ada", Date: time.Unix(1, 0)})

	got := s.Messages(domain.HistoryKey{ChatID: 9})
	require.Len(t, got, 1)
	assert.Equal(t, "Ada", got[0].SenderName)
}

// Renaming an optimistic sentinel onto the id the server assigned was how a sent
// message stopped being a guess. Nothing renames anything since #195: the owner
// records the message under its real id, and the duplicate this guarded against
// is now covered by AppendMessage being idempotent (see the test above).
