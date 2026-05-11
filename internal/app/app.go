package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type App struct {
	cfg    *config.Config
	log    *zap.Logger
	st     store.Store
	client *internaltg.GotdClient
}

func New(cfg *config.Config, log *zap.Logger) *App {
	return &App{
		cfg:    cfg,
		log:    log,
		st:     store.NewMemory(),
		client: internaltg.NewGotdClient(),
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
	root := ui.NewRootModel(a.client, a.st)
	root.SetLoginModel(screens.NewLoginModel(authFlow))

	prog := tea.NewProgram(root, tea.WithAltScreen())

	// Bridge: auth requests + ready signal → bubbletea
	go func() {
		for {
			cmd := screens.WaitForAuthRequest(authFlow, readyCh)
			msg := cmd()
			prog.Send(msg)
			if _, done := msg.(screens.ConnectedMsg); done {
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
				prog.Send(evt)
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
