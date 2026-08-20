package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/outbox"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type App struct {
	// cfgStore is the config file and the config the app is running on. The app
	// asks it rather than holding a Config of its own, so that a reload reaches
	// everything through one place.
	cfgStore *config.Store
	log      *zap.Logger
	st       store.Store
	owner    *core.Owner
	// sqlite is the same object as st, kept concretely because notice
	// seen-state needs the database handle and store.Store does not expose it.
	sqlite *store.SQLiteStore
	// tmpDir holds this run's scratch files: media saved for an external
	// player, GIFs staged for decoding, and the media cache when the user asked
	// for no persistent one. Removed on exit.
	tmpDir  string
	verbose bool
	// stateMoved reports that startup migration relocated the account state, so
	// the user can be told where it went.
	stateMoved bool
	// logPath is this run's log file, known only to the caller that opened it.
	logPath string
	// selfMu guards the account identity: it arrives on the Telegram goroutine
	// and is read on exit, after the TUI is gone.
	selfMu       sync.Mutex
	selfID       int64
	selfUsername string
}

// SetStateMoved records whether startup migration relocated the account state.
func (a *App) SetStateMoved(moved bool) { a.stateMoved = moved }

// SetLogPath records where this run's log is written, so the farewell banner
// can point at it.
func (a *App) SetLogPath(path string) { a.logPath = path }

func (a *App) setSelf(userID int64, username string) {
	a.selfMu.Lock()
	defer a.selfMu.Unlock()
	a.selfID, a.selfUsername = userID, username
}

func (a *App) self() (int64, string) {
	a.selfMu.Lock()
	defer a.selfMu.Unlock()
	return a.selfID, a.selfUsername
}

// pendingNotices lists the one-time startup messages this build can show.
// Conditional entries are omitted when they do not apply, so a user only ever
// sees what actually happened on their machine.
func (a *App) pendingNotices() []notices.Notice {
	const delay = 7 * time.Second
	out := []notices.Notice{
		{
			ID:    "single-instance-v1.10",
			Title: "Only one tele at a time",
			Delay: delay,
			Body: "Starting a second tele on the same account now fails with a message " +
				"instead of starting. Two instances shared one session and one database " +
				"with nothing arbitrating between them, quietly overwriting each other's " +
				"unread counts and sync state, which surfaced later as missed messages.",
		},
	}
	if a.stateMoved {
		out = append(out, notices.Notice{
			ID:    "state-dir-moved-v1.10",
			Title: "Your data moved",
			Delay: delay,
			Body: "The session and local database now live in " + a.cfg().StateDir +
				", instead of next to the config file. They were moved for you and " +
				"nothing was lost: you are still logged in. The old location is now empty " +
				"and can be ignored.",
		})
	}
	if a.cfg().SessionPinned {
		out = append(out, notices.Notice{
			ID:    "session-file-deprecated-v1.10",
			Title: "session_file is going away",
			Delay: delay,
			Body: "Your config sets telegram.session_file, so your session was left exactly " +
				"where it is. That setting is deprecated and will be removed in the next " +
				"release. Replace it with state_dir pointing at the directory that should " +
				"hold the session and the database.",
		})
	}
	return out
}

// cfg is the config the app is running on. Asked for rather than held, so that
// a reload is seen by whoever asks next.
func (a *App) cfg() *config.Config { return a.cfgStore.Current() }

// reloadConfig re-reads the config file and hands the result to everything that
// holds one, before returning it to the UI to apply to itself. This is the whole
// of "apply what is on disk": one function, called whether the change was made
// in the settings overlay or in an editor (ADR 0009).
func (a *App) reloadConfig() (*config.Config, error) {
	if err := a.cfgStore.Reload(); err != nil {
		return nil, err
	}
	cfg := a.cfg()
	a.owner.SetConfig(cfg)
	return cfg, nil
}

func New(cfgStore *config.Store, log *zap.Logger, verbose bool, trace bool) (*App, error) {
	cfg := cfgStore.Current()
	statePath := filepath.Join(cfg.StateDir, "state.db")
	sqliteStore, err := store.NewSQLite(statePath, log)
	if err != nil {
		return nil, fmt.Errorf("open state DB: %w", err)
	}
	stateStorage := internaltg.NewSQLiteStateStorage(sqliteStore.DB())
	client := internaltg.NewGotdClient(log, stateStorage, trace)
	owner := core.New(cfg, log, state.New(sqliteStore), client, newNotifier(log))

	// The send queue shares the account database: the file DB runs on a single
	// connection (#119), and a second one to the same file is how SQLITE_BUSY
	// came back last time.
	sendQueue, err := outbox.NewStore(sqliteStore.DB())
	if err != nil {
		return nil, fmt.Errorf("open outbox: %w", err)
	}
	owner.SetOutbox(sendQueue)

	// The temp directory is created here rather than in Run because the media
	// cache may live inside it, and the owner needs the cache before it starts.
	tmpDir, err := os.MkdirTemp("", "tele-*")
	if err != nil {
		log.Warn("failed to create temp dir for media", zap.Error(err))
		tmpDir = ""
	}
	removeLegacyMediaCache(log)
	if cache, cerr := openMediaCache(cfg, tmpDir, log); cerr != nil {
		log.Warn("media cache unavailable; media will not be cached", zap.Error(cerr))
	} else {
		owner.SetMediaCache(cache)
	}
	if cache, cerr := openAvatarCache(cfg, tmpDir, log); cerr != nil {
		log.Warn("avatar cache unavailable; avatars will not be shown", zap.Error(cerr))
	} else {
		owner.SetAvatarCache(cache)
	}

	a := &App{
		cfgStore: cfgStore,
		log:      log,
		st:       sqliteStore,
		sqlite:   sqliteStore,
		owner:    owner,
		tmpDir:   tmpDir,
		verbose:  verbose,
	}
	// Registered after the App exists: the account identity is needed both by
	// the message list (own messages) and by the farewell banner on exit.
	owner.SetOnAuth(func(userID int64, username string) {
		components.SetSelfIdentity(userID, username)
		a.setSelf(userID, username)
	})
	return a, nil
}

