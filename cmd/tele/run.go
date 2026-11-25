package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/accountstate"
	"github.com/sorokin-vladimir/tele/internal/app"
	"github.com/sorokin-vladimir/tele/internal/appkey"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/statedir"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// helloTimeout bounds how long a client waits for its host to answer hello.
const helloTimeout = 5 * time.Second

// attachTimeout bounds how long a window waits to attach to another tele's
// host, which takes as long as a service restarting on a newer version does
// (#183).
const attachTimeout = time.Minute

// stopGrace is how long a stopping host gives its other clients to hear it.
const stopGrace = 2 * time.Second

// run is what a start has settled by the time it knows whether it hosts.
type run struct {
	cfgStore      *config.Store
	cfgPath       string
	key           appkey.Key
	log           *zap.Logger
	logPath       string
	verbose       bool
	trace         bool
	themeWarnings []string
	reached       hosting.Reached
}

// hello is what this process says of itself on the endpoint, as a client and
// as a host alike.
func (r run) hello() wire.Hello {
	return wire.Hello{Protocol: wire.ProtocolVersion, Binary: version.Version, Config: r.cfgPath}
}

// helloAs is what this process says of itself as a client of kind (#213).
func (r run) helloAs(kind wire.ClientKind) wire.Hello {
	h := r.hello()
	h.Kind = kind
	return h
}

// host runs this process as the host, with its own TUI attached to it through
// the endpoint like any other client (#208).
func (r run) host() error {
	h, err := r.startHost(wire.KeptByWindow)
	if err != nil {
		return err
	}
	defer h.stop(core.StopQuit)

	att, err := r.attach(r.reached.Endpoint)
	if err != nil {
		return err
	}
	cfg := r.cfgStore.Current()
	return r.runClient(att, app.ClientConfig{
		// This window is one of the host's clients; the rest close with it.
		// A command attached is not one: quitting does not close anything
		// anybody looks at for it (#213).
		OtherWindows: func() int { return max(h.clients.Windows()-1, 0) },
		// Only a tele that hosts offers its service: one that only attaches
		// is a window of a host that may be the service already (#208).
		Notices: app.StartupNotices(cfg.StateDir, h.stateMoved, cfg.SessionPinned,
			systemInstaller(io.Discard, nil).offer(options{cfgPath: r.cfgPath})),
	})
}

// daemon runs this process as the host with no window of its own, kept running
// by kept, until ctx ends or, as a service, until a newer client asks it to
// restart (#208, #183).
func (r run) daemon(ctx context.Context, kept wire.Keeper) error {
	h, err := r.startHost(kept)
	if err != nil {
		return err
	}
	defer h.stop(core.StopQuit)
	r.log.Info("hosting with no window", zap.String("state_dir", r.cfgStore.Current().StateDir))
	select {
	case <-ctx.Done():
		// Told to quit, by a person or by the service manager; for good, when
		// the service was told it is being uninstalled first (#182).
		h.stop(h.stopReason())
		return nil
	case <-h.restart:
		r.log.Info("restarting, as a newer tele asked")
		h.stop(core.StopRestart)
		return &exitCode{code: restartCode, msg: "tele restarts: a newer tele asked it to"}
	}
}

// restartCode is what a service exits with to be started again: a failure, so
// that a service manager that restarts only a failed process restarts it too.
// EX_TEMPFAIL, the closest of the conventional codes.
const restartCode = 75

// hosted is this process being the host: its clients, and how it stops.
type hosted struct {
	clients *hosting.Clients
	// stateMoved says the state was migrated to the state directory on the
	// way in, which the first window says once.
	stateMoved bool
	// restart is closed when a newer client asked this host, a service, to
	// restart.
	restart chan struct{}
	// stop stops the host for reason. Only the first call stops it.
	stop func(reason core.StopReason)
	// stopReason is why the host stops when told to quit.
	stopReason func() core.StopReason
}

