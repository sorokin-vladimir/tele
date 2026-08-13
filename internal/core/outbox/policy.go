// Package outbox is the durable send queue: the entries on disk, and the two
// decisions that drive them — which entry goes next, and how long a failed one
// waits. Both decisions are pure functions over values, deliberately knowing
// nothing about Telegram or about domain state, so every ordering and backoff
// rule is testable without a connection or a database (#193).
package outbox

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// maxBackoff bounds the wait between transient-failure retries. There is no
// bound on the number of retries: a week-long outage leaves the message queued
// and labelled, because nobody lost it and discarding it is the user's call
// rather than a timer's.
const maxBackoff = 5 * time.Minute

// Next returns the entry the worker should attempt, if any:
//
//	the oldest entry by Seq whose NextAttemptAt has arrived, and which has no
//	older unresolved entry in the same chat.
//
// The second clause is the whole ordering model. Only the head of each chat's
// queue is ever eligible, so FIFO within a chat holds with no extra bookkeeping,
// and a chat sitting in backoff delays nobody else.
//
// "Unresolved" is doing the work there. A failed entry is not a head: it will
// never be attempted again without a person acting on it, so a position in the
// order it can never occupy is not an order, it is a stop (#224). That holds for
// every failure and not only for a refusal — a send forbidden by the chat is as
// immovable as one Telegram would not accept. An entry waiting out a backoff is
// a head, because it is going to happen and its place in the conversation is
// still its own.
//
// The alternative was to keep the ordering absolute and render the sends behind
// a failed head as blocked rather than pending. It loses because it makes a
// person do the work to recover something the queue could simply not break, and
// because a failure nobody triages still leaves a chat that cannot be typed in.
//
// entries need not be sorted.
func Next(entries []domain.OutboxEntry, now time.Time) (domain.OutboxEntry, bool) {
	heads := chatHeads(entries)

	var best domain.OutboxEntry
	found := false
	for _, e := range entries {
		if !eligible(e, heads, now) {
			continue
		}
		if !found || e.Seq < best.Seq {
			best, found = e, true
		}
	}
	return best, found
}

// EarliestDue returns when Next could first return something, so a caller can
// sleep exactly that long instead of polling. ok is false when no queued head
// is waiting on a clock — either there is nothing to do, or something is due
// now and Next will hand it over.
func EarliestDue(entries []domain.OutboxEntry, now time.Time) (time.Time, bool) {
	heads := chatHeads(entries)

	var at time.Time
	found := false
	for _, e := range entries {
		if e.State != domain.OutboxQueued || !isHead(e, heads) {
			continue
		}
		if !e.NextAttemptAt.After(now) {
			return time.Time{}, false // due now
		}
		if !found || e.NextAttemptAt.Before(at) {
			at, found = e.NextAttemptAt, true
		}
	}
	return at, found
}

// chatHeads maps each chat to the lowest Seq it holds that is still going to
// happen. Only a head is ever eligible, which is what keeps a chat in order.
//
// Failed entries are skipped, so a chat whose oldest entry failed is headed by
// the next one instead. A chat holding nothing else has no entry in the map at
// all, which is why the caller must not read a missing key as Seq 0 — see
// eligible.
func chatHeads(entries []domain.OutboxEntry) map[int64]int64 {
	heads := make(map[int64]int64, len(entries))
	for _, e := range entries {
		if e.State == domain.OutboxFailed {
			continue
		}
		if seq, ok := heads[e.ChatID]; !ok || e.Seq < seq {
			heads[e.ChatID] = e.Seq
		}
	}
	return heads
}

func eligible(e domain.OutboxEntry, heads map[int64]int64, now time.Time) bool {
	return e.State == domain.OutboxQueued &&
		isHead(e, heads) &&
		!e.NextAttemptAt.After(now)
}

// isHead answers against the map rather than by indexing it: a chat with no
// head has no key, and a bare lookup would return 0 and match an entry whose Seq
// happened to be 0.
func isHead(e domain.OutboxEntry, heads map[int64]int64) bool {
	seq, ok := heads[e.ChatID]
	return ok && seq == e.Seq
}

// Backoff says how long an entry waits after err, and whether it is finished.
//
// Terminality follows the error kind and never an attempt count: a repeat
// cannot change a forbidden action or a missing peer, and a repeat is exactly
// what an outage needs. There is no "attempts exhausted" state.
func Backoff(err error, attempts int) (time.Duration, bool) {
	e, ok := telerr.As(err)
	if !ok {
		return 0, true // unmapped: treated as internal, which is terminal
	}
	switch e.Kind {
	case telerr.RateLimited:
		// Telegram named the interval; waiting anything else is guessing.
		return e.RetryAfter, false
	case telerr.Network:
		if !e.Transient {
			return 0, true
		}
		return growing(attempts), false
	case telerr.Unauthorized:
		// Not the message's failure. The worker is parked on readiness anyway;
		// this is the interval at which it re-checks once a session returns.
		return maxBackoff, false
	default:
		// peer_not_found, forbidden, not_found, rejected, internal,
		// stale_reference. All terminal: repeating the same request cannot
		// change a refusal, a missing peer or content Telegram would not take.
		return 0, true
	}
}

// CountsAsAttempt reports whether err should advance the backoff curve. A rate
// limit does not: it is an interval Telegram assigned, not a failure of ours,
// and counting it would make a long wait shorten the next one.
func CountsAsAttempt(err error) bool {
	return telerr.Of(err) != telerr.RateLimited
}

// growing doubles from two seconds and stops at maxBackoff.
func growing(attempts int) time.Duration {
	d := 2 * time.Second
	for i := 0; i < attempts; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return d
}
