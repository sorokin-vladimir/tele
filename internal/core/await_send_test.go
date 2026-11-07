package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// A send that becomes a message while it is waited on is answered with the
// message it became (#181).
func TestAwaitSend_AnswersWithTheMessageASendBecame(t *testing.T) {
	c := &stubClient{sentID: 77}
	o, _ := newCmdOwner(t, c)
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)
	require.NoError(t, o.Send(ctx, SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	got, err := o.AwaitSend(ctx, "r1", 2*time.Second)

	require.NoError(t, err)
	require.NotNil(t, got.Sent, "%+v", got)
	assert.Equal(t, []int{77}, got.Sent.MsgIDs)
	assert.Equal(t, int64(1), got.Sent.ChatID)
}

// A send that went before it was asked about is answered at once: the ref is
// remembered.
func TestAwaitSend_ASendThatWentIsAnsweredAtOnce(t *testing.T) {
	c := &stubClient{sentID: 77}
	o, _ := newCmdOwner(t, c)
	q := newOutboxStore(t)
	o.SetOutbox(q)
	ctx := runWorker(t, o)
	require.NoError(t, o.Send(ctx, SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))
	waitFor(t, "the send never went", func() bool { _, ok := q.SentRef("r1"); return ok })

	got, err := o.AwaitSend(ctx, "r1", 0)

	require.NoError(t, err)
	require.NotNil(t, got.Sent)
}

// A send that fails for good is answered with why: it will not go by itself.
func TestAwaitSend_AnswersWithAFailure(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden, Detail: "CHAT_WRITE_FORBIDDEN"}}
	o, _ := newCmdOwner(t, c)
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)
	require.NoError(t, o.Send(ctx, SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	got, err := o.AwaitSend(ctx, "r1", 2*time.Second)

	require.NoError(t, err)
	require.NotNil(t, got.Entry, "%+v", got)
	assert.Equal(t, domain.OutboxFailed, got.Entry.State)
	assert.Equal(t, telerr.Forbidden, got.Entry.ErrKind)
}

// A send still waiting when the time is up is answered with where it stands,
// not with an error: it is queued, and it will go.
func TestAwaitSend_WhatIsStillQueuedWhenTheTimeIsUp(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{sentID: 77})
	o.SetOutbox(newOutboxStore(t))
	// No worker: nothing sends it.
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))

	start := time.Now()
	got, err := o.AwaitSend(context.Background(), "r1", 100*time.Millisecond)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
	require.NotNil(t, got.Entry)
	assert.Equal(t, domain.OutboxQueued, got.Entry.State)
	assert.Nil(t, got.Sent)
}

// A ref neither queued nor sent names nothing: never submitted, or discarded.
func TestAwaitSend_AnUnknownRefIsNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	o.SetOutbox(newOutboxStore(t))

	_, err := o.AwaitSend(context.Background(), "nobody", time.Second)

	assert.Equal(t, telerr.NotFound, telerr.Of(err), "got %v", err)
}

// A send discarded while it is waited on is gone, and said to be.
func TestAwaitSend_ADiscardWhileWaitingIsNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})
	o.SetOutbox(newOutboxStore(t))
	require.NoError(t, o.Send(context.Background(), SendRequest{Ref: "r1", ChatID: 1, Text: "hi"}))
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = o.DiscardOutbox("r1")
	}()

	start := time.Now()
	_, err := o.AwaitSend(context.Background(), "r1", 2*time.Second)

	assert.Equal(t, telerr.NotFound, telerr.Of(err), "got %v", err)
	assert.Less(t, time.Since(start), time.Second, "the discard did not wake the wait")
}
