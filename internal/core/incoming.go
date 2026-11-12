package core

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// Event is something that happened, told to the clients attached when it
// happened: everything on an attachment's stream that is not a delta. The set
// is closed, so a connection can name every member of it (#209).
type Event interface{ isEvent() }

func (Incoming) isEvent()         {}
func (Failure) isEvent()          {}
func (Typing) isEvent()           {}
func (Progress) isEvent()         {}
func (DownloadProgress) isEvent() {}
func (Notification) isEvent()     {}
func (AccountEnded) isEvent()     {}
func (MessageGone) isEvent()      {}
func (HostFailure) isEvent()      {}
func (HostStopping) isEvent()     {}

// Incoming reports that something arrived in a chat the client is not showing:
// enough for the row to flash and follow the reorder (#39). It is an event, not
// state, which is why it does not ride on a projection — a chat-list row says
// what the unread count is now, never that something just happened.
//
// Whether it also deserves the user's attention is a Notification, decided
// separately. The gates differ: a muted chat still flashes.
type Incoming struct {
	ChatID int64
	// TopicID is the forum topic the message arrived in, 0 outside a forum.
	TopicID int
}

// Failure reports work the owner could not finish on a client's behalf. A client
// cannot see the attempt — filling a window may or may not touch the network —
// so a silent failure would leave a spinner running forever.
//
// Err carries the domain error kind from #191; the client decides how to say it.
// It is a domain error and nothing else, since it is a value that crosses to a
// client (#309).
type Failure struct {
	ChatID int64
	Op     string
	Err    *telerr.Error
}

// MessageGone tells the client whose window was anchored on a message that
// Telegram no longer has it: deleted, or out of reach. Nearest is the message
// of the history around it nearest to where it was, zero when Telegram
// returned none, for the client to open its window on instead (#347).
//
// It is an event rather than a window change because the window is the
// client's: the owner cannot move it, only say why it shows nothing.
type MessageGone struct {
	ChatID  int64
	TopicID int
	MsgID   int
	Nearest int
}

// OpSend names a send the queue gave up on. Clients single it out: a refused
// send is not a window that failed to fill, so it belongs in a toast rather
// than in the pane's load-error banner, and the message is still on screen to
// retry (#193).
const OpSend = "send"

// Typing reports that someone started or stopped composing in a chat. An empty
// Label means stopped.
//
// It is an event rather than part of the chat projection, because there is no
// state behind it to be right about: Telegram is not obliged to send a stop, so
// a stored label has nothing to clear it and would outlive the typing forever.
// The client shows it and lets it expire.
type Typing struct {
	ChatID int64
	// TopicID is the forum topic the typing is in, 0 outside a forum (#275).
	TopicID int
	Label   string
}

// broadcast tells every attached client that something happened. Whoever was
// not attached at that moment never hears it, which is what makes it an event
// rather than state. A client that stopped draining loses it too: nothing may
// stall the owner, and no later delta is stated against an event. A quiet event
// is dropped without a log line, for the ones frequent enough that a line per
// drop would bury the log.
func (o *Owner) broadcast(ev any, quiet bool) {
	for _, a := range o.attached() {
		if !a.offer(ev) && !quiet {
			a.dropped(ev)
		}
	}
}

// tell tells this client alone that something happened, for an event that is
// the course of its own request.
func (a *seat) tell(ev any) {
	if !a.offer(ev) {
		a.dropped(ev)
	}
}

func (a *seat) dropped(ev any) {
	a.log.Warn("event dropped: client is not draining", zap.String("event", fmt.Sprintf("%T", ev)))
}

func (o *Owner) publishTyping(t Typing) { o.broadcast(t, false) }

// Progress is how far a queued media send has uploaded.
//
// It is an event rather than part of the chat projection, for the reason typing
// is: there is nothing persisted behind it, a dropped frame is corrected by the
// next one, and the uploader emits one per chunk. Routed through the projection,
// that stream would rebuild and diff every subscription per chunk, and would
// compete for the delta buffer with changes whose loss costs a stale window.
type Progress struct {
	ChatID int64
	Ref    string
	// Part is the 1-based file being uploaded now, Parts the total in the entry.
	Part  int
	Parts int
	// Done and Total are bytes over the whole entry, so a client renders the
	// aggregate without having to know what each part weighs.
	Done  int64
	Total int64
}

// DownloadProgress is how far a save the client asked to watch has come. It is
// addressed to the call rather than to every client: Ref is whatever the caller
// passed to SaveMedia, and over a socket it is the call's own id. Like upload
// progress it is an absolute value, so a frame lost on the way is corrected by
// the next one (#204).
type DownloadProgress struct {
	Ref   string
	Done  int64
	Total int64
}

// publishProgress goes to every client: an entry being uploaded is in the send
// queue every client may be showing. It is quiet: under a large upload a log
// line per drop would be thousands of lines about a repaint nobody missed.
func (o *Owner) publishProgress(p Progress) { o.broadcast(p, true) }

// publishFailure tells every client about work the owner gave up on that
// belongs to all of them, such as a send the shared queue could not deliver. A
// failure of one client's own request is told to that client instead.
func (o *Owner) publishFailure(f Failure) { o.broadcast(f, false) }

// publishNotification hands the decision the owner already made to every
// client to draw as a toast; none of them re-judges it (#192). A client that
// stopped draining is not drawing the toast anyway.
func (o *Owner) publishNotification(n Notification) { o.broadcast(n, false) }

// publishIncoming reports an inbound message in a chat no client is showing.
// Outgoing messages and the focused chat are excluded: neither should flash a
// row. Mute is deliberately not consulted — that gates the interruption, not the
// reorder cue.
func (o *Owner) publishIncoming(evt store.Event, focused func(domain.HistoryKey) bool) {
	if evt.Kind != store.EventNewMessage || evt.Message.IsOut {
		return
	}
	chat, ok := o.state.Store().GetChat(evt.Message.ChatID)
	if !ok {
		return
	}
	h := chat.HistoryOf(evt.Message)
	if focused(h) {
		return
	}
	o.broadcast(Incoming{ChatID: h.ChatID, TopicID: h.TopicID}, false)
}
