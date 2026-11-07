package core

import (
	"context"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// SendOutcome is where a send stands: the message it became, or the entry
// still in the queue. Exactly one is set.
type SendOutcome struct {
	// Sent is what the send became, once it did.
	Sent *domain.SentSend
	// Entry is the send while it is queued: waiting, being sent, deferred
	// until a time, or failed and waiting on a person.
	Entry *domain.OutboxEntry
}

// AwaitSend waits up to within for the send ref to become a message or to
// fail, and answers with where it stands then. Time running out is no error:
// the send is queued and will go, and the answer says so (#181). A ref
// neither queued nor remembered as sent is NotFound: never submitted, or
// discarded.
//
// The wait is the owner's rather than the caller's context, so that a client
// over a connection gets the answer rather than only a deadline.
func (o *Owner) AwaitSend(ctx context.Context, ref string, within time.Duration) (SendOutcome, error) {
	if o.outbox == nil {
		return SendOutcome{}, &telerr.Error{Kind: telerr.Internal, Op: "outbox.await", Detail: "no outbox configured"}
	}
	deadline := time.NewTimer(within)
	defer deadline.Stop()
	for {
		// Taken before looking, so that a change between the look and the
		// wait is not missed.
		moved := o.nextOutboxMove()
		if sent, ok := o.outbox.SentRef(ref); ok {
			return SendOutcome{Sent: &sent}, nil
		}
		e, ok := o.outbox.Get(ref)
		if !ok {
			return SendOutcome{}, &telerr.Error{Kind: telerr.NotFound, Op: "outbox.await"}
		}
		if e.State == domain.OutboxFailed {
			return SendOutcome{Entry: &e}, nil
		}
		select {
		case <-moved:
		case <-deadline.C:
			return SendOutcome{Entry: &e}, nil
		case <-ctx.Done():
			return SendOutcome{}, ctx.Err()
		}
	}
}

// nextOutboxMove is closed at the next change to where a send stands.
func (o *Owner) nextOutboxMove() <-chan struct{} {
	o.outboxMoveMu.Lock()
	defer o.outboxMoveMu.Unlock()
	if o.outboxMove == nil {
		o.outboxMove = make(chan struct{})
	}
	return o.outboxMove
}

// outboxMoved wakes whoever waits on a send: one went, failed, was put off or
// was discarded.
func (o *Owner) outboxMoved() {
	o.outboxMoveMu.Lock()
	defer o.outboxMoveMu.Unlock()
	if o.outboxMove != nil {
		close(o.outboxMove)
		o.outboxMove = nil
	}
}
