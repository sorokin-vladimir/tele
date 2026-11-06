package core

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// Connection is the owner's view of the Telegram client: the whole command
// surface plus the connect call. tg.Client deliberately omits Connect because
// connecting is the owner's business and no caller's; this interface joins the
// two so the owner can be built over a test double.
type Connection interface {
	internaltg.Client
	Connect(ctx context.Context, af *internaltg.AuthFlow, readyCh chan<- struct{}, onAuth func(domain.User)) error
}

// Owner holds the Telegram connection and everything that may only exist once
// per account: the gotd client, the domain state as sole writer on the update
// path, the update loop and the notification decision. Clients attach to it; in
// this release the only client is the TUI in the same process.
type Owner struct {
	// cfg is the config the owner is running on, swapped whole when the file is
	// reloaded. A pointer rather than a copy of the values that matter: a value
	// copied at construction is a value that quietly ignores every reload, and
	// which settings are live should not depend on how each reader was written
	// (#239).
	cfg      atomic.Pointer[config.Config]
	log      *zap.Logger
	state    *state.State
	client   Connection
	notifier Notifier
	authFlow *internaltg.AuthFlow

	events <-chan store.Event
	// clients are the attached clients, each with its own subscriptions and
	// its own stream. Nothing the owner says is shared between them on the way
	// out (#309).
	clientsMu sync.Mutex
	clients   map[clientID]*seat
	// acct is where the account stands: the login step, the connection, the
	// clock (#309).
	acct accountState
	// health is how the connection is doing, for whoever asks the host.
	health health
	// now is the owner's clock.
	now      func() time.Time
	readyCh  chan struct{}
	onAuthFn func(userID int64, username string)
	// selfName is the logged-in user's username, once known: a log out names
	// whose account it ends (#297).
	selfName atomic.Pointer[string]

	// ctx bounds the owner's background work (history backfill). It is stored
	// rather than passed because that work is started by a subscription, which
	// has no call context of its own and outlives it either way. It is the
	// owner's own and ends with Stop: an owner lasts one account (#297).
	ctx    context.Context
	cancel context.CancelFunc
	// work counts the background work started through spawn, so Stop can wait
	// for all of it. stopped refuses new work once Stop has begun, which is
	// also what keeps an Add from racing the Wait.
	workMu  sync.Mutex
	work    sync.WaitGroup
	stopped bool

	// fetching guards one in-flight history fetch per subscription: rapid
	// scroll-up would otherwise fire several identical fetches whose duplicate
	// chunks stack into a repeating date range (issue #120). A subscription is
	// named by its client too: two clients choose their ids apart.
	fetchMu  sync.Mutex
	fetching map[subKey]bool
	// repairing guards one in-flight gap repair per chat. A repair belongs to
	// the chat rather than to a window: it is started by opening one, and also
	// by Telegram saying a channel fell behind while nobody was looking at it.
	repairing map[int64]bool

	// topics coalesces the forum topic reads owed to Telegram (#275).
	topicsMu sync.Mutex
	topics   topicRefresh

	// focus is what each attached client is showing. The notification policy's
	// only view of clients (#192).
	focus *focusRegistry

	// media downloads on behalf of clients and owns the disk cache. It is the
	// only holder of file references (#196).
	media *mediaFetcher

	// avatars downloads people's pictures and owns a cache of its own. Separate
	// from media because the two agree on nothing but their shape: an avatar is
	// small, long-lived, fetched over and over for the same handful of people,
	// and must not compete for a budget sized for scrolled-past video
	// thumbnails (#223, ADR 0007).
	avatars *avatarFetcher

	// outbox is the durable send queue. Sends are handed to it and drained by
	// one worker; nothing about a send lives in a client's memory (#193).
	outbox *outbox.Store
	// outboxWake tells the worker to look again without waiting for its timer.
	// Buffered and dropped when full: one pending wake is as good as ten.
	outboxWake chan struct{}

	// uploadCancels stops the bytes of an entry being uploaded right now, so a
	// discard does not have to wait out a large file (#195). Written by the
	// worker goroutine, read by whichever goroutine serves the discard.
	uploadMu      sync.Mutex
	uploadCancels map[string]context.CancelFunc
	// outboxMove is closed when a send goes, fails, is put off or is
	// discarded, for whoever waits on one (#181). Nil until somebody does.
	outboxMoveMu sync.Mutex
	outboxMove   chan struct{}
}

