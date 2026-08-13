package core

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/core/project"
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
	Connect(ctx context.Context, cfg *config.Config, af *internaltg.AuthFlow, readyCh chan<- struct{}, onAuth func(int64, string)) error
}

// Owner holds the Telegram connection and everything that may only exist once
// per account: the gotd client, the domain state as sole writer on the update
// path, the update loop and the notification decision. Clients attach to it; in
// this release the only client is the TUI in the same process.
type Owner struct {
	cfg      *config.Config
	log      *zap.Logger
	state    *state.State
	client   Connection
	notifier Notifier
	authFlow *internaltg.AuthFlow

	events   <-chan store.Event
	deltas   chan project.Delta
	incoming chan Incoming
	failures chan Failure
	typing   chan Typing
	progress chan Progress
	// notifications carries decisions the owner has already made, so a client
	// renders rather than judges (#192).
	notifications chan Notification
	registry      *project.Registry
	readyCh       chan struct{}
	onAuthFn      func(userID int64, username string)

	// historyLimit is how many messages one backfill fetches, from config.
	historyLimit int

	// ctx bounds the owner's background work (history backfill). It is stored
	// rather than passed because that work is started by a subscription, which
	// has no call context of its own and outlives it either way.
	ctx context.Context

	// fetching guards one in-flight history fetch per subscription: rapid
	// scroll-up would otherwise fire several identical fetches whose duplicate
	// chunks stack into a repeating date range (issue #120).
	fetchMu  sync.Mutex
	fetching map[project.SubID]bool

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
}

func New(cfg *config.Config, log *zap.Logger, st *state.State, client Connection, n Notifier) *Owner {
	o := &Owner{
		cfg:           cfg,
		log:           log,
		state:         st,
		client:        client,
		notifier:      n,
		authFlow:      internaltg.NewAuthFlow(),
		deltas:        make(chan project.Delta, 256),
		incoming:      make(chan Incoming, 32),
		failures:      make(chan Failure, 32),
		typing:        make(chan Typing, 32),
		progress:      make(chan Progress, 32),
		notifications: make(chan Notification, 32),
		readyCh:       make(chan struct{}),
		historyLimit:  cfg.UI.HistoryLimit,
		ctx:           context.Background(),
		fetching:      make(map[project.SubID]bool),
		focus:         newFocusRegistry(),
		outboxWake:    make(chan struct{}, 1),
		uploadCancels: make(map[string]context.CancelFunc),
	}
	// Built from the owner, not from the store alone: the projection reads the
	// send queue too, and the queue arrives later through SetOutbox (#193).
	o.registry = project.NewRegistry(projectionReader{Store: st.Store(), owner: o})
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

// SetContext bounds the owner's background work. Call before Start.
func (o *Owner) SetContext(ctx context.Context) { o.ctx = ctx }

// AuthFlow is the login conversation the client drives on the owner's behalf.
func (o *Owner) AuthFlow() *internaltg.AuthFlow { return o.authFlow }

// Ready is closed once the connection is up and authenticated.
func (o *Owner) Ready() <-chan struct{} { return o.readyCh }

// SetOnAuth registers a callback fired once the account is known, so a client
// can record the self identity. Set before Start.
func (o *Owner) SetOnAuth(fn func(userID int64, username string)) { o.onAuthFn = fn }

func (o *Owner) onAuth(userID int64, username string) {
	if o.onAuthFn != nil {
		o.onAuthFn(userID, username)
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

// SaveMedia streams the named media into destDir, bypassing the cache, and
// returns the path it actually wrote. The owner picks the name: it follows from
// the document's own name or its MIME type, which is domain knowledge rather
// than rendering.
func (o *Owner) SaveMedia(ctx context.Context, chatID int64, msgID int, slot domain.MediaSlot, destDir string) (string, error) {
	return o.media.Save(ctx, chatID, msgID, slot, destDir)
}

// InvalidateMedia drops a cached file a client could not decode, so the next
// fetch downloads it again rather than handing back the same broken entry.
func (o *Owner) InvalidateMedia(chatID int64, msgID int, slot domain.MediaSlot) {
	o.media.Invalidate(chatID, msgID, slot)
}
