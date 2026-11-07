package hosting

import (
	"sync"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// Clients is every attachment the host has, each seated at the owner of the
// moment. The host seats them all at each owner in turn, tells them all what
// it has to say, and hears any of them ask for the account to end (#208).
type Clients struct {
	mu  sync.Mutex
	all map[*core.Attachment]chan struct{}
	// kinds is what kind each client said it is in its hello; one that has
	// not said yet is neither a window nor a command.
	kinds map[*core.Attachment]wire.ClientKind
	owner *core.Owner
	// ends carries the requests that the account end, from whichever client
	// made one. One waiting is enough, as it is for a single attachment.
	ends chan core.EndReason
}

// NewClients is a host with no clients and no owner yet.
func NewClients() *Clients {
	return &Clients{
		all:   make(map[*core.Attachment]chan struct{}),
		kinds: make(map[*core.Attachment]wire.ClientKind),
		ends:  make(chan core.EndReason, 1),
	}
}

// Add takes att in, seating it at the owner of the moment if there is one,
// and returns what takes it out again.
func (c *Clients) Add(att *core.Attachment) (remove func()) {
	gone := make(chan struct{})
	c.mu.Lock()
	c.all[att] = gone
	if c.owner != nil {
		att.Join(c.owner)
	}
	c.mu.Unlock()
	go c.hearEnds(att, gone)

	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.all, att)
			delete(c.kinds, att)
			c.tellWatched()
			c.mu.Unlock()
			close(gone)
		})
	}
}

// hearEnds passes att's requests that the account end on, until att is taken
// out.
func (c *Clients) hearEnds(att *core.Attachment, gone <-chan struct{}) {
	for {
		select {
		case <-gone:
			return
		case reason := <-att.EndRequests():
			select {
			case c.ends <- reason:
			default:
			}
		}
	}
}

// Join seats every client at o, the owner now current, and every client added
// from now on.
func (c *Clients) Join(o *core.Owner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.owner = o
	for att := range c.all {
		att.Join(o)
	}
	c.tellWatched()
}

// tellWatched tells the owner of the moment whether a window is attached,
// which is when its login issues a QR code (#330). The host is the one to
// say: only it heard what each client is. Called with c.mu held.
func (c *Clients) tellWatched() {
	if c.owner != nil {
		c.owner.SetWatched(c.countedLocked(wire.KindWindow) > 0)
	}
}

// Leave unseats every client from the owner that ended, telling each why.
// Telling waits on each client to take it, so it is done outside the lock.
func (c *Clients) Leave(ended core.AccountEnded) {
	for _, att := range c.unseat() {
		att.Leave(ended)
	}
}

func (c *Clients) unseat() []*core.Attachment {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.owner = nil
	return c.list()
}

// Tell tells every client something from the host.
func (c *Clients) Tell(v any) {
	c.mu.Lock()
	all := c.list()
	c.mu.Unlock()
	for _, att := range all {
		att.Tell(v)
	}
}

// Count is how many clients there are.
func (c *Clients) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.all)
}

// introduce records that att said in its hello it is a client of kind.
func (c *Clients) introduce(att *core.Attachment, kind wire.ClientKind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.all[att]; ok {
		c.kinds[att] = kind
		c.tellWatched()
	}
}

// Windows is how many of the clients are windows: someone looking.
func (c *Clients) Windows() int { return c.counted(wire.KindWindow) }

// Commands is how many of the clients are commands.
func (c *Clients) Commands() int { return c.counted(wire.KindCommand) }

func (c *Clients) counted(kind wire.ClientKind) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.countedLocked(kind)
}

func (c *Clients) countedLocked(kind wire.ClientKind) int {
	n := 0
	for _, k := range c.kinds {
		if k == kind {
			n++
		}
	}
	return n
}

// EndRequests is where the host hears that a client asked for the account to
// end.
func (c *Clients) EndRequests() <-chan core.EndReason { return c.ends }

func (c *Clients) list() []*core.Attachment {
	all := make([]*core.Attachment, 0, len(c.all))
	for att := range c.all {
		all = append(all, att)
	}
	return all
}