func New(cfg *config.Config, log *zap.Logger, st *state.State, client Connection, n Notifier) *Owner {
	o := &Owner{
		log:           log,
		state:         st,
		client:        client,
		notifier:      n,
		authFlow:      internaltg.NewAuthFlow(log),
		clients:       make(map[clientID]*seat),
		now:           time.Now,
		readyCh:       make(chan struct{}),
		fetching:      make(map[subKey]bool),
		repairing:     make(map[int64]bool),
		focus:         newFocusRegistry(),
		outboxWake:    make(chan struct{}, 1),
		uploadCancels: make(map[string]context.CancelFunc),
	}
	o.ctx, o.cancel = context.WithCancel(context.Background())
	o.cfg.Store(cfg)
	o.acct.c.HistoryPage = cfg.UI.HistoryLimit
	o.media = newMediaFetcher(client, st, log)
	o.avatars = newAvatarFetcher(client, log)
	if client != nil {
		o.events = client.Updates()
	}
	// Every committed mutation rebuilds the subscribed projections, wherever it
	// originated: the update loop, a history backfill, or a command.
	st.OnChange(o.publishChange)
	return o
}

// spawn runs fn as the owner's background work: bounded by the owner's context
// and waited for by Stop. Once Stop has begun nothing new is started, so a
// command arriving while the account ends cannot reach what is being closed.
func (o *Owner) spawn(fn func()) {
	o.workMu.Lock()
	defer o.workMu.Unlock()
	if o.stopped {
		return
	}
	o.work.Add(1)
	go func() {
		defer o.work.Done()
		fn()
	}()
}

// Stop ends the owner's background work and waits for it. The connection and
// the two loops are stopped by whoever runs them, through the context they
// were given; Stop is for what the owner started on its own. After it returns
// nothing of the owner touches the account's files (#297).
func (o *Owner) Stop() {
	o.workMu.Lock()
	o.stopped = true
	o.workMu.Unlock()
	o.cancel()
	o.work.Wait()
}

// Config is the config the owner is running on. Read it where it is used rather
// than copying a value out of it, so that a setting follows a reload without
// anybody having to remember to re-copy it.
func (o *Owner) Config() *config.Config { return o.cfg.Load() }

// SetConfig installs a reloaded config. One store at the root, the way a theme
// is installed: no reader can see half of a change.
func (o *Owner) SetConfig(cfg *config.Config) {
	o.cfg.Store(cfg)
	o.historyPageIs(cfg.UI.HistoryLimit)
}

// SetOnAuth registers a callback fired once the account is known, so a client
// can record the self identity. Set before Start.
func (o *Owner) SetOnAuth(fn func(userID int64, username string)) { o.onAuthFn = fn }

func (o *Owner) onAuth(self domain.User) {
	o.selfName.Store(&self.Username)
	o.loggedIn(self)
	if o.onAuthFn != nil {
		o.onAuthFn(self.ID, self.Username)
	}
}

// SetOutbox gives the owner its durable send queue. Call before Start.
//
// It is set after construction rather than passed to New because the queue
// needs the store's database handle, which the app opens on its own schedule.
func (o *Owner) SetOutbox(s *outbox.Store) { o.outbox = s }

// SetMediaCache gives the owner the directory it caches media in. Call before
// Start. The cache is account-scoped and process-owned: two processes evicting
// independently in one directory would fight (#196).
func (o *Owner) SetMediaCache(c *mediacache.Cache) { o.media.cache = c }

// SetAvatarCache gives the owner the directory it caches avatars in. Call
// before Start. It is a second cache with a second bound rather than a second
// directory sharing one: see ADR 0007.
func (o *Owner) SetAvatarCache(c *mediacache.Cache) { o.avatars.cache = c }

// FetchMedia downloads the named media into the owner's cache if it is not
// there already, and returns the path. The client decodes the file; the bytes
// never cross the owner boundary.
//
// The returned file may in principle be evicted before the client opens it. A
// client that cannot open it renders nothing and asks again on the next
// repaint; see mediacache.Cache.Path.
func (o *Owner) FetchMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot) (string, error) {
	return o.media.Fetch(ctx, chatID, msgID, slot)
}

// InvalidateMedia drops a cached file a client could not decode, so the next
// fetch downloads it again rather than handing back the same broken entry.
func (o *Owner) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	o.media.Invalidate(chatID, msgID, slot)
}
