package core

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// sentGracePeriod bounds how long a successfully sent entry waits for its
// message to arrive through the update path before the worker removes it
// anyway. The send happened; a bubble stuck at "sending" would be a lie.
// A variable so tests can shorten it.
var sentGracePeriod = 5 * time.Second

// idleInterval is how long the worker sleeps when nothing is due and nothing is
// scheduled. It exists only so a missed wake cannot strand the queue forever.
const idleInterval = time.Minute

// RunOutbox drains the durable queue. One goroutine, one request in flight:
// Telegram's rate limits are account-global, so parallel sends across chats buy
// nothing and provoke exactly the FLOOD_WAIT they would be trying to outrun.
// Head-of-line blocking is avoided in the selection instead (outbox.Next).
func (o *Owner) RunOutbox(ctx context.Context) {
	if o.outbox == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		entry, ok := outbox.Next(o.outbox.All(), time.Now())
		if !ok {
			if !o.waitForOutboxWork(ctx) {
				return
			}
			continue
		}
		o.attempt(ctx, entry)
	}
}

// waitForOutboxWork sleeps until something could become due, or a submission
// wakes it. It returns false when the context ended.
func (o *Owner) waitForOutboxWork(ctx context.Context) bool {
	wait := idleInterval
	if at, ok := outbox.EarliestDue(o.outbox.All(), time.Now()); ok {
		if d := time.Until(at); d < wait {
			wait = d
		}
	}
	if wait < 0 {
		wait = 0
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-o.outboxWake:
		return true
	case <-timer.C:
		return true
	}
}

// attempt sends one entry and records what came back.
func (o *Owner) attempt(ctx context.Context, e domain.OutboxEntry) {
	if e.Kind == domain.OutboxMedia {
		o.attemptMedia(ctx, e)
		return
	}
	peer, err := o.peer(e.ChatID)
	if err != nil {
		o.recordFailure(e, err)
		return
	}

	e.State = domain.OutboxSending
	if err := o.outbox.Update(e); err != nil {
		o.log.Error("outbox: could not mark an entry in flight", zap.Error(err))
		return
	}
	o.Refresh()

	o.log.Debug("outbox: sending",
		zap.String("ref", e.Ref), zap.Int64("chat_id", e.ChatID), zap.Int("attempt", e.Attempts))

	sent, err := o.client.SendMessage(ctx, peer, e.Message.Text, e.Message.ReplyToMsgID, e.TopicID, o.addressMentions(e.Message.Entities), e.RandomID)
	if ctx.Err() != nil {
		// The owner is going away. The row stays in "sending" on purpose: the
		// next process resets it and resends with the same random_id.
		return
	}
	if err != nil {
		o.recordFailure(e, err)
		return
	}
	o.recordSent(e, sent)
}

// recordFailure applies the queue's policy to a refused send.
func (o *Owner) recordFailure(e domain.OutboxEntry, err error) {
	delay, terminal := outbox.Backoff(err, e.Attempts)
	if outbox.CountsAsAttempt(err) {
		e.Attempts++
	}
	if terminal {
		e.State = domain.OutboxFailed
		e.NextAttemptAt = time.Time{}
		if te, ok := telerr.As(err); ok {
			e.ErrKind, e.ErrReason, e.ErrDetail = te.Kind, te.Reason, te.Detail
		} else {
			e.ErrKind, e.ErrReason, e.ErrDetail = telerr.Internal, "", err.Error()
		}
		o.log.Warn("outbox: send failed for good",
			zap.String("ref", e.Ref), zap.String("kind", string(e.ErrKind)))
		// Said once, when it happens. The bubble keeps only a glyph; the reason
		// is repeated by the client when the cursor rests on the entry (#193).
		o.publishFailure(Failure{ChatID: e.ChatID, Op: OpSend, Err: telerr.From(err)})
	} else {
		e.State = domain.OutboxQueued
		e.NextAttemptAt = time.Now().Add(delay)
		e.ErrKind, e.ErrReason, e.ErrDetail = "", "", ""
		o.log.Debug("outbox: retrying later",
			zap.String("ref", e.Ref), zap.Duration("in", delay), zap.Int("attempts", e.Attempts))
	}
	if uerr := o.outbox.Update(e); uerr != nil {
		o.log.Error("outbox: could not record a failure", zap.Error(uerr))
	}
	o.outboxMoved()
	o.Refresh()
}

