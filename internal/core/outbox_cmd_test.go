package core

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func newOutboxStore(t *testing.T) *outbox.Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "q.db"))
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := outbox.NewStore(db)
	require.NoError(t, err)
	return s
}

func TestSend_QueuesTheMessage(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)

	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	entries := q.ForChat(1)
	require.Len(t, entries, 1)
	assert.Equal(t, domain.OutboxQueued, entries[0].State)
	require.NotNil(t, entries[0].Message)
	assert.Equal(t, "hi", entries[0].Message.Text)
	assert.Equal(t, outbox.RandomIDFor("r1"), entries[0].RandomID)
}

func TestSend_IsIdempotentPerRef(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)

	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	assert.Len(t, q.ForChat(1), 1)
}

func TestSend_RefusesAnUnknownChatAndPersistsNothing(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)

	err := o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 999, Text: "hi"})

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.Empty(t, q.All(), "a rejected submission must leave nothing behind")
}

func TestSend_WithoutAQueueIsAnInternalError(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	err := o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"})

	assert.Equal(t, telerr.Internal, telerr.Of(err))
}

func TestRetryOutbox_RequeuesAndClearsTheFailure(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))
	e, _ := q.Get("r1")
	e.State = domain.OutboxFailed
	e.ErrKind = telerr.Forbidden
	e.ErrDetail = "CHAT_WRITE_FORBIDDEN"
	e.Attempts = 3
	require.NoError(t, q.Update(e))

	require.NoError(t, o.RetryOutbox("r1"))

	got, ok := q.Get("r1")
	require.True(t, ok)
	assert.Equal(t, domain.OutboxQueued, got.State)
	assert.Zero(t, got.Attempts)
	assert.Empty(t, string(got.ErrKind))
	assert.True(t, got.NextAttemptAt.IsZero())
}

// A failed send does not hold its chat, so by the time a person retries it the
// sends composed after it are already in the conversation. It rejoins behind
// them rather than claiming a position that no longer exists (#224).
func TestRetryOutbox_RejoinsBehindTheSendsThatOvertookIt(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "the photo"}))
	rejected, _ := q.Get("r1")
	rejected.State = domain.OutboxFailed
	rejected.ErrKind, rejected.ErrReason = telerr.Rejected, telerr.ReasonPhotoType
	require.NoError(t, q.Update(rejected))
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r2", ChatID: 1, Text: "typed after"}))
	overtook, _ := q.Get("r2")

	require.NoError(t, o.RetryOutbox("r1"))

	got, ok := q.Get("r1")
	require.True(t, ok)
	assert.Greater(t, got.Seq, overtook.Seq)
	assert.Equal(t, rejected.RandomID, got.RandomID,
		"a retry must not turn one message into two")
	assert.Empty(t, got.ErrReason)
}

func TestRetryOutbox_AnUnknownRefIsNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	o.SetOutbox(newOutboxStore(t))

	assert.Equal(t, telerr.NotFound, telerr.Of(o.RetryOutbox("nope")))
}

func TestDiscardOutbox_RemovesTheEntry(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	q := newOutboxStore(t)
	o.SetOutbox(q)
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	require.NoError(t, o.DiscardOutbox("r1"))

	assert.Empty(t, q.All())
}

func TestNewRef_IsUnique(t *testing.T) {
	assert.NotEqual(t, NewRef(), NewRef())
}
