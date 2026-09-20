package store_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

func TestSQLite_ClearForNewAccount_RemovesPersistedAccountState(t *testing.T) {
	s := newTestSQLite(t)
	require.NoError(t, s.ClearForNewAccount(100))

	s.SetChat(domain.Chat{ID: 7, Title: "old account"})
	s.SetMessages(7, []domain.Message{{ID: 9, ChatID: 7, Text: "private", Date: time.Unix(100, 0)}})
	s.SetFolderFilters([]domain.FolderFilter{{ID: 1, Title: "old filter"}})
	s.Flush()
	seedAccountTables(t, s.DB())
	_, err := s.DB().Exec(`INSERT INTO metadata(key, value) VALUES ('notice:kept', '1')`)
	require.NoError(t, err)

	require.NoError(t, s.ClearForNewAccount(200))

	for _, tc := range []struct {
		name  string
		query string
	}{
		{name: "chats", query: `SELECT COUNT(*) FROM chats`},
		{name: "messages", query: `SELECT COUNT(*) FROM messages`},
		{name: "folder filters", query: `SELECT COUNT(*) FROM folder_filters`},
		{name: "chat gaps", query: `SELECT COUNT(*) FROM chat_gap`},
		{name: "update state", query: `SELECT COUNT(*) FROM update_state`},
		{name: "channel pts", query: `SELECT COUNT(*) FROM channel_pts`},
		{name: "channel access hashes", query: `SELECT COUNT(*) FROM channel_access_hash`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var count int
			require.NoError(t, s.DB().QueryRow(tc.query).Scan(&count))
			assert.Zero(t, count)
		})
	}
	assert.Empty(t, s.Chats())
	assert.Empty(t, s.Messages(7))
	assert.Empty(t, s.FolderFilters())
	assertMetadataValue(t, s.DB(), "owner_id", "200")
	assertMetadataValue(t, s.DB(), "notice:kept", "1")
}

func TestSQLite_ClearForNewAccount_DropsPendingMessageWrites(t *testing.T) {
	s := newTestSQLite(t)
	require.NoError(t, s.ClearForNewAccount(100))

	s.SetMessages(7, []domain.Message{{ID: 9, ChatID: 7, Text: "private", Date: time.Unix(100, 0)}})
	require.NoError(t, s.ClearForNewAccount(200))
	s.Flush()

	var count int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count))
	assert.Zero(t, count, "a pending write from the old account must not resurrect after the wipe")

	_, err := s.DB().Exec(
		`INSERT INTO messages(chat_id, msg_id, date, data) VALUES (7, 10, 200, ?)`,
		`{"id":10,"chat_id":7,"text":"new account"}`,
	)
	require.NoError(t, err)
	s.LoadMessages(7)
	require.Len(t, s.Messages(7), 1, "the old account's loaded marker must not suppress the new account")
}

func TestSQLite_ClearForNewAccount_PreservesLegacyAndSameOwnerState(t *testing.T) {
	s := newTestSQLite(t)
	s.SetChat(domain.Chat{ID: 7, Title: "kept"})

	require.NoError(t, s.ClearForNewAccount(100))
	require.Len(t, s.Chats(), 1, "first owner recording preserves a pre-owner database")

	require.NoError(t, s.ClearForNewAccount(100))
	require.Len(t, s.Chats(), 1, "reconnecting the same owner is a strict no-op")
}

func TestSQLite_ClearForNewAccount_MigratesExistingOwner(t *testing.T) {
	s := newTestSQLite(t)
	_, err := s.DB().Exec(`INSERT INTO metadata(key, value) VALUES ('owner_id', '200')`)
	require.NoError(t, err)
	s.SetChat(domain.Chat{ID: 7, Title: "old account"})
	s.SetMessages(7, []domain.Message{{ID: 9, ChatID: 7, Text: "private", Date: time.Unix(100, 0)}})
	s.Flush()

	require.NoError(t, s.ClearForNewAccount(200))

	var count int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&count))
	assert.Zero(t, count, "the first upgraded launch must remove rows missed by the old reset")
	assert.Empty(t, s.Chats())
	assertMetadataValue(t, s.DB(), accountResetVersionKeyForTest, "1")
}

func TestSQLite_ClearForNewAccount_RollsBackBeforeChangingOwner(t *testing.T) {
	s := newTestSQLite(t)
	require.NoError(t, s.ClearForNewAccount(100))
	s.SetChat(domain.Chat{ID: 7, Title: "old account"})
	s.SetMessages(7, []domain.Message{{ID: 9, ChatID: 7, Text: "private", Date: time.Unix(100, 0)}})
	s.Flush()
	_, err := s.DB().Exec(`
		CREATE TRIGGER refuse_message_delete
		BEFORE DELETE ON messages
		BEGIN
			SELECT RAISE(ABORT, 'refuse delete');
		END;
	`)
	require.NoError(t, err)

	assert.Error(t, s.ClearForNewAccount(200))

	assertMetadataValue(t, s.DB(), "owner_id", "100")
	require.Len(t, s.Chats(), 1, "memory remains with the old owner when the disk wipe rolls back")
	var chats, messages int
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM chats`).Scan(&chats))
	require.NoError(t, s.DB().QueryRow(`SELECT COUNT(*) FROM messages`).Scan(&messages))
	assert.Equal(t, 1, chats)
	assert.Equal(t, 1, messages)
}

func seedAccountTables(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, query := range []string{
		`INSERT INTO chat_gap(chat_id, after_msg_id) VALUES (7, 9)`,
		`INSERT INTO update_state(user_id, pts, qts, date, seq) VALUES (100, 1, 2, 3, 4)`,
		`INSERT INTO channel_pts(user_id, channel_id, pts) VALUES (100, 7, 8)`,
		`INSERT INTO channel_access_hash(user_id, channel_id, access_hash) VALUES (100, 7, 9)`,
	} {
		_, err := db.Exec(query)
		require.NoError(t, err)
	}
}

func assertMetadataValue(t *testing.T, db *sql.DB, key, want string) {
	t.Helper()
	var got string
	require.NoError(t, db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&got))
	assert.Equal(t, want, got)
}

const accountResetVersionKeyForTest = "account_reset_version"
