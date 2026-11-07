package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/service"
)

// managed is a service manager the test reads back: what was installed, and
// what was uninstalled.
type managed struct {
	installed   []service.Unit
	uninstalled []string
	// none says nothing is installed to uninstall.
	none bool
	// runs is the command the installed service runs, empty when none is
	// installed.
	runs string
	// failing is what Install fails with.
	failing error
}

func (m *managed) Installed(string) (string, bool, error) { return m.runs, m.runs != "", nil }

func (m *managed) Install(u service.Unit) error {
	m.installed = append(m.installed, u)
	return m.failing
}

func (m *managed) Uninstall(name string) (bool, error) {
	m.uninstalled = append(m.uninstalled, name)
	return !m.none, nil
}

// teleOnPath puts a tele on a PATH of its own, as a package manager would,
// and gives the file that runs and the name the PATH finds.
func teleOnPath(t *testing.T) (running, onPath string) {
	t.Helper()
	name := "tele"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	onPath = filepath.Join(dir, name)
	if err := os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return onPath, onPath
}

// installing is an installer over m, as a tele that runs running.
func installing(m *managed, running string, out *bytes.Buffer) installer {
	return installer{
		manager:    func() (service.Manager, error) { return m, nil },
		executable: func() (string, error) { return running, nil },
		out:        out,
	}
}

// The service runs the tele on the PATH, as a service, on the config it was
// installed with, named after that config, and logs beside tele's log (#182).
func TestInstall_RegistersTheServiceOfThisConfig(t *testing.T) {
	_, cfg := isolatedHome(t)
	running, onPath := teleOnPath(t)
	m := &managed{}
	var out bytes.Buffer

	err := installing(m, running, &out).install(context.Background(), options{cfgPath: cfg}, "")

	if err != nil {
		t.Fatal(err)
	}
	if len(m.installed) != 1 {
		t.Fatalf("installed %d units", len(m.installed))
	}
	u := m.installed[0]
	if u.Binary != onPath || u.Config != cfg || !strings.HasPrefix(u.Name, "tele-") {
		t.Errorf("installed %+v", u)
	}
	if filepath.Base(u.LogPath) != "service.log" {
		t.Errorf("the service logs to %q", u.LogPath)
	}
	assertSays(t, out.String(), "Installed", onPath)
}

// A service that outlasted the wait for it to stop left the new one written
// and not started: installing again is what starts it, and the error says so
// (#327).
func TestInstall_AnOldServiceThatStaysAsksForAnotherInstall(t *testing.T) {
	_, cfg := isolatedHome(t)
	running, _ := teleOnPath(t)
	m := &managed{failing: fmt.Errorf("%w: launchctl: Bootstrap failed: 5: Input/output error", service.ErrStillRunning)}
	var out bytes.Buffer

	err := installing(m, running, &out).install(context.Background(), options{cfgPath: cfg}, "")

	if err == nil {
		t.Fatal("installed over a service that did not stop")
	}
	assertSays(t, err.Error(), "did not stop in 30s", "Input/output error", "run `tele daemon install` again")
	if out.Len() != 0 {
		t.Errorf("said %q", out.String())
	}
}

