package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// SendRequest is one submission to the durable queue. Ref is the caller's
// idempotency key: it owns the key so that resubmitting after a lost
// acknowledgement cannot produce a second message.
type SendRequest struct {
	Ref          string
	ChatID       int64
	Text         string
	Entities     []domain.MessageEntity
	ReplyToMsgID int
}

// NewRef returns a fresh idempotency key. Callers generate one per composed
// message, not per attempt.
func NewRef() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic("core: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(buf[:])
}

// Send puts a message on the durable queue and returns once the entry is on
// disk. It does not wait for Telegram: what happens after this — ordering,
// backoff, surviving a restart — is the queue's business.
//
// The only synchronous failure is a chat the owner cannot address. Everything
// else is reported through the entry's own state, because a client that has
// been acknowledged must not have to stay alive to learn the outcome.
func (o *Owner) Send(ctx context.Context, req SendRequest) error {
	if o.outbox == nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.send", Detail: "no outbox configured"}
	}
	// Resolved here to fail fast, and again on every attempt: a peer can become
	// addressable later, but one that is unknown now is a caller's mistake.
	if _, err := o.peer(req.ChatID); err != nil {
		return err
	}
	entry := domain.OutboxEntry{
		Ref:       req.Ref,
		ChatID:    req.ChatID,
		RandomID:  outbox.RandomIDFor(req.Ref),
		Kind:      domain.OutboxText,
		State:     domain.OutboxQueued,
		Message:   &domain.OutboxMessage{Text: req.Text, Entities: req.Entities, ReplyToMsgID: req.ReplyToMsgID},
		CreatedAt: time.Now(),
	}
	added, isNew, err := o.outbox.Add(entry)
	if err != nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.send", Detail: err.Error(), Cause: err}
	}
	if !isNew {
		return nil
	}
	o.log.Debug("outbox: queued", zap.String("ref", added.Ref), zap.Int64("chat_id", added.ChatID))
	o.Refresh()
	o.wakeOutbox()
	return nil
}

// RetryOutbox puts a failed entry back in the queue. Attempts reset: the user
// asking again is a new decision, not a continuation of the backoff curve.
//
// It goes to the back of its chat's queue rather than to the place it held. A
// failed entry does not hold its chat, so the sends composed after it have
// already gone; putting it back where it was would claim an order that is no
// longer true of the conversation, and would restore the block (#224). Its ref
// and random_id are untouched, so this cannot produce a second message.
func (o *Owner) RetryOutbox(ref string) error {
	if o.outbox == nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.retry", Detail: "no outbox configured"}
	}
	if _, ok := o.outbox.Get(ref); !ok {
		return &telerr.Error{Kind: telerr.NotFound, Op: "outbox.retry"}
	}
	if _, err := o.outbox.Requeue(ref); err != nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.retry", Detail: err.Error(), Cause: err}
	}
	o.Refresh()
	o.wakeOutbox()
	return nil
}

// DiscardOutbox drops an entry. What was queued is gone; that is the point of
// the action, and it is the only way an entry leaves the queue unsent.
//
// An upload in flight is stopped rather than waited out: discarding a
// half-uploaded video should not hold the queue for the rest of the file. The
// worker finds the entry missing when it returns and ends quietly, so a
// discarded send never comes back as a failure (#195).
func (o *Owner) DiscardOutbox(ref string) error {
	if o.outbox == nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.discard", Detail: "no outbox configured"}
	}
	o.uploadMu.Lock()
	cancel, uploading := o.uploadCancels[ref]
	o.uploadMu.Unlock()
	if uploading {
		cancel()
	}
	if err := o.outbox.Delete(ref); err != nil {
		return &telerr.Error{Kind: telerr.Internal, Op: "outbox.discard", Detail: err.Error(), Cause: err}
	}
	o.Refresh()
	o.wakeOutbox()
	return nil
}

// wakeOutbox nudges the worker. A full buffer means a wake is already pending,
// and two wakes do the same work as one.
func (o *Owner) wakeOutbox() {
	select {
	case o.outboxWake <- struct{}{}:
	default:
	}
}