// startHost makes this process the host of the endpoint it reached, kept
// running by kept. It holds the state directory until stop. Clients are served
// before anything else is done, so a tele starting beside this one attaches
// rather than waits; they are seated at the first owner when it is open.
func (r run) startHost(kept wire.Keeper) (*hosted, error) {
	cfg := r.cfgStore.Current()
	clients := hosting.NewClients()
	hello := r.hello()
	hello.Kept = kept
	hello.PID = os.Getpid()
	end := app.NewHostEnd(r.cfgStore, hello)
	h := &hosted{clients: clients, restart: make(chan struct{}), stopReason: end.StopReason}
	// Before the endpoint is served: a newer client may ask the moment it
	// attaches, and the restart waits for the start to finish.
	var restartOnce sync.Once
	end.RestartWith(func() { restartOnce.Do(func() { close(h.restart) }) })
	srv := hosting.Serve(context.Background(), r.reached.Listener, end, clients)
	ctx, cancel := context.WithCancel(context.Background())
	var host *app.Host
	// In this order: the endpoint goes, so nobody new attaches and the others
	// hear the host is stopping; then the account stops; the lock goes last.
	var stopOnce sync.Once
	h.stop = func(reason core.StopReason) {
		stopOnce.Do(func() {
			cancel()
			srv.Stop(stopGrace, reason)
			if host != nil {
				host.Stop()
			}
			r.reached.Close()
		})
	}

	// A pinned session_file is left exactly where the user put it, so nothing
	// is migrated in that case.
	if !cfg.SessionPinned {
		moved, err := statedir.Migrate(cfg.StateDir, filepath.Dir(r.cfgPath), r.log)
		if err != nil {
			h.stop(core.StopQuit)
			return nil, fmt.Errorf("state migration: %w", err)
		}
		h.stateMoved = moved
	}
	cleared, reason, err := accountstate.Reconcile(cfg.StateDir, cfg.Telegram.SessionFile)
	if err != nil {
		h.stop(core.StopQuit)
		return nil, fmt.Errorf("account state: %w", err)
	}
	if cleared {
		r.log.Info("cleared local state before startup", zap.String("reason", string(reason)))
	}

	host, err = app.NewHost(r.cfgStore, r.key, r.log, r.trace, clients)
	if err != nil {
		h.stop(core.StopQuit)
		return nil, fmt.Errorf("init: %w", err)
	}
	end.ApplyTo(host.Apply)
	end.StatusFrom(clients.Windows, clients.Commands, host.AccountStatus)
	host.Start(ctx)
	return h, nil
}

// client runs this process as a client of a host another tele keeps, over the
// connection reaching it found. Nothing of the account is opened here: no
// lock, no migration, no owner. The host's settings are its own, from its own
// file, and this client changes them only by asking it (#315).
func (r run) client() error {
	ctx, cancel := context.WithTimeout(context.Background(), attachTimeout)
	defer cancel()
	att, err := hosting.Attach(ctx, r.reached, r.helloAs(wire.KindWindow))
	if err != nil {
		return errors.New(reachFailure(err, r.cfgStore.Current().StateDir))
	}
	return r.runClient(att, app.ClientConfig{
		Notices: app.StartupNotices(r.cfgStore.Current().StateDir, false, false, nil),
	})
}

// attach reaches the host at ep the way any client does.
func (r run) attach(ep ipc.Endpoint) (*dial.Attachment, error) {
	ctx, cancel := context.WithTimeout(context.Background(), helloTimeout)
	defer cancel()
	conn, err := ipc.Dial(ctx, ep)
	if err != nil {
		return nil, fmt.Errorf("reaching this tele's own host: %w", err)
	}
	att, err := dial.Dial(ctx, conn, r.helloAs(wire.KindWindow))
	if err != nil {
		return nil, fmt.Errorf("attaching to this tele's own host: %w", err)
	}
	return att, nil
}

// runClient draws the account over att until the person quits. role is what
// differs between a client of this process's own host and one of another
// tele's: how it reloads, whose settings it shows, what quitting closes and
// which notices it may show; the rest is filled in here.
func (r run) runClient(att *dial.Attachment, role app.ClientConfig) error {
	role.Store = r.cfgStore
	role.Log = r.log
	role.Attachment = att
	role.Detach = func() { _ = att.Close() }
	role.StateDir = r.cfgStore.Current().StateDir
	role.ThemeWarnings = r.themeWarnings
	role.LogPath = r.logPath
	role.Verbose = r.verbose
	if err := app.NewClient(role).Run(); err != nil {
		return fmt.Errorf("error: %w", err)
	}
	return nil
}
