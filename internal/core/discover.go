package core

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// discoverDelay is how long a fetch for unknown chats waits for company. A
// reconnect's catch-up can deliver first messages of many new chats at once,
// and one dialog list read answers them all.
const discoverDelay = 300 * time.Millisecond

// discoverBackoff is how long a chat the dialog list did not contain is left
// alone. Messages can arrive for chats that are in no dialog list (a channel
// seen through a discussion group), and a reload per message would hammer
// Telegram for nothing.
const discoverBackoff = time.Minute

// discovery is the unknown-chat fetch: what is waiting for it, and whether it
// is running.
type discovery struct {
	mu      sync.Mutex
	running bool
	waiting []store.Event
	// lost is when each chat the last reload did not resolve may be tried again.
	lost map[int64]time.Time
}

// discoverChat handles a message for a chat the store has no row for: a person
// who never wrote before, a group or channel the account was just added to, a
// first message sent from another device. Chats only enter the store through
// the dialog list, so without this the chat stays out of the list until the
// next start while its messages pile up.
//
// The fetch is the dialog list rather than the one peer. An update carries the
// peer's access hash only in entities the dispatcher does not keep, and the
// dialog list is the one place a chat's unread count, last message and archive
// flag are stated together. It runs off the update loop and coalesces bursts.
func (o *Owner) discoverChat(evt store.Event) {
	d := &o.discovery
	d.mu.Lock()
	if until, backedOff := d.lost[evt.Message.ChatID]; backedOff && o.now().Before(until) {
		d.mu.Unlock()
		return
	}
	d.waiting = append(d.waiting, evt)
	start := !d.running
	d.running = true
	d.mu.Unlock()
	if start {
		o.spawn(o.runDiscovery)
	}
}

// runDiscovery works through what is waiting until nothing is left. A message
// that arrives during a fetch may be for a chat the fetch did not see, so it
// waits for another round instead of being dropped.
func (o *Owner) runDiscovery() {
	d := &o.discovery
	for {
		select {
		case <-time.After(o.discoverDelay):
		case <-o.ctx.Done():
			return
		}
		d.mu.Lock()
		batch := d.waiting
		d.waiting = nil
		d.mu.Unlock()

		o.loadDialogsForNewChats(batch)

		d.mu.Lock()
		if len(d.waiting) == 0 {
			d.running = false
			d.mu.Unlock()
			return
		}
		d.mu.Unlock()
	}
}

// loadDialogsForNewChats reads the dialog lists and adds the chats the batch
// was waiting for, then announces each message whose chat is there now.
//
// Only chats still unknown are written (State.AddDiscovered): the dialog list's
// unread count is authoritative, and it already includes the message that
// started this, so nothing is counted twice, while a chat that was known all
// along keeps counters the update stream has moved since.
func (o *Owner) loadDialogsForNewChats(batch []store.Event) {
	st := o.state.Store()
	unresolved := func() bool {
		for _, e := range batch {
			if _, ok := st.GetChat(e.Message.ChatID); !ok {
				return true
			}
		}
		return false
	}

	chats, err := o.client.GetDialogs(o.ctx)
	if err != nil {
		o.log.Warn("could not load the dialog list for a new chat", zap.Error(err))
	} else {
		o.state.AddDiscovered(chats)
		// A first message can land in the Archive, where the main list does not
		// show it; the archived list is what states that flag.
		if unresolved() {
			archived, aerr := o.client.GetArchivedDialogs(o.ctx)
			if aerr != nil {
				o.log.Warn("could not load the archived list for a new chat", zap.Error(aerr))
			} else {
				o.state.AddDiscovered(archived)
			}
		}
	}

	d := &o.discovery
	retry := o.now().Add(discoverBackoff)
	for _, e := range batch {
		if _, ok := st.GetChat(e.Message.ChatID); ok {
			o.announce(e)
			continue
		}
		d.mu.Lock()
		if d.lost == nil {
			d.lost = make(map[int64]time.Time)
		}
		d.lost[e.Message.ChatID] = retry
		d.mu.Unlock()
	}
}
