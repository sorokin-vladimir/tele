package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// newDaemonCmd is `tele daemon`: the host with no window, for whoever wants
// tele to keep the account between windows.
func newDaemonCmd(o *options) *cobra.Command {
	var service bool
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Host the account with no window, until stopped",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kept := wire.KeptByDaemon
			if service {
				kept = wire.KeptByService
			}
			return runDaemon(cmd.Context(), *o, kept)
		},
	}
	// Said by the unit a service manager runs, never by a person: it is what
	// tells a host that it is a service, and nothing is inferred from the
	// environment (#208).
	cmd.Flags().BoolVar(&service, "service", false, "run as the service the service manager keeps running")
	cmd.AddCommand(newStatusCmd(o))
	cmd.AddCommand(newInstallCmds(o)...)
	return cmd
}

// runDaemon hosts the state directory with no window until the process is
// told to stop.
func runDaemon(ctx context.Context, o options, kept wire.Keeper) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	key, err := s.appKey()
	if err != nil {
		return err
	}
	if err := s.makeLogDir(); err != nil {
		return err
	}
	cfg := s.store.Current()
	r := run{cfgStore: s.store, cfgPath: s.cfgPath, key: key, verbose: o.verbose, trace: o.trace}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	log, logPath, closeLog := openLog(s.logDir, true, o.verbose)
	defer func() {
		_ = log.Sync()
		_ = closeLog()
	}()
	for _, w := range cfg.Warnings {
		log.Warn("config: " + w.Text)
		fmt.Fprintf(os.Stderr, "config: %s\n", w.Text)
	}

	var reached hosting.Reached
	if kept == wire.KeptByService {
		reached, err = hosting.Await(ctx, appName, cfg.StateDir, func(holder error) {
			log.Warn("the state directory is held; waiting for it", zap.Error(holder))
			fmt.Fprintf(os.Stderr, "Another tele holds %s (%v). Waiting for it to quit.\n", cfg.StateDir, holder)
		})
		if errors.Is(err, context.Canceled) {
			return nil // stopped while it waited
		}
	} else {
		// It asks the tele in its way who it is, and goes: a command (#213).
		reached, err = reachAsDaemon(ctx, appName, cfg.StateDir, r.helloAs(wire.KindCommand))
	}
	if err != nil {
		return err
	}
	r.log, r.logPath, r.reached = log, logPath, reached

	// A service's stderr goes to its service manager's log, where nobody can
	// press Ctrl+C.
	if kept == wire.KeptByService {
		fmt.Fprintf(os.Stderr, "tele %s (pid %d) hosts %s as a service.\n", version.Version, os.Getpid(), cfg.StateDir)
	} else {
		fmt.Fprintf(os.Stderr, "tele %s (pid %d) hosts %s. Ctrl+C stops it.\n", version.Version, os.Getpid(), cfg.StateDir)
	}
	return r.daemon(ctx, kept)
}

// reachAsDaemon reaches the host of stateDir the way `tele daemon` does: it
// becomes the host, or stops. A daemon only ever hosts, so a tele already
// hosting the state directory is named, by its version and pid, rather than
// attached to (#208).
func reachAsDaemon(ctx context.Context, app, stateDir string, own wire.Hello) (hosting.Reached, error) {
	r, err := hosting.Reach(ctx, app, stateDir)
	if err != nil {
		return hosting.Reached{}, errors.New(reachFailure(err, stateDir))
	}
	if r.Listener != nil {
		return r, nil
	}
	host, err := dial.Introduce(ctx, r.Conn, own)
	if err != nil {
		return hosting.Reached{}, fmt.Errorf("another tele already hosts %s, and did not say which: %v", stateDir, err)
	}
	return hosting.Reached{}, errors.New(alreadyHosted(host, stateDir))
}

// alreadyHosted is what a daemon says of the tele it found hosting stateDir.
// It is printed as the daemon exits, so it says what to do about it.
func alreadyHosted(host wire.Hello, stateDir string) string {
	return fmt.Sprintf("tele %s (pid %d) already hosts %s, and a daemon only ever hosts.\nQuit it and start the daemon again.",
		host.Binary, host.PID, stateDir)
}
