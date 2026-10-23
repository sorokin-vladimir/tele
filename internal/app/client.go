package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// Attachment is a client's link to its host: the owner's surface, the stream
// it is told everything on, what the host said of itself, and the calls about
// the host's settings.
type Attachment interface {
	core.Surface
	Stream() <-chan any
	Host() wire.Hello
	HostCalls
	ReloadHost(ctx context.Context) (wire.ReloadResult, error)
	// Lost is why the attachment ended, once its stream has.
	Lost() error
}

// ClientConfig is what a client is made from.
type ClientConfig struct {
	// Store is the config file the client reads its own settings from.
	Store *config.Store
	Log   *zap.Logger
	// Attachment is the client's link to its host, and Detach ends it.
	Attachment Attachment
	Detach     func()
	// OtherWindows counts the other clients of the host this process holds,
	// which quitting would close too. Nil when the host is another tele's.
	OtherWindows func() int
	// StateDir is where the machine's record of the notices shown is kept.
	StateDir string
	// Notices are the one-time notices this start may show.
	Notices       []notices.Notice
	ThemeWarnings []string
	LogPath       string
	Verbose       bool
}

// Client draws the account its host is running, one account after another,
// and is driven from the keyboard. It reaches the owner of the moment only
// through its attachment, whether the host is in this process or another
// (#208).
type Client struct {
	c ClientConfig
	// imageMode is how images are drawn, settled when the client starts: a
	// renderer swapped under images already sent to the terminal is a mess,
	// not a setting taking effect (#239).
	imageMode media.Mode

	// prog draws whichever account is running; keyMap and noticeSeen are the
	// person's and the machine's, so every account's model shares them.
	prog *tea.Program
	// send hands a message to the model: the program's, once it runs.
	send       func(tea.Msg)
	keyMap     keys.KeyMap
	noticeSeen notices.Seen
	// tmpDir holds the client's scratch files: what a model saved to show the
	// person. screens counts the models built, and screenDir is the directory
	// of the one on screen, which the next one's replaces.
	tmpDir    string
	screens   uint64
	screenDir string

	// What the client heard of the account, for after the program ends: who
	// it was logged in as, for the farewell, and what its connection ended
	// with, for the exit code. Both come over the stream, as everything the
	// client knows does.
	heardMu sync.Mutex
	user    project.LoggedInUser
	ended   *telerr.Error
}

// NewClient makes a client from c. Nothing is drawn until Run.
func NewClient(c ClientConfig) *Client {
	tmpDir, err := os.MkdirTemp("", "tele-client-*")
	if err != nil {
		c.Log.Warn("no scratch directory for the client", zap.Error(err))
		tmpDir = ""
	}
	mode := media.DetectMode(c.Store.Current().Photos.Mode, os.Getenv)
	// The mode decides whether a photo modal can hold real pixels at all, and it
	// is picked from the environment, so say which one won and on what evidence:
	// "why is my photo block art" then has an answer in the log instead of a guess.
	c.Log.Info("image mode",
		zap.String("mode", mode.String()),
		zap.String("photos.mode", c.Store.Current().Photos.Mode),
		zap.String("TERM", os.Getenv("TERM")),
		zap.String("TERM_PROGRAM", os.Getenv("TERM_PROGRAM")),
		zap.Bool("tmux", os.Getenv("TMUX") != ""),
		zap.Bool("kitty_window", os.Getenv("KITTY_WINDOW_ID") != ""),
	)
	return &Client{
		c:         c,
		imageMode: mode,
		tmpDir:    tmpDir,
	}
}

// Run draws until the person quits or the process is told to stop, and ends
// the client's attachment when it does.
func (c *Client) Run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	defer func() {
		if c.tmpDir != "" {
			_ = os.RemoveAll(c.tmpDir)
		}
	}()

	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), c.c.Store.Current().KeybindingOverrides())
	for _, w := range warns {
		c.c.Log.Warn("keybindings: " + w)
	}
	c.keyMap = km
	c.noticeSeen, _ = notices.NewFileSeen(filepath.Join(c.c.StateDir, NoticesFile))

	c.prog = tea.NewProgram(ui.NewShell(c.buildRoot(ctx, true), func() ui.RootModel {
		return c.buildRoot(ctx, false)
	}))
	c.send = c.prog.Send
	go c.bridge(ctx)

	final, err := c.prog.Run()
	cancel()
	c.c.Detach()

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
		_, _ = fmt.Fprint(os.Stdout, c.farewell(home))
	}
	// What was being typed when the host went is the person's, and nothing
	// else kept it: it is handed back where they can copy it (#208).
	if shell, ok := final.(ui.Shell); ok && shell.Unsent() != "" {
		_, _ = fmt.Fprintf(os.Stdout, "Not sent:\n%s\n", shell.Unsent())
	}
	if ended := c.exitErr(); ended != nil && err == nil {
		return fmt.Errorf("telegram: %w", ended)
	}
	return err
}

