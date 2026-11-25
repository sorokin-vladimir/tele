package core

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// StreamSize bounds what one client may fall behind by before it starts losing
// things. Deltas and events share it: one stream keeps them in the order they
// happened. The client's end of a connection holds as much, so a client falls
// behind by the same amount whichever side of a connection it is on.
const StreamSize = 512

// seat is an attachment's place at one owner: the subscriptions made through
// it, the focus it reports and the way to the client's stream. An attachment
// outlives owners and takes a new seat at each; a seat ends with its owner, or
// earlier when the client goes (#309).
type seat struct {
	*Owner
	id clientID

	// registry holds the client's subscriptions at this owner, under the ids
	// the client chose.
	registry *project.Registry
	// stream is the attachment's: what the client hears from every owner it
	// is attached to, in order.
	stream chan<- any
	// left is set once the seat ends. Nothing it would say after that reaches
	// the stream, which is what keeps a late delta of an owner that ended from
	// landing after the news that it did.
	mu   sync.Mutex
	left bool
	// resyncArmed is set while a rebuild is scheduled to repair a delta this
	// client lost, so a burst of drops schedules one rebuild rather than one
	// each (#276).
	resyncArmed atomic.Bool
}

// attach seats a client whose stream is stream.
func (o *Owner) attach(stream chan<- any) *seat {
	s := &seat{Owner: o, id: o.focus.attach(), stream: stream}
	s.registry = project.NewRegistry(o.reader(), s.emit)
	o.clientsMu.Lock()
	o.clients[s.id] = s
	o.clientsMu.Unlock()
	return s
}

// leave ends the seat: its subscriptions stop costing anything, the focus it
// held is dropped, a login it was driving is abandoned, and nothing more of
// this owner reaches the client.
func (s *seat) leave() {
	s.driverGone(s.id)
	s.mu.Lock()
	s.left = true
	s.mu.Unlock()
	s.clientsMu.Lock()
	delete(s.clients, s.id)
	s.clientsMu.Unlock()
	s.focus.detach(s.id)
	s.syncShown()
}

// hasLeft reports whether the seat has ended.
func (s *seat) hasLeft() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.left
}

// setFocus reports the history this client is showing - a chat, or one topic
// of a forum - and the zero key for none.
func (s *seat) setFocus(h domain.HistoryKey) { s.focus.set(s.id, h) }

// subscribe registers a window under the id the client chose.
func (s *seat) subscribe(id project.SubID, w project.Window) {
	// Bring the chat's persisted tail into memory before the window is built, so
	// it paints cached history at once instead of waiting on the network — and
	// still shows something when there is no network at all (#139).
	if cw, ok := w.(project.HistoryWindow); ok {
		s.state.Store().LoadMessages(cw.History())
	}
	s.registry.Subscribe(id, w)
	// Before the fetch: the page it brings is what the window asked for, and
	// the store must know that by the time it caps.
	s.syncShown()
	s.maybeFetch(s, id, w)
}

// moveWindow repositions a subscription. It returns immediately: over a socket
// a window move cannot be synchronous, so it is not synchronous here either.
func (s *seat) moveWindow(id project.SubID, w project.Window) {
	s.registry.MoveWindow(id, w)
	s.syncShown()
	s.maybeFetch(s, id, w)
}

// saveMedia saves with its progress addressed to this client alone: the course
// of a call is the caller's to watch (#204).
func (s *seat) saveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir, ref string) (string, error) {
	return s.media.Save(ctx, chatID, msgID, slot, destDir, ref, func(p DownloadProgress) { s.offer(p) })
}

// offer hands one thing to the client without waiting: a client that stopped
// draining must never stall the owner. It reports whether it was taken. A seat
// that ended swallows it: it is not the client's to hear, and not a drop to
// repair either.
func (s *seat) offer(v any) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.left {
		return true
	}
	select {
	case s.stream <- v:
		return true
	default:
		return false
	}
}

// resyncDelay is how long after a dropped delta the owner rebuilds on its own.
// Long enough for a stalled client to drain, short enough that the repair is
// not noticed.
const resyncDelay = 200 * time.Millisecond

// emit drops a delta rather than blocking when the client is not draining.
// Reporting the drop is what repairs it: the registry forgets what that
// subscription was told, so its next delta is a resync rather than one stated
// against a delta never received.
//
// That next delta must not wait for an unrelated change, so a drop also
// schedules one rebuild of this client's subscriptions. It runs on a timer of
// its own: emit is called under the registry's lock and must not call back
// into it.
func (s *seat) emit(d project.Delta) bool {
	if s.offer(d) {
		return true
	}
	s.log.Warn("projection delta dropped: client is not draining")
	if s.resyncArmed.CompareAndSwap(false, true) {
		time.AfterFunc(resyncDelay, func() {
			// Disarmed before the rebuild, so a drop during it schedules
			// another rather than being lost.
			s.resyncArmed.Store(false)
			s.registry.Refresh()
		})
	}
	return false
}
