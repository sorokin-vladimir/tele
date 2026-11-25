package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/app"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/service"
	"github.com/sorokin-vladimir/tele/internal/version"
)

// installer installs and uninstalls the service of a config with the system's
// service manager, and checks the one installed (#182).
type installer struct {
	manager func() (service.Manager, error)
	// executable is the running binary, which is what installs the service.
	executable func() (string, error)
	// versionOf is what the command at a path says its version is.
	versionOf func(path string) (string, error)
	out       io.Writer
}

// newInstaller is the installer of this system, saying what it does on cmd's
// output, and showing a long wait for the running service to stop on its
// error output (#327).
func newInstaller(cmd *cobra.Command) installer {
	return systemInstaller(cmd.OutOrStdout(), newIndicator(cmd.ErrOrStderr()).waiting)
}

// systemInstaller is the installer of this system, saying what it does on out
// and telling waiting, if set, of a wait for the running service to stop. A
// window sets none: it would draw over its own screen.
func systemInstaller(out io.Writer, waiting func() (done func())) installer {
	return installer{
		manager: func() (service.Manager, error) {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			return service.ForThisSystem(home, waiting)
		},
		executable: os.Executable,
		versionOf:  versionOf,
		out:        out,
	}
}

// versionOf runs the tele at path for its version.
func versionOf(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), helloTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// newInstallCmds are `tele daemon install` and `tele daemon uninstall`.
func newInstallCmds(o *options) []*cobra.Command {
	in := newInstaller
	var path string
	install := &cobra.Command{
		Use:   "install",
		Short: "Keep the account held at every login, with no window, as a service",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return in(cmd).install(cmd.Context(), *o, path)
		},
	}
	install.Flags().StringVar(&path, "path", "", "the command the service runs, in place of tele as the PATH finds it")
	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop the service and remove it; state and config are left alone",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return in(cmd).uninstall(cmd.Context(), *o)
		},
	}
	return []*cobra.Command{install, uninstall}
}

// unitOf is the service of the config read by s, running entry.
func unitOf(s setup, entry string) (service.Unit, error) {
	cfgPath, err := filepath.Abs(s.cfgPath)
	if err != nil {
		return service.Unit{}, err
	}
	defaultConfig := cfgPath == config.ExpandTilde(defaultConfigPath(appName))
	return service.Unit{
		Name:    service.Name(appName, s.store.Current().StateDir, defaultConfig),
		Binary:  entry,
		Config:  cfgPath,
		LogPath: filepath.Join(s.logDir, "service.log"),
	}, nil
}

// install installs the service of o's config, running given or tele as the
// PATH finds it, and starts it.
func (in installer) install(ctx context.Context, o options, given string) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	// A service that cannot start is no service: it would only be started
	// again and again.
	if _, err := s.appKey(); err != nil {
		return err
	}
	cfg := s.store.Current()
	m, err := in.manager()
	if errors.Is(err, service.ErrUnsupported) {
		cfgPath, _ := filepath.Abs(s.cfgPath)
		return fmt.Errorf("%w; run `%s daemon --config %s` from your session's startup instead", err, appName, cfgPath)
	}
	if err != nil {
		return err
	}
	running, err := in.executable()
	if err != nil {
		return err
	}
	entry, err := service.EntryPoint(appName, running, given)
	if err != nil {
		return err
	}
	u, err := unitOf(s, entry)
	if err != nil {
		return err
	}
	holder, held := whoHolds(ctx, cfg.StateDir)
	if err := m.Install(u); errors.Is(err, service.ErrStillRunning) {
		// The new service is written and not started: an install once the
		// old one has gone starts it (#327).
		return fmt.Errorf("installing the service: %w; run `%s daemon install` again", err, appName)
	} else if err != nil {
		return fmt.Errorf("installing the service: %w", err)
	}
	_, _ = fmt.Fprintf(in.out, "Installed the service %s: it runs %s at every login, and now.\n", u.Name, entry)
	switch {
	case !held || holder.Kept == wire.KeptByService:
	case holder.PID == os.Getpid():
		// Installed from the window of the tele that hosts (#208).
		_, _ = fmt.Fprintln(in.out, "The service takes over when this tele quits.")
	default:
		_, _ = fmt.Fprintf(in.out, "tele %s (pid %d) holds %s now; the service takes over when it quits.\n",
			holder.Binary, holder.PID, cfg.StateDir)
	}
	return nil
}