// buildRoot builds a model for whichever account the client is attached to.
// The person's things - config, key map, log path - are shared; what the
// model saves to show the person goes in a scratch directory of its own, which
// the next model's replaces. The one-time startup notices belong to the start
// of the process, so only the first model has them.
func (c *Client) buildRoot(ctx context.Context, first bool) ui.RootModel {
	cfg := c.c.Store.Current()
	root := ui.NewRootModel(c.c.Verbose)
	// Every window reaches its host's settings over its attachment, the
	// hosting one included: one way, used every day (#315).
	root = root.WithContext(ctx).WithImageMode(c.imageMode).WithConfig(cfg).WithKeyMap(c.keyMap).WithOwner(c.c.Attachment).
		WithLogger(c.c.Log).WithConfigReload(c.reloadOwn).WithHostReload(hostReload(c.c.Attachment)).
		WithSettingsStore(NewWindowSettings(c.c.Store, c.c.Attachment)).WithLogPath(c.c.LogPath).
		WithOtherWindows(c.c.OtherWindows)
	root.SetLoginModel(screens.NewLoginModel())
	root.SetTmpDir(c.nextScreenDir())
	if first {
		// Seen-state is written on dismissal, so quitting before the countdown
		// ends shows the notice again next time (#197).
		root = root.WithNotices(notices.Pending(c.c.Notices, c.noticeSeen), c.noticeSeen)
		// The themes loaded at startup may have been reloaded since, and a
		// reload says what is wrong with them itself.
		root = root.WithThemeWarnings(c.c.ThemeWarnings)
		// What the host found wrong with its own settings when it started,
		// said in its hello.
		root = root.WithHostWarnings(fromWire(c.c.Attachment.Host().Warnings))
	}
	return root
}

// reloadOwn re-reads this window's own config file.
func (c *Client) reloadOwn() (*config.Config, error) {
	if err := c.c.Store.Reload(); err != nil {
		return nil, err
	}
	return c.c.Store.Current(), nil
}

// hostReload is how a window asks its host to re-read the host's settings: the
// warnings it answers with, or why it would not take its file.
func hostReload(att interface {
	ReloadHost(context.Context) (wire.ReloadResult, error)
}) func() ([]config.Warning, error) {
	return func() ([]config.Warning, error) {
		ctx, cancel := context.WithTimeout(context.Background(), hostCallTimeout)
		defer cancel()
		answer, err := att.ReloadHost(ctx)
		if err != nil {
			return nil, errors.New("the tele hosting this account did not answer")
		}
		if answer.Failed != "" {
			return nil, errors.New(answer.Failed)
		}
		return fromWire(answer.Warnings), nil
	}
}

// fromWire is what the host said was wrong with its settings, as warnings
// about the host's.
func fromWire(ws []wire.Warning) []config.Warning {
	out := make([]config.Warning, 0, len(ws))
	for _, w := range ws {
		out = append(out, config.Warning{Text: w.Text, ID: w.ID, Process: settings.Host})
	}
	return out
}

// nextScreenDir makes the scratch directory of the next model and removes the
// last one's: documents opened and photos saved for an account that ended are
// that account's, and must not outlive it on disk.
func (c *Client) nextScreenDir() string {
	if c.tmpDir == "" {
		return ""
	}
	if c.screenDir != "" {
		_ = os.RemoveAll(c.screenDir)
	}
	c.screens++
	dir := filepath.Join(c.tmpDir, "screen-"+strconv.FormatUint(c.screens, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		c.c.Log.Warn("no scratch directory for the screen", zap.Error(err))
		dir = ""
	}
	c.screenDir = dir
	return dir
}

// bridge carries the stream to the model for the life of the client: deltas
// and events from whichever owner the client is seated at, and what the host
// tells it. The stream is one and ordered, so whatever arrives after the news
// that an account ended belongs to the next one.
//
// When the stream ends, the host is gone, and the model is told whether it
// quit - it said it was stopping, last - or the link broke (#208).
func (c *Client) bridge(ctx context.Context) {
	stopping, uninstalled := false, false
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.c.Attachment.Stream():
			if !ok {
				if c.c.OtherWindows != nil && !stopping {
					// The host is this process's own: its link breaking is a
					// fault here, not a host that went away.
					c.c.Log.Error("the link to this process's own host broke")
				}
				lost := hostLost(stopping, c.c.Attachment.Lost())
				lost.Uninstalled = uninstalled
				c.send(lost)
				return
			}
			switch m := msg.(type) {
			case core.HostStopping:
				stopping = true
				uninstalled = m.Reason == core.StopUninstalled
				continue
			case dial.Reconnecting:
				c.send(ui.HostAwayMsg{})
				continue
			case dial.Reconnected:
				// A service that stopped and came back has not quit: its
				// stopping was the restart's (#210).
				stopping = false
				c.send(ui.HostBackMsg{})
				continue
			}
			c.hear(msg)
			c.send(msg)
		}
	}
}

// hostLost is what the window is told when its link to its host is gone for
// good: whether the host said it was stopping, and for a service the window
// waited for, whether it gave up on it or the service came back newer.
func hostLost(stopping bool, lost error) ui.HostLostMsg {
	var mismatch *dial.Mismatch
	switch {
	case errors.Is(lost, dial.ErrGaveUp):
		return ui.HostLostMsg{GaveUp: true}
	case errors.As(lost, &mismatch) && mismatch.Verdict == wire.RestartClient:
		return ui.HostLostMsg{UpdatedTo: mismatch.Host.Binary}
	default:
		return ui.HostLostMsg{Quit: stopping}
	}
}

// hear keeps what the client needs from msg once the program has ended: who
// the account is logged in as, and what its connection ended with. The end of
// an account forgets both, since they were that account's.
func (c *Client) hear(msg any) {
	c.heardMu.Lock()
	defer c.heardMu.Unlock()
	switch m := msg.(type) {
	case project.Delta:
		if m.Account != nil {
			c.user = m.Account.Contents.LoggedInUser
			c.ended = m.Account.Contents.Ended
		}
	case core.AccountEnded:
		c.user, c.ended = project.LoggedInUser{}, nil
	}
}

// exitErr is what the account's connection ended with, nil while it ran or
// when the account ended with a log out.
func (c *Client) exitErr() error {
	c.heardMu.Lock()
	defer c.heardMu.Unlock()
	if c.ended == nil {
		return nil
	}
	return c.ended
}
