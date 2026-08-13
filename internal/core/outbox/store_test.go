package outbox

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func openDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func textEntry(ref string, chatID int64, body string) domain.OutboxEntry {
	return domain.OutboxEntry{
		Ref:       ref,
		ChatID:    chatID,
		RandomID:  RandomIDFor(ref),
		Kind:      domain.OutboxText,
		State:     domain.OutboxQueued,
		Message:   &domain.OutboxMessage{Text: body},
		CreatedAt: time.Unix(1700000000, 0),
	}
}

func TestAdd_AssignsAMonotonicSeq(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)

	first, added, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)
	require.True(t, added)
	second, _, err := s.Add(textEntry("b", 10, "two"))
	require.NoError(t, err)

	assert.Greater(t, second.Seq, first.Seq)
}

func TestAdd_IsIdempotentPerRef(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)

	_, first, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)
	_, second, err := s.Add(textEntry("a", 10, "one again"))
	require.NoError(t, err)

	assert.True(t, first)
	assert.False(t, second, "a known ref must not create a second entry")
	assert.Len(t, s.All(), 1)
}

func TestSeqIsNeverReused(t *testing.T) {
	// AUTOINCREMENT, not a plain rowid: entries are deleted on success, and a
	// reused id would silently reorder submissions.
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)

	first, _, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)
	require.NoError(t, s.Delete("a"))
	second, _, err := s.Add(textEntry("b", 10, "two"))
	require.NoError(t, err)

	assert.Greater(t, second.Seq, first.Seq)
}

func TestReopen_KeepsEntriesAndResetsSending(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")

	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	e, _, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)
	e.State = domain.OutboxSending
	require.NoError(t, s.Update(e))

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)

	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.Equal(t, domain.OutboxQueued, got.State, "a send in flight when the process died is queued again")
	assert.Equal(t, e.RandomID, got.RandomID, "the random_id must survive: it is what makes the retry safe")
	require.NotNil(t, got.Message)
	assert.Equal(t, "one", got.Message.Text)
	assert.True(t, got.NextAttemptAt.IsZero())
}

func TestForChat_ReturnsOnlyItsOwnInSubmissionOrder(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)
	_, _, err = s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)
	_, _, err = s.Add(textEntry("b", 20, "other chat"))
	require.NoError(t, err)
	_, _, err = s.Add(textEntry("c", 10, "two"))
	require.NoError(t, err)

	got := s.ForChat(10)

	require.Len(t, got, 2)
	assert.Equal(t, "one", got[0].Message.Text)
	assert.Equal(t, "two", got[1].Message.Text)
}

func TestUpdate_PersistsFailureDetail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	e, _, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)

	e.State = domain.OutboxFailed
	e.ErrKind = "forbidden"
	e.ErrDetail = "CHAT_WRITE_FORBIDDEN"
	e.Attempts = 2
	require.NoError(t, s.Update(e))

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.Equal(t, domain.OutboxFailed, got.State)
	assert.Equal(t, "CHAT_WRITE_FORBIDDEN", got.ErrDetail)
	assert.Equal(t, 2, got.Attempts)
}

func TestUpdate_PersistsTheRetryTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	e, _, err := s.Add(textEntry("a", 10, "one"))
	require.NoError(t, err)

	due := time.Unix(1700000123, 0)
	e.NextAttemptAt = due
	require.NoError(t, s.Update(e))

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.True(t, got.NextAttemptAt.Equal(due))
}

func mediaEntry(ref string, chatID int64, parts []domain.OutboxMediaPart) domain.OutboxEntry {
	return domain.OutboxEntry{
		Ref:       ref,
		ChatID:    chatID,
		RandomID:  RandomIDFor(ref),
		Kind:      domain.OutboxMedia,
		State:     domain.OutboxQueued,
		Media:     &domain.OutboxMediaSend{Parts: parts},
		CreatedAt: time.Unix(1700000000, 0),
	}
}

func TestAdd_RoundTripsAMediaEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)

	e := mediaEntry("a", 10, []domain.OutboxMediaPart{
		{Path: "/tmp/a.jpg", Name: "a.jpg", Size: 12, SendAs: domain.MediaPhoto},
		{Path: "/tmp/b.mp4", Name: "b.mp4", Size: 34, SendAs: domain.MediaVideo},
	})
	e.Media.Caption = "hi"
	e.Media.ReplyToMsgID = 9
	_, _, err = s.Add(e)
	require.NoError(t, err)

	// A second store over the same file is what a restart looks like.
	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.Equal(t, domain.OutboxMedia, got.Kind)
	require.NotNil(t, got.Media)
	require.Len(t, got.Media.Parts, 2)
	assert.Equal(t, "/tmp/b.mp4", got.Media.Parts[1].Path)
	assert.Equal(t, domain.MediaVideo, got.Media.Parts[1].SendAs)
	assert.Equal(t, "hi", got.Media.Caption)
	assert.Equal(t, 9, got.Media.ReplyToMsgID)
	assert.Nil(t, got.Message, "a media entry carries no text payload")
}

func TestNewStore_ResetsAnUploadingEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	e := mediaEntry("a", 10, []domain.OutboxMediaPart{{Path: "/tmp/a.jpg", Name: "a.jpg"}})
	e.State = domain.OutboxUploading
	_, _, err = s.Add(e)
	require.NoError(t, err)

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.Equal(t, domain.OutboxQueued, got.State,
		"bytes that were going up when the process died must be sent again")
}