// offer is the service of o's config for a tele that hosts to offer, or nil
// where a yes could not install it: with no service manager, with another tele
// on the PATH, which only `--path` could name, or with the service installed
// already (#208). A yes installs it as `tele daemon install` would, and says
// what that would print.
func (in installer) offer(o options) *app.ServiceOffer {
	m, err := in.manager()
	if err != nil {
		return nil
	}
	running, err := in.executable()
	if err != nil {
		return nil
	}
	if _, err := service.EntryPoint(appName, running, ""); err != nil {
		return nil
	}
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return nil
	}
	u, err := unitOf(s, "")
	if err != nil {
		return nil
	}
	if _, installed, err := m.Installed(u.Name); err != nil || installed {
		return nil
	}
	return &app.ServiceOffer{App: appName, Install: func(ctx context.Context) (string, error) {
		var said strings.Builder
		in.out = &said
		err := in.install(ctx, o, "")
		return strings.TrimSpace(said.String()), err
	}}
}

// uninstall tells the service of o's config it is being uninstalled, so it
// stops for good, and removes its unit. State and config are left alone.
func (in installer) uninstall(ctx context.Context, o options) error {
	s, err := newSetup(o.cfgPath)
	if err != nil {
		return err
	}
	cfg := s.store.Current()
	m, err := in.manager()
	if errors.Is(err, service.ErrUnsupported) {
		_, _ = fmt.Fprintln(in.out, "No tele service is installed: this system has none to install.")
		return nil
	}
	if err != nil {
		return err
	}
	u, err := unitOf(s, "")
	if err != nil {
		return err
	}
	tellUninstalled(ctx, cfg.StateDir)
	removed, err := m.Uninstall(u.Name)
	if err != nil {
		return fmt.Errorf("uninstalling the service: %w", err)
	}
	if !removed {
		_, _ = fmt.Fprintf(in.out, "No tele service is installed for %s.\n", u.Config)
		return nil
	}
	_, _ = fmt.Fprintf(in.out, "Uninstalled the service %s. State and config are left as they were.\n", u.Name)
	return nil
}

// whoHolds is what the tele holding stateDir says of itself, if one answers.
func whoHolds(ctx context.Context, stateDir string) (wire.Hello, bool) {
	ctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	conn, ok := reachHolder(ctx, stateDir)
	if !ok {
		return wire.Hello{}, false
	}
	host, err := dial.Introduce(ctx, conn, ownHello())
	return host, err == nil
}

// tellUninstalled tells the service holding stateDir, if one does, that it is
// being uninstalled. Anything else holding it refuses, which changes nothing.
func tellUninstalled(ctx context.Context, stateDir string) {
	ctx, cancel := context.WithTimeout(ctx, helloTimeout)
	defer cancel()
	conn, ok := reachHolder(ctx, stateDir)
	if !ok {
		return
	}
	_, _ = dial.AskUninstall(ctx, conn, ownHello())
}

// reachHolder connects to the tele holding stateDir, if one answers. It never
// becomes one.
func reachHolder(ctx context.Context, stateDir string) (io.ReadWriteCloser, bool) {
	if _, err := os.Stat(stateDir); err != nil {
		return nil, false
	}
	ep, err := ipc.EndpointFor(appName, stateDir)
	if err != nil {
		return nil, false
	}
	conn, err := ipc.Dial(ctx, ep)
	if err != nil {
		return nil, false
	}
	return conn, true
}

// ownHello is what a command that only asks the host says of itself.
func ownHello() wire.Hello {
	return wire.Hello{Protocol: wire.ProtocolVersion, Binary: version.Version, Kind: wire.KindCommand}
}
