package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/gen2brain/beeep"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
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

func maybeNotify(notifier Notifier, st store.Store, evt store.Event, currentChatID int64) {
	if evt.Kind != store.EventNewMessage {
		return
	}
	if evt.Message.ChatID == currentChatID {
		return
	}
	chat, ok := st.GetChat(evt.Message.ChatID)
	if !ok {
		return
	}
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

func New(cfg *config.Config, log *zap.Logger, verbose bool) *App {
	return &App{
		cfg:      cfg,
		log:      log,
		st:       store.NewMemory(),
		client:   internaltg.NewGotdClient(log),
		verbose:  verbose,
		notifier: beeepNotifier{},
	}
}

func (a *App) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	authFlow := internaltg.NewAuthFlow()
	readyCh := make(chan struct{})

	// Start gotd in background goroutine
	tgErr := make(chan error, 1)
	go func() {
		tgErr <- a.client.Connect(ctx, a.cfg, authFlow, readyCh)
	}()

	// Build bubbletea model
	root := ui.NewRootModel(a.client, a.st, a.cfg.UI.HistoryLimit, a.verbose)
	root.SetLoginModel(screens.NewLoginModel(authFlow))
	root.SetOnChatOpen(func(id int64) {
		atomic.StoreInt64(&a.currentChatID, id)
	})

	prog := tea.NewProgram(root)

	// Bridge: auth requests + ready signal → bubbletea
	go func() {
		for {
			cmd := screens.WaitForAuthRequest(authFlow, readyCh)
			msg := cmd()
			prog.Send(msg)
			if req, isReq := msg.(screens.AuthRequestMsg); isReq {
				a.log.Debug("auth step requested", zap.Int("step", int(req.Step)))
			}
			if _, done := msg.(screens.ConnectedMsg); done {
				a.log.Info("connected, loading dialogs")
				break
			}
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
			prog.Send(screens.TransitionToMainMsg{})
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

	_, err := prog.Run()
	cancel()

	// Wait for tg client goroutine
	select {
	case tgErr := <-tgErr:
		if tgErr != nil && err == nil {
			return fmt.Errorf("telegram: %w", tgErr)
		}
	}
	return err
}