func TestRequeue_PutsTheEntryBehindWhatOvertookIt(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)
	failed, _, err := s.Add(textEntry("a", 10, "the photo"))
	require.NoError(t, err)
	failed.State = domain.OutboxFailed
	require.NoError(t, s.Update(failed))
	overtook, _, err := s.Add(textEntry("b", 10, "typed after"))
	require.NoError(t, err)

	got, err := s.Requeue("a")

	require.NoError(t, err)
	assert.Greater(t, got.Seq, overtook.Seq, "it goes behind the sends already in the conversation")
	assert.Equal(t, domain.OutboxQueued, got.State)
}

func TestRequeue_KeepsTheIdentityTelegramDeduplicatesOn(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)
	before, _, err := s.Add(textEntry("a", 10, "the photo"))
	require.NoError(t, err)

	got, err := s.Requeue("a")

	require.NoError(t, err)
	assert.Equal(t, before.Ref, got.Ref)
	assert.Equal(t, before.RandomID, got.RandomID,
		"a requeued send must not become a second message")
	assert.Equal(t, "the photo", got.Message.Text)
	assert.Equal(t, before.CreatedAt.UTC(), got.CreatedAt.UTC(),
		"it was composed when it was composed")
}

func TestRequeue_ClearsTheFailureItIsBeingRetriedFrom(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)
	e, _, err := s.Add(textEntry("a", 10, "the photo"))
	require.NoError(t, err)
	e.State = domain.OutboxFailed
	e.Attempts = 3
	e.ErrKind, e.ErrReason, e.ErrDetail = telerr.Rejected, telerr.ReasonPhotoType, "PHOTO_EXT_INVALID"
	require.NoError(t, s.Update(e))

	got, err := s.Requeue("a")

	require.NoError(t, err)
	assert.Zero(t, got.Attempts, "asking again is a new decision, not a continued backoff")
	assert.Empty(t, got.ErrKind)
	assert.Empty(t, got.ErrReason)
	assert.Empty(t, got.ErrDetail)
}

func TestRequeue_SurvivesAReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	_, _, err = s.Add(textEntry("a", 10, "the photo"))
	require.NoError(t, err)
	requeued, err := s.Requeue("a")
	require.NoError(t, err)

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)

	got, ok := reopened.Get("a")
	require.True(t, ok, "the row must exist exactly once, under the same ref")
	assert.Equal(t, requeued.Seq, got.Seq)
	assert.Len(t, reopened.All(), 1)
}

func TestRequeue_AnUnknownRefIsNotAnEntry(t *testing.T) {
	s, err := NewStore(openDB(t, filepath.Join(t.TempDir(), "q.db")))
	require.NoError(t, err)

	_, err = s.Requeue("nobody")

	assert.Error(t, err)
}

// The refusal has to survive a restart, or a rejected send comes back
// describing itself in the vaguest terms available.
func TestUpdate_PersistsTheReasonBehindARefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	s, err := NewStore(openDB(t, path))
	require.NoError(t, err)
	e, _, err := s.Add(textEntry("a", 10, "the photo"))
	require.NoError(t, err)
	e.State = domain.OutboxFailed
	e.ErrKind, e.ErrReason, e.ErrDetail = telerr.Rejected, telerr.ReasonPhotoType, "PHOTO_EXT_INVALID"
	require.NoError(t, s.Update(e))

	reopened, err := NewStore(openDB(t, path))
	require.NoError(t, err)

	got, ok := reopened.Get("a")
	require.True(t, ok)
	assert.Equal(t, telerr.Rejected, got.ErrKind)
	assert.Equal(t, telerr.ReasonPhotoType, got.ErrReason)
	assert.Equal(t, "PHOTO_EXT_INVALID", got.ErrDetail)
}

// A database written before err_reason existed must open and keep its rows.
// This is the case the reporter of #224 is in.
func TestNewStore_OpensADatabaseWrittenBeforeTheReasonColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	db := openDB(t, path)
	_, err := db.Exec(`
CREATE TABLE outbox (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	ref             TEXT    NOT NULL UNIQUE,
	chat_id         INTEGER NOT NULL,
	random_id       INTEGER NOT NULL,
	kind            TEXT    NOT NULL,
	payload         TEXT    NOT NULL,
	state           TEXT    NOT NULL,
	attempts        INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER NOT NULL DEFAULT 0,
	created_at      INTEGER NOT NULL,
	err_kind        TEXT    NOT NULL DEFAULT '',
	err_detail      TEXT    NOT NULL DEFAULT ''
);
INSERT INTO outbox (ref, chat_id, random_id, kind, payload, state, created_at, err_kind, err_detail)
VALUES ('stuck', 10, 42, 'text', '{"Text":"the photo"}', 'failed', 1700000000000, 'internal', 'PHOTO_EXT_INVALID');
`)
	require.NoError(t, err)

	s, err := NewStore(db)

	require.NoError(t, err)
	got, ok := s.Get("stuck")
	require.True(t, ok)
	assert.Empty(t, got.ErrReason, "an older build recorded no reason, and none is invented")
	assert.Equal(t, "PHOTO_EXT_INVALID", got.ErrDetail)
}

func TestNewStore_MigratesOnlyOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.db")
	_, err := NewStore(openDB(t, path))
	require.NoError(t, err)

	_, err = NewStore(openDB(t, path))

	assert.NoError(t, err, "reopening must not trip over a column that is already there")
}

func TestRandomIDFor_IsDeterministicAndNonZero(t *testing.T) {
	assert.Equal(t, RandomIDFor("abc"), RandomIDFor("abc"))
	assert.NotEqual(t, RandomIDFor("abc"), RandomIDFor("abd"))
	assert.NotZero(t, RandomIDFor("abc"))
}
