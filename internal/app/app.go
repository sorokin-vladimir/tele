package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gen2brain/beeep"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// Notifier sends OS desktop notifications.
type Notifier interface {
	Notify(title, body string) error
}

type beeepNotifier struct{}

func (b beeepNotifier) Notify(title, body string) error {
	return beeep.Notify(title, body, "")
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

// notifyFreshnessWindow bounds how old an incoming message may be and still
// raise a desktop notification. Catch-up/backlog messages recovered via
// getDifference after an idle period carry their original (old) send time, so
// anything older than this window is treated as catch-up and stays silent
// (#123). Live delivery latency is typically well under this.
const notifyFreshnessWindow = 10 * time.Second

// shouldNotify decides whether evt warrants a desktop notification. Pure and
// clock-injected so the freshness rule is unit-testable. now is the reference
// time the message age is measured against (time.Now() in production).
func shouldNotify(st store.Store, evt store.Event, currentChatID int64, now time.Time) bool {
	if evt.Kind != store.EventNewMessage {
		return false
	}
	if evt.Message.IsOut {
		return false
	}
	if evt.Message.ChatID == currentChatID {
		return false
	}
	chat, ok := st.GetChat(evt.Message.ChatID)
	if !ok {
		return false
	}
	// Suppress notifications for muted chats and anything in the Archive
	// folder (archived chats are treated as muted).
	if chat.IsMuted || chat.IsArchived {
		return false
	}
	// Suppress catch-up backlog: an old-dated message is a difference-recovery
	// message, not live traffic (#123).
	if now.Sub(evt.Message.Date) > notifyFreshnessWindow {
		return false
	}
	return true
}

func maybeNotify(notifier Notifier, st store.Store, evt store.Event, currentChatID int64) {
	if !shouldNotify(st, evt, currentChatID, time.Now()) {
		return
	}
	// shouldNotify guarantees the chat exists.
	chat, _ := st.GetChat(evt.Message.ChatID)
	_ = notifier.Notify(chat.Title, truncate(evt.Message.Text, 100))
}

type App struct {
	cfg           *config.Config
	log           *zap.Logger
	st            store.Store
	client        *internaltg.GotdClient
	verbose       bool
	notifier      Notifier
	currentChatID int64
}

func New(cfg *config.Config, log *zap.Logger, verbose bool, trace bool) (*App, error) {
	statePath := filepath.Join(filepath.Dir(cfg.Telegram.SessionFile), "state.db")
	sqliteStore, err := store.NewSQLite(statePath, log)
	if err != nil {
		return nil, fmt.Errorf("open state DB: %w", err)
	}
	stateStorage := internaltg.NewSQLiteStateStorage(sqliteStore.DB())
	return &App{
		cfg:      cfg,
		log:      log,
		st:       sqliteStore,
		client:   internaltg.NewGotdClient(log, stateStorage, trace),
		verbose:  verbose,
		notifier: newNotifier(log),
	}, nil
}

func (a *App) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if sc, ok := a.st.(interface{ Close() error }); ok {
		defer func() { _ = sc.Close() }()
	}

	tmpDir, err := os.MkdirTemp("", "tele-*")
	if err != nil {
		a.log.Warn("failed to create temp dir for viewer photos", zap.Error(err))
		tmpDir = ""
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	authFlow := internaltg.NewAuthFlow()
	readyCh := make(chan struct{})

	// Start gotd in background goroutine
	tgErr := make(chan error, 1)
	go func() {
		tgErr <- a.client.Connect(ctx, a.cfg, authFlow, readyCh, func(userID int64) {
			a.st.ClearForNewAccount(userID)
		})
	}()

	// Build bubbletea model
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), a.cfg.KeybindingOverrides())
	for _, w := range warns {
		a.log.Warn("keybindings: " + w)
	}
	root := ui.NewRootModel(a.client, a.st, a.cfg.UI.HistoryLimit, a.verbose)
	root = root.WithContext(ctx).WithConfig(a.cfg).WithKeyMap(km)
	root.SetLoginModel(screens.NewLoginModel(authFlow))
	root.SetOnChatOpen(func(id int64) {
		atomic.StoreInt64(&a.currentChatID, id)
	})
	root.SetTmpDir(tmpDir)

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
		// Connected: load initial chats
		go func() {
			chats, err := a.client.GetDialogs(ctx)
			if err != nil {
				a.log.Error("GetDialogs failed", zap.Error(err))
				return
			}
			a.log.Info("dialogs loaded", zap.Int("count", len(chats)))
			for _, c := range chats {
				a.st.SetChat(c)
			}
			archived, err := a.client.GetArchivedDialogs(ctx)
			if err != nil {
				a.log.Warn("GetArchivedDialogs failed", zap.Error(err))
			} else {
				a.log.Info("archived dialogs loaded", zap.Int("count", len(archived)))
				for _, c := range archived {
					a.st.SetChat(c)
				}
			}
			prog.Send(screens.TransitionToMainMsg{})
		}()

		// Send cached folder filters immediately, then refresh from network
		if cached := a.st.FolderFilters(); len(cached) > 0 {
			prog.Send(ui.FolderFiltersMsg{Filters: cached})
		}
		go func() {
			filters, err := a.client.GetDialogFilters(ctx)
			if err != nil {
				a.log.Warn("GetDialogFilters failed", zap.Error(err))
				return
			}
			if len(filters) == 0 {
				return
			}
			a.log.Info("folder filters loaded", zap.Int("count", len(filters)))
			a.st.SetFolderFilters(filters)
			prog.Send(ui.FolderFiltersMsg{Filters: filters})
		}()
	}()

	// Bridge: incoming updates → bubbletea
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-a.client.Updates():
				a.log.Debug("incoming update", zap.Int("kind", int(evt.Kind)))
				prog.Send(evt)
				maybeNotify(a.notifier, a.st, evt, atomic.LoadInt64(&a.currentChatID))
			}
		}
	}()

	_, err = prog.Run()
	cancel()

	// Wait for tg client goroutine
	tgClientErr := <-tgErr
	if tgClientErr != nil && err == nil {
		return fmt.Errorf("telegram: %w", tgClientErr)
	}
	return err
}
