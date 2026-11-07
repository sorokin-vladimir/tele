package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gotd/td/telegram/updates"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// account is everything tele holds for one account while it runs: the
// database, the connection, the owner over them, the send queue and the
// caches. It is built whole and stopped whole. The host outlives it and holds
// nothing of it, which is what lets a log out end one account and a login start
// the next in the same process (#297).
type account struct {
	store *store.SQLiteStore
	owner *core.Owner
	log   *zap.Logger
	// startup is what the process settled when it started, which the account
	// was opened from and is drawn and removed by (#239).
	startup startup
	// tmpDir holds the account's scratch files, and its caches when nothing is
	// to be kept between runs. Empty when there is none.
	tmpDir string

	// ctx is the account's lifetime: it ends with stop, or with the host.
	ctx    context.Context
	cancel context.CancelFunc
	// loops counts what run started: the connection, the two owner loops and
	// the work the host runs alongside them.
	loops    sync.WaitGroup
	stopOnce sync.Once
}

// accountDeps is what the host hands an account it opens: its own things, which
// last longer than any account, and how to reach Telegram.
type accountDeps struct {
	// cfg is the config the account starts on, for its live settings; startup
	// is what the process settled when it started, which the account is opened
	// from (#239).
	cfg      *config.Config
	startup  startup
	log      *zap.Logger
	tmpDir   string
	notifier core.Notifier
	// connect builds the connection with the endpoint over the account's update
	// state storage.
	connect func(internaltg.Endpoint, updates.StateStorage) core.Connection
	// onAuth is told who logged in, once the account knows.
	onAuth func(userID int64, username string)
}

// openAccount builds an account over the state directory: the database, the
// connection, the owner, the send queue and the caches. Nothing runs yet.
func openAccount(d accountDeps) (*account, error) {
	endpoint, err := d.startup.endpoint()
	if err != nil {
		return nil, err
	}
	statePath := filepath.Join(d.startup.stateDir, "state.db")
	sqliteStore, err := store.NewSQLite(statePath, d.log)
	if err != nil {
		return nil, fmt.Errorf("open state DB: %w", err)
	}
	conn := d.connect(endpoint, internaltg.NewSQLiteStateStorage(sqliteStore.DB()))
	owner := core.New(d.cfg, d.log, state.New(sqliteStore), conn, d.notifier)
	// The client finds a skewed clock in what gotd logs; the owner hands it on
	// to whoever is drawing (#277).
	if skewed, ok := conn.(interface{ SetOnClockSkew(func(time.Duration)) }); ok {
		skewed.SetOnClockSkew(owner.SetClockSkew)
	}
	// Whether the link is up is the host's to say to whoever asks it (#208).
	if linked, ok := conn.(interface{ SetOnLink(func(bool)) }); ok {
		linked.SetOnLink(owner.SetLink)
	}
	// A flood wait somebody runs into shows in every client, not as a freeze
	// (#212).
	if throttled, ok := conn.(interface {
		SetOnThrottle(func(internaltg.Throttled))
	}); ok {
		throttled.SetOnThrottle(func(t internaltg.Throttled) { owner.SetThrottle(t.Method, t.Until) })
	}

	// The send queue shares the account database: the file DB runs on a single
	// connection (#119), and a second one to the same file is how SQLITE_BUSY
	// came back last time.
	sendQueue, err := outbox.NewStore(sqliteStore.DB())
	if err != nil {
		_ = sqliteStore.Close()
		return nil, fmt.Errorf("open outbox: %w", err)
	}
	owner.SetOutbox(sendQueue)

	if cache, cerr := openMediaCache(d.startup, d.tmpDir, d.log); cerr != nil {
		d.log.Warn("media cache unavailable; media will not be cached", zap.Error(cerr))
	} else {
		owner.SetMediaCache(cache)
	}
	if cache, cerr := openAvatarCache(d.startup, d.tmpDir, d.log); cerr != nil {
		d.log.Warn("avatar cache unavailable; avatars will not be shown", zap.Error(cerr))
	} else {
		owner.SetAvatarCache(cache)
	}
	if d.onAuth != nil {
		owner.SetOnAuth(d.onAuth)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &account{store: sqliteStore, owner: owner, log: d.log, startup: d.startup, tmpDir: d.tmpDir,
		ctx: ctx, cancel: cancel}, nil
}

// run connects and starts the owner's two loops, and alongside them the work
// the host runs for this account, all under the account's context, which ends
// with stop or with parent. onEnd is told what the connection ended with; it
// is called once, from the connection's goroutine, and must not wait for stop.
func (a *account) run(parent context.Context, onEnd func(error), work ...func(context.Context)) {
	ctx := a.ctx
	stopWithHost := context.AfterFunc(parent, a.cancel)
	go func() {
		<-ctx.Done()
		stopWithHost()
	}()
	a.loops.Add(3 + len(work))
	go func() {
		defer a.loops.Done()
		err := a.owner.Start(ctx)
		if onEnd != nil {
			onEnd(err)
		}
	}()
	go func() {
		defer a.loops.Done()
		a.owner.RunUpdates(ctx)
	}()
	go func() {
		defer a.loops.Done()
		a.owner.RunOutbox(ctx)
	}()
	for _, w := range work {
		go func() {
			defer a.loops.Done()
			w(ctx)
		}()
	}
}

// stop ends the account and waits for it: what run started, then the owner's
// own background work, then the database. Once it returns nothing of the
// account is running and its files may be removed. Safe to call more than
// once, and on an account that never ran.
func (a *account) stop() {
	a.stopOnce.Do(func() {
		a.cancel()
		a.loops.Wait()
		a.owner.Stop()
		if err := a.store.Close(); err != nil {
			a.log.Warn("closing the account database", zap.Error(err))
		}
	})
}
