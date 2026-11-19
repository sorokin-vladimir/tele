package core

import (
	"sync"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// clientID identifies one attached client for the lifetime of its attachment.
type clientID uint64

// focusRegistry records which chat each attached client is looking at. It is the
// notification policy's only view of clients: nothing else about a client
// affects a decision.
//
// It is a set rather than a single value for two reasons. The owner may have
// several clients attached (#183), and a client that goes away has to take its
// focus with it — otherwise a crashed client silences its last-open chat
// permanently.
type focusRegistry struct {
	mu   sync.Mutex
	next clientID
	// on is the history each client shows: a chat, or one topic of a forum.
	// The zero key means the client has nothing open.
	on map[clientID]domain.HistoryKey
}

func newFocusRegistry() *focusRegistry {
	return &focusRegistry{on: make(map[clientID]domain.HistoryKey)}
}

// attach registers a client with no chat open and returns its id.
func (r *focusRegistry) attach() clientID {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	r.on[r.next] = domain.HistoryKey{}
	return r.next
}

// set records the history a client is showing; the zero key means it has none
// open. A set from a client that has already detached is dropped: its focus is
// gone and must not come back.
func (r *focusRegistry) set(id clientID, h domain.HistoryKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.on[id]; !ok {
		return
	}
	r.on[id] = h
}

// showing reports that some attached client shows exactly this history. It is
// what silences a notification: in a forum, only the topic on screen is being
// looked at (#275).
func (r *focusRegistry) showing(h domain.HistoryKey) bool {
	if h.ChatID == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, on := range r.on {
		if on == h || (on.ChatID == h.ChatID && on.TopicID == domain.AllTopicsID) {
			return true
		}
	}
	return false
}

// detach forgets a client and the focus it held.
func (r *focusRegistry) detach(id clientID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.on, id)
}

// focused reports that some attached client is looking at chatID, through any
// history of it: a forum is on screen while one of its topics is. Chat 0 is
// never focused: it is how a client says it has no chat open, not a chat.
func (r *focusRegistry) focused(chatID int64) bool {
	if chatID == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, on := range r.on {
		if on.ChatID == chatID {
			return true
		}
	}
	return false
}