// The default config's service is named after the build alone.
func TestInstall_TheDefaultConfigsServiceIsNamedAfterTheBuild(t *testing.T) {
	isolatedHome(t)
	home, _ := os.UserHomeDir()
	cfg := filepath.Join(home, ".config", "tele", "config.yml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, _ := teleOnPath(t)
	m := &managed{}

	err := installing(m, running, &bytes.Buffer{}).install(context.Background(), options{cfgPath: defaultConfigPath("tele")}, "")

	if err != nil {
		t.Fatal(err)
	}
	if m.installed[0].Name != "tele" {
		t.Errorf("the default config's service is named %q", m.installed[0].Name)
	}
}

// A tele that is not the one on the PATH installs nothing, and says why.
func TestInstall_RefusesWhenThePathFindsAnotherTele(t *testing.T) {
	_, cfg := isolatedHome(t)
	teleOnPath(t)
	elsewhere := filepath.Join(t.TempDir(), "tele")
	if err := os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &managed{}

	err := installing(m, elsewhere, &bytes.Buffer{}).install(context.Background(), options{cfgPath: cfg}, "")

	var notOnPath *service.NotOnPathError
	if !errors.As(err, &notOnPath) {
		t.Fatalf("got %v", err)
	}
	if len(m.installed) != 0 {
		t.Error("a service was installed on another tele")
	}
}

// --path names the command the service runs, whatever the PATH finds.
func TestInstall_APathGivenIsRun(t *testing.T) {
	_, cfg := isolatedHome(t)
	teleOnPath(t)
	elsewhere := filepath.Join(t.TempDir(), "tele")
	if err := os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &managed{}

	err := installing(m, elsewhere, &bytes.Buffer{}).install(context.Background(), options{cfgPath: cfg}, elsewhere)

	if err != nil {
		t.Fatal(err)
	}
	if m.installed[0].Binary != elsewhere {
		t.Errorf("the service runs %q", m.installed[0].Binary)
	}
}

// Where there is no service manager to install with, the host is run by
// hand, and the person is told how.
func TestInstall_WithNoServiceManagerSaysHowToRunTheHost(t *testing.T) {
	_, cfg := isolatedHome(t)
	running, _ := teleOnPath(t)
	in := installing(&managed{}, running, &bytes.Buffer{})
	in.manager = func() (service.Manager, error) { return nil, service.ErrUnsupported }

	err := in.install(context.Background(), options{cfgPath: cfg}, "")

	if err == nil {
		t.Fatal("installing with no service manager succeeded")
	}
	assertSays(t, err.Error(), "tele daemon --config", cfg)
}

// A tele already holding the state directory keeps it: the service waits, and
// takes over when that one quits. The person is told so.
func TestInstall_SaysTheServiceTakesOverFromATeleRunningNow(t *testing.T) {
	stateDir, cfg := isolatedHome(t)
	running, _ := teleOnPath(t)
	serveHeld(t, stateDir, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0", PID: 4242, Kept: wire.KeptByWindow})
	var out bytes.Buffer

	err := installing(&managed{}, running, &out).install(context.Background(), options{cfgPath: cfg}, "")

	if err != nil {
		t.Fatal(err)
	}
	assertSays(t, out.String(), "pid 4242", "takes over")
}

// uninstalledHost is a service that keeps whether it was told it is being
// uninstalled.
type uninstalledHost struct {
	heldHost
	mu   *sync.Mutex
	told *bool
}

func (h uninstalledHost) Uninstall() wire.UninstallResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.told = true
	return wire.UninstallResult{}
}

// Uninstalling tells the service first, so that it stops for good, and then
// removes its unit; state and config are left alone (#182).
func TestUninstall_TellsTheServiceAndRemovesItsUnit(t *testing.T) {
	stateDir, cfg := isolatedHome(t)
	var mu sync.Mutex
	told := false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := hosting.Reach(ctx, "tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	host := uninstalledHost{heldHost: heldHost{wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kept: wire.KeptByService}}, mu: &mu, told: &told}
	srv := hosting.Serve(context.Background(), r.Listener, host, hosting.NewClients())
	t.Cleanup(func() {
		srv.Stop(time.Second, core.StopQuit)
		r.Close()
	})
	m := &managed{}
	var out bytes.Buffer

	err = installing(m, "", &out).uninstall(ctx, options{cfgPath: cfg})

	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !told {
		t.Error("the service was not told it is being uninstalled")
	}
	if len(m.uninstalled) != 1 || !strings.HasPrefix(m.uninstalled[0], "tele-") {
		t.Errorf("uninstalled %v", m.uninstalled)
	}
	assertSays(t, out.String(), "Uninstalled", "left")
}

// With nothing installed, uninstalling says so and succeeds.
func TestUninstall_NothingInstalled(t *testing.T) {
	_, cfg := isolatedHome(t)
	var out bytes.Buffer

	err := installing(&managed{none: true}, "", &out).uninstall(context.Background(), options{cfgPath: cfg})

	if err != nil {
		t.Fatal(err)
	}
	assertSays(t, out.String(), "No tele service is installed")
}

// serveHeld serves the endpoint of stateDir as another tele saying hello,
// for the length of the test.
func serveHeld(t *testing.T, stateDir string, hello wire.Hello) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := hosting.Reach(ctx, "tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := hosting.Serve(context.Background(), r.Listener, heldHost{hello}, hosting.NewClients())
	t.Cleanup(func() {
		srv.Stop(time.Second, core.StopQuit)
		r.Close()
	})
}

// A tele that hosts offers to install its service where installing would
// succeed, and a yes installs it, saying what it did (#208).
func TestOffer_AServiceThatCanBeInstalledIsOffered(t *testing.T) {
	_, cfg := isolatedHome(t)
	running, onPath := teleOnPath(t)
	m := &managed{}

	offer := installing(m, running, &bytes.Buffer{}).offer(options{cfgPath: cfg})

	if offer == nil {
		t.Fatal("nothing offered")
	}
	if offer.App != appName {
		t.Errorf("offered as %q", offer.App)
	}
	text, err := offer.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.installed) != 1 {
		t.Fatalf("installed %d units", len(m.installed))
	}
	assertSays(t, text, "Installed", onPath)
}

// Nothing is offered that a yes could not install, nor what is installed
// already.
func TestOffer_NothingThatCannotBeInstalled(t *testing.T) {
	_, cfg := isolatedHome(t)
	running, onPath := teleOnPath(t)
	elsewhere := filepath.Join(t.TempDir(), "tele")
	if err := os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	unsupported := installing(&managed{}, running, &bytes.Buffer{})
	unsupported.manager = func() (service.Manager, error) { return nil, service.ErrUnsupported }

	for name, in := range map[string]installer{
		"no service manager":       unsupported,
		"another tele on the PATH": installing(&managed{}, elsewhere, &bytes.Buffer{}),
		"the service installed":    installing(&managed{runs: onPath}, running, &bytes.Buffer{}),
	} {
		if offer := in.offer(options{cfgPath: cfg}); offer != nil {
			t.Errorf("%s: offered", name)
		}
	}
}

// Installed from the window of the tele that holds the state directory, the
// service waits for that very tele, and is said to.
func TestOffer_TheServiceTakesOverFromTheTeleThatInstalledIt(t *testing.T) {
	stateDir, cfg := isolatedHome(t)
	running, _ := teleOnPath(t)
	serveHeld(t, stateDir, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0", PID: os.Getpid(), Kept: wire.KeptByWindow})

	text, err := installing(&managed{}, running, &bytes.Buffer{}).offer(options{cfgPath: cfg}).Install(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	assertSays(t, text, "takes over when this tele quits")
	if strings.Contains(text, "pid") {
		t.Errorf("names its own pid:\n%s", text)
	}
}