func (a *App) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if sc, ok := a.st.(interface{ Close() error }); ok {
		defer func() { _ = sc.Close() }()
	}

	defer os.RemoveAll(a.tmpDir) //nolint:errcheck

	authFlow := a.owner.AuthFlow()
	readyCh := a.owner.Ready()

	// The owner holds the connection; this process also happens to render it.
	tgErr := make(chan error, 1)
	a.owner.SetContext(ctx)
	go func() { tgErr <- a.owner.Start(ctx) }()
	go a.owner.RunUpdates(ctx)
	go a.owner.RunOutbox(ctx)

	// Build bubbletea model
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), a.cfg().KeybindingOverrides())
	for _, w := range warns {
		a.log.Warn("keybindings: " + w)
	}
	// The client attaches: the focus it reports and the detach that ends it
	// belong to it, everything else is the owner's (#192).
	att := a.owner.Attach()
	defer att.Detach()

	root := ui.NewRootModel(a.st, a.cfg().UI.HistoryLimit, a.verbose)
	root = root.WithContext(ctx).WithConfig(a.cfg()).WithKeyMap(km).WithOwner(att).WithLogger(a.log).
		WithConfigReload(a.reloadConfig).WithSettingsStore(a.cfgStore)
	root.SetLoginModel(screens.NewLoginModel(authFlow))
	root.SetTmpDir(a.tmpDir)

	// One-time startup notices (#197). Seen-state is written on dismissal, so
	// quitting before the countdown ends shows the notice again next time.
	noticeSeen := notices.NewSQLiteSeen(a.sqlite.DB())
	root = root.WithNotices(notices.Pending(a.pendingNotices(), noticeSeen), noticeSeen)

	prog := tea.NewProgram(root)

	// Bridge: auth requests + ready signal → bubbletea
	go func() {
		var authOK bool
		for {
			cmd := screens.WaitForAuthRequest(authFlow, readyCh)
			msg := cmd()
			prog.Send(msg)
			if req, isReq := msg.(screens.AuthRequestMsg); isReq {
				a.log.Debug("auth step requested", zap.Int("step", int(req.Step)))
			}
			if _, done := msg.(screens.ConnectedMsg); done {
				a.log.Info("connected, loading dialogs")
				authOK = true
				break
			}
			if errMsg, failed := msg.(screens.AuthErrorMsg); failed {
				a.log.Error("auth error", zap.String("reason", errMsg.Text))
				break
			}
		}
		if !authOK {
			return
		}
		// Connected: the owner loads the authoritative dialog list.
		go func() {
			if err := a.owner.Bootstrap(ctx); err != nil {
				a.log.Error("GetDialogs failed", zap.Error(err))
				return
			}
			prog.Send(screens.TransitionToMainMsg{})
		}()

		// Send cached folder filters immediately, then refresh from network
		if cached := a.st.FolderFilters(); len(cached) > 0 {
			prog.Send(ui.FolderFiltersMsg{Filters: cached})
		}
		go func() {
			filters, err := a.owner.LoadFolderFilters(ctx)
			if err != nil {
				a.log.Warn("GetDialogFilters failed", zap.Error(err))
				return
			}
			if len(filters) == 0 {
				return
			}
			prog.Send(ui.FolderFiltersMsg{Filters: filters})
		}()
	}()

	// Bridge: projection deltas → bubbletea
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case d := <-a.owner.Deltas():
				prog.Send(d)
			case in := <-a.owner.Incoming():
				prog.Send(in)
			case n := <-a.owner.Notifications():
				prog.Send(n)
			case f := <-a.owner.Failures():
				prog.Send(f)
			case tp := <-a.owner.Typing():
				prog.Send(tp)
			case pr := <-a.owner.Progress():
				prog.Send(pr)
			}
		}
	}()

	_, err := prog.Run()
	cancel()

	// Disable OS color-scheme reports (DEC mode 2031) enabled at startup, so the
	// terminal stops emitting report sequences to the shell after tele exits
	// (issue #148). The program has restored the normal screen by now, so write
	// the reset directly.
	_, _ = fmt.Fprint(os.Stdout, ansi.ResetModeLightDark)

	// Leave something in the scrollback: which build ran, as whom, and where to
	// look afterwards. Skipped when the run failed (the error is the message) or
	// when stdout is not a terminal, so piping tele stays clean.
	if err == nil && term.IsTerminal(os.Stdout.Fd()) {
		home, _ := os.UserHomeDir()
		_, _ = fmt.Fprint(os.Stdout, a.farewell(home))
	}

	// Wait for tg client goroutine
	tgClientErr := <-tgErr
	if tgClientErr != nil && err == nil {
		return fmt.Errorf("telegram: %w", tgClientErr)
	}
	return err
}