// recordSent puts the sent message into domain state, which is what makes the
// pending bubble become a real one.
//
// Telegram sends no echo for a message this account sent: a send into a user
// chat answers with an updateShortSentMessage that carries no body, so nothing
// arrives through the update stream to record. The message comes back from the
// send itself and is applied here.
//
// The entry is not deleted here either. Applying commits, the commit listener
// runs clearSentOutbox, and that is where the row goes — so the bubble and the
// message swap inside one delta instead of across two with a frame showing
// neither (#193).
func (o *Owner) recordSent(e domain.OutboxEntry, sent domain.Message) {
	if sent.ID == 0 {
		// The send succeeded but named no message. Nothing can be recorded and
		// nothing should be re-sent: drop the entry and let the next history
		// fetch surface the message.
		o.log.Warn("outbox: send confirmed no message id", zap.String("ref", e.Ref))
		o.dropSent(e)
		return
	}
	e.SentMsgIDs = []int{sent.ID}
	if err := o.outbox.Update(e); err != nil {
		o.log.Error("outbox: could not record a send", zap.Error(err))
	}
	o.log.Debug("outbox: sent", zap.String("ref", e.Ref), zap.Int("msg_id", sent.ID))

	// Commits, so the listener clears the entry before the projections rebuild.
	o.state.ApplyIncoming(sent)

	// The listener normally has the row already. This is the backstop for the
	// case where it did not: the send happened, and a bubble stuck at "sending"
	// would be the lie.
	o.spawn(func() { o.dropIfUndelivered(e.Ref) })
}

// dropIfUndelivered removes a sent entry the update path never claimed. Without
// it a message that went out but produced no update would sit at "sending"
// forever, which is a worse lie than dropping the entry: the send did happen.
func (o *Owner) dropIfUndelivered(ref string) {
	select {
	case <-time.After(sentGracePeriod):
	case <-o.ctx.Done():
		// The account is ending, and its send queue goes with it.
		return
	}
	cur, ok := o.outbox.Get(ref)
	if !ok || len(cur.SentMsgIDs) == 0 {
		return
	}
	o.log.Warn("outbox: no update for a sent message, dropping the entry",
		zap.String("ref", ref), zap.Ints("msg_ids", cur.SentMsgIDs))
	o.dropSent(cur)
	o.Refresh()
}

// dropSent takes a send that happened off the queue, remembering its ref and
// the messages it became, and lets the worker know. The wake is the point: a
// sent entry stays the head of its chat until it goes, so the next message in
// that chat is not eligible — and without a nudge the worker would sleep out
// its idle interval before noticing (#193). The ref is remembered so that the
// same one submitted again is not sent again (#181).
func (o *Owner) dropSent(e domain.OutboxEntry) {
	if err := o.outbox.Sent(e, time.Now()); err != nil {
		o.log.Error("outbox: could not drop an entry", zap.Error(err))
		return
	}
	o.outboxMoved()
	o.wakeOutbox()
}

// clearSentOutbox removes entries whose message has arrived. Called from the
// change listener, so the pending bubble and the real message swap inside one
// delta rather than across two.
func (o *Owner) clearSentOutbox(chatID int64) {
	if o.outbox == nil {
		return
	}
	for _, e := range o.outbox.ForChat(chatID) {
		if len(e.SentMsgIDs) == 0 {
			continue
		}
		// An album group goes when all of its parts have arrived: dropping it on
		// the first would leave the rest of the bubble unaccounted for.
		landed := true
		for _, id := range e.SentMsgIDs {
			if _, err := o.messageByID(chatID, id); err != nil {
				landed = false
				break
			}
		}
		if !landed {
			continue
		}
		o.dropSent(e)
	}
}
