package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// heldHost is another tele hosting the state directory: it says who it is and
// holds no settings.
type heldHost struct{ hello wire.Hello }

func (h heldHost) Hello() wire.Hello                      { return h.hello }
func (heldHost) Settings() wire.HostSettings              { return wire.HostSettings{} }
func (heldHost) SetSetting(wire.SetParams) wire.SetResult { return wire.SetResult{} }
func (heldHost) Reload() wire.ReloadResult                { return wire.ReloadResult{} }
func (heldHost) Status() wire.HostStatus                  { return wire.HostStatus{} }
func (heldHost) Restart() (wire.RestartResult, func())    { return wire.RestartResult{}, nil }
func (heldHost) Uninstall() wire.UninstallResult          { return wire.UninstallResult{} }
func (heldHost) AccountCommands() wire.CommandAccess      { return wire.CommandsAll }

// `tele daemon` only ever hosts: with the state directory already hosted by
// another tele, it stops and names that tele by its pid and version (#208).
func TestDaemon_RefusesAStateDirAnotherTeleHosts(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	first, err := hosting.Reach(ctx, "tele-test", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	other := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9", PID: 4242, Kept: wire.KeptByWindow}
	srv := hosting.Serve(context.Background(), first.Listener, heldHost{other}, hosting.NewClients())
	t.Cleanup(func() {
		srv.Stop(time.Second, core.StopQuit)
		first.Close()
	})

	r, err := reachAsDaemon(ctx, "tele-test", stateDir, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"})
	if err == nil {
		r.Close()
		t.Fatal("a daemon took a state directory another tele hosts")
	}
	for _, want := range []string{"pid 4242", "2.0.0-beta.9", stateDir} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not say %q", err, want)
		}
	}
}

// `tele daemon` is a command of its own: run over a state directory another
// tele hosts, it names that tele and exits with a failure.
func TestDaemonCommand_ExitsNamingTheTeleThatHosts(t *testing.T) {
	bin := buildTele(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	stateDir := filepath.Join(home, "state", "tele")
	cfg := filepath.Join(home, "config.yml")
	if err := os.WriteFile(cfg, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	first, err := hosting.Reach(ctx, "tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	other := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9", PID: 4242, Kept: wire.KeptByWindow}
	srv := hosting.Serve(context.Background(), first.Listener, heldHost{other}, hosting.NewClients())
	t.Cleanup(func() {
		srv.Stop(time.Second, core.StopQuit)
		first.Close()
	})

	out, err := exec.CommandContext(ctx, bin, "daemon", "--config", cfg).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("tele daemon over a hosted state directory: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "pid 4242") {
		t.Fatalf("tele daemon printed %q, want it to name pid 4242", out)
	}
}

// A service never exits over a state directory another tele holds, or its
// service manager would start it again in a loop: it says it is waiting, and
// hosts once the other tele quits, saying it is a service (#208).
func TestDaemonCommand_AServiceWaitsForTheStateDirAndHostsAsAService(t *testing.T) {
	bin := buildTele(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	stateDir := filepath.Join(home, "state", "tele")
	cfg := filepath.Join(home, "config.yml")
	if err := os.WriteFile(cfg, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	first, err := hosting.Reach(ctx, "tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := hosting.Serve(context.Background(), first.Listener, heldHost{wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"}}, hosting.NewClients())

	cmd := exec.CommandContext(ctx, bin, "daemon", "--service", "--config", cfg)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	lines := make(chan string, 100)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			lines <- s.Text()
		}
		close(lines)
	}()
	// The holder is named by the pid in the lock, which is this test's.
	waitForLine(t, lines, fmt.Sprintf("pid %d", os.Getpid()))

	srv.Stop(time.Second, core.StopQuit)
	first.Close()
	waitForLine(t, lines, "hosts")

	conn, err := ipc.Dial(ctx, first.Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	host, err := dial.Introduce(ctx, conn, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if host.Kept != wire.KeptByService || host.PID != cmd.Process.Pid {
		t.Fatalf("the service said it is kept by %q with pid %d, want %q and %d", host.Kept, host.PID, wire.KeptByService, cmd.Process.Pid)
	}
}

// A service asked to restart says so to its clients and exits as a failure,
// for its service manager to start it again on the binary installed now
// (#183).
func TestDaemonCommand_AServiceRestartsWhenAsked(t *testing.T) {
	bin := buildTele(t)
	stateDir, cfg := isolatedHome(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "daemon", "--service", "--config", cfg)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	lines := make(chan string, 100)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			lines <- s.Text()
		}
		close(lines)
	}()
	waitForLine(t, lines, "hosts")

	ep, err := ipc.EndpointFor("tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := ipc.Dial(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dial.Dial(ctx, conn, wire.Hello{Protocol: wire.ProtocolVersion, Binary: builtVersion})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	got, err := a.RestartHost(ctx)
	if err != nil || got.Refused != "" {
		t.Fatalf("the service did not agree to restart: %v %q", err, got.Refused)
	}

	stopping := false
	for msg := range a.Stream() {
		if s, ok := msg.(core.HostStopping); ok {
			stopping = s.Reason == core.StopRestart
		}
	}
	if !stopping {
		t.Error("the client did not hear the service stop to restart")
	}
	go func() {
		for range lines {
		}
	}()
	var exit *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exit) || exit.ExitCode() != restartCode {
		t.Fatalf("the service exited with %v, want %d", err, restartCode)
	}
}

// A service told it is being uninstalled, then stopped by its service
// manager, tells its clients it stopped for good (#182).
func TestDaemonCommand_AnUninstalledServiceSaysSoWhenStopped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a process is stopped with a signal only where there are signals")
	}
	bin := buildTele(t)
	stateDir, cfg := isolatedHome(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "daemon", "--service", "--config", cfg)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	lines := make(chan string, 100)
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			lines <- s.Text()
		}
		close(lines)
	}()
	waitForLine(t, lines, "hosts")
	ep, err := ipc.EndpointFor("tele", stateDir)
	if err != nil {
		t.Fatal(err)
	}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: builtVersion}
	conn, err := ipc.Dial(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	a, err := dial.Dial(ctx, conn, mine)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	told, err := ipc.Dial(ctx, ep)
	if err != nil {
		t.Fatal(err)
	}
	got, err := dial.AskUninstall(ctx, told, mine)
	if err != nil || got.Refused != "" {
		t.Fatalf("the service did not take note: %v %q", err, got.Refused)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	var reason core.StopReason
	for msg := range a.Stream() {
		if s, ok := msg.(core.HostStopping); ok {
			reason = s.Reason
		}
	}
	if reason != core.StopUninstalled {
		t.Fatalf("the client heard the service stop for %q, want %q", reason, core.StopUninstalled)
	}
}

// waitForLine waits for a line of lines that says want, failing the test when
// they end first.
func waitForLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	for line := range lines {
		if strings.Contains(line, want) {
			return
		}
	}
	t.Fatalf("nothing said %q", want)
}

// With nobody there, the daemon is the host.
func TestDaemon_HostsAStateDirNobodyHolds(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, err := reachAsDaemon(ctx, "tele-test", stateDir, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.Listener == nil {
		t.Fatal("the daemon did not become the host")
	}
}
