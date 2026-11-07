package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

var statusNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// assertSays fails for every one of wants that text does not say.
func assertSays(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Errorf("status does not say %q:\n%s", want, text)
		}
	}
}

// Status says which tele hosts, what keeps it running and for how long, who is
// attached, and how the account and its link to Telegram are doing (#208).
func TestStatusText_SaysHowTheHostIsDoing(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9", PID: 4242, Kept: wire.KeptByService}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9"}
	status := wire.HostStatus{
		Since:    statusNow.Add(-3 * time.Hour),
		Windows:  2,
		Commands: 1,
		Account: &wire.AccountStatus{
			Phase: wire.PhaseReady, User: "ada",
			Link: wire.LinkUp, LinkSince: statusNow.Add(-time.Hour),
			LastUpdate: statusNow.Add(-12 * time.Second),
			Outbox:     1,
		},
	}

	text := statusText("/state/tele", host, status, nil, mine, statusNow)

	assertSays(t, text, "tele 2.0.0-beta.9", "pid 4242", "/state/tele", "service", "3h0m0s",
		"Windows: 2, commands: 1", "ready", "@ada", "up", "1h0m0s ago", "12s ago", "1 waiting")
	if strings.Contains(text, "This tele") {
		t.Errorf("the same version is not compared:\n%s", text)
	}
}

// A host that holds commands back says how far, since that is why a script
// was refused with exit 7; one that lets them do everything says nothing
// (#181).
func TestStatusText_SaysWhenCommandsAreHeldBack(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", PID: 7, Kept: wire.KeptByService}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"}
	text := func(access wire.CommandAccess) string {
		return statusText("/s", host, wire.HostStatus{Since: statusNow, AccountCommands: access}, nil, mine, statusNow)
	}

	assertSays(t, text(wire.CommandsRead), "cli.account_commands is read")
	assertSays(t, text(wire.CommandsNone), "cli.account_commands is none")
	if all := text(wire.CommandsAll); strings.Contains(all, "cli.account_commands") {
		t.Errorf("a host letting commands do everything says so:\n%s", all)
	}
}

// A host that keeps nothing yet says so rather than inventing it.
func TestStatusText_SaysWhatIsNotThereYet(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", PID: 7, Kept: wire.KeptByWindow}
	status := wire.HostStatus{
		Since:   statusNow,
		Account: &wire.AccountStatus{Phase: wire.PhaseLoggingIn, Link: wire.LinkUnknown},
	}

	text := statusText("/s", host, status, nil, host, statusNow)

	assertSays(t, text, "window", "Windows: 0, commands: 0", "logging in", "not connected yet", "Last update: none", "0 waiting")
}

// A wait Telegram asked for is said with what is left of it and the call it
// holds (#212).
func TestStatusText_SaysAWaitTelegramAskedFor(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kept: wire.KeptByService}
	status := wire.HostStatus{Since: statusNow, Account: &wire.AccountStatus{
		Phase: wire.PhaseReady, Link: wire.LinkUp,
		Throttle: &wire.Throttle{Method: "messages.getHistory", Until: statusNow.Add(32 * time.Second)},
	}}

	text := statusText("/s", host, status, nil, host, statusNow)

	assertSays(t, text, "Telegram asks to wait: 32s, on messages.getHistory")
}

// Between one account and the next there is no account to tell of.
func TestStatusText_BetweenAccounts(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kept: wire.KeptByDaemon}

	text := statusText("/s", host, wire.HostStatus{Since: statusNow}, nil, host, statusNow)

	assertSays(t, text, "tele daemon", "Account: none")
}

// A connection that ended says why.
func TestStatusText_AnEndedConnectionSaysWhy(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kept: wire.KeptByService}
	status := wire.HostStatus{Since: statusNow, Account: &wire.AccountStatus{
		Phase: wire.PhaseConnecting, Ended: "account_banned", Link: wire.LinkDown, LinkSince: statusNow,
	}}

	text := statusText("/s", host, status, nil, host, statusNow)

	assertSays(t, text, "ended", "account_banned", "down")
}

// Versions are compared, and what the difference means is said: a newer
// window restarts a service, an older one is turned away by it.
func TestStatusText_ComparesVersions(t *testing.T) {
	service := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9", Kept: wire.KeptByService}
	status := wire.HostStatus{Since: statusNow}

	newer := statusText("/s", service, status, nil, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0"}, statusNow)
	assertSays(t, newer, "This tele is 2.0.0", "restart")

	older := statusText("/s", service, status, nil, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.1"}, statusNow)
	assertSays(t, older, "This tele is 2.0.0-beta.1", "older")
}

// reportedVersions answers what each command says its version is; a command it
// does not know is not there.
func reportedVersions(known map[string]string) func(string) (string, error) {
	return func(path string) (string, error) {
		v, ok := known[path]
		if !ok {
			return "", os.ErrNotExist
		}
		return v, nil
	}
}

// A service whose command is gone - an upgrade removed the file it names -
// would never start again, and nothing else would say so (#182).
func TestUnitWarning_ACommandThatIsGone(t *testing.T) {
	got := unitWarning("/opt/old/tele", reportedVersions(nil), "2.0.0")

	assertSays(t, got, "/opt/old/tele", "not there", "tele daemon install")
}

// A service whose command is another tele than this one would restart onto it.
func TestUnitWarning_ACommandThatIsAnotherTele(t *testing.T) {
	got := unitWarning("/opt/old/tele", reportedVersions(map[string]string{"/opt/old/tele": "2.0.0-beta.1"}), "2.0.0")

	assertSays(t, got, "/opt/old/tele", "2.0.0-beta.1", "2.0.0", "tele daemon install")
}

// A service that runs this tele is as it should be.
func TestUnitWarning_ACommandThatIsThisTele(t *testing.T) {
	got := unitWarning("/opt/homebrew/bin/tele", reportedVersions(map[string]string{"/opt/homebrew/bin/tele": "2.0.0"}), "2.0.0")

	if got != "" {
		t.Errorf("warned %q", got)
	}
}

// The warning is said whether or not a host answers: a service whose command
// is gone is exactly one that does not (#182).
func TestStatus_WarnsOfAnInstalledServiceWithNoHost(t *testing.T) {
	_, cfg := isolatedHome(t)
	var out bytes.Buffer
	in := installing(&managed{runs: "/opt/old/tele"}, "", &out)
	in.versionOf = reportedVersions(nil)

	err := in.status(context.Background(), options{cfgPath: cfg}, false, &bytes.Buffer{})

	var code *exitCode
	if !errors.As(err, &code) || code.code != noHostCode {
		t.Fatalf("got %v, want exit %d", err, noHostCode)
	}
	assertSays(t, out.String(), "/opt/old/tele", "not there")
}

// isolatedHome points the tele the test runs at a home of its own, with a
// config naming an app key, and gives its state directory and config path.
func isolatedHome(t *testing.T) (stateDir, cfg string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	cfg = filepath.Join(home, "config.yml")
	if err := os.WriteFile(cfg, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, "state", "tele"), cfg
}

// With no host to answer, status says so and exits 3, which a script can tell
// from any other failure (#208).
func TestStatusCommand_NoHostExitsThree(t *testing.T) {
	bin := buildTele(t)
	_, cfg := isolatedHome(t)

	out, err := exec.Command(bin, "daemon", "status", "--config", cfg).CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("tele daemon status with no host: %v, want exit 3\n%s", err, out)
	}
	assertSays(t, string(out), "No tele hosts")
}

// A host that answers is told of, and status exits 0.
func TestStatusCommand_AHostThatAnswersIsToldOf(t *testing.T) {
	bin := buildTele(t)
	stateDir, cfg := isolatedHome(t)
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

	out, err := exec.CommandContext(ctx, bin, "daemon", "status", "--config", cfg).CombinedOutput()

	if err != nil {
		t.Fatalf("tele daemon status: %v\n%s", err, out)
	}
	assertSays(t, string(out), "tele 2.0.0-beta.9", "pid 4242")
}

// A host on another protocol is named by its hello alone: nothing else was
// asked of it.
func TestStatusText_AHostOnAnotherProtocolIsOnlyNamed(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion + 1, Binary: "3.0.0", PID: 9, Kept: wire.KeptByWindow}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0"}

	text := statusText("/s", host, wire.HostStatus{}, &dial.Mismatch{Verdict: wire.Refuse, Host: host}, mine, statusNow)

	assertSays(t, text, "tele 3.0.0", "pid 9", "This tele is 2.0.0", "cannot")
	if strings.Contains(text, "Account") {
		t.Errorf("a host on another protocol was not asked about its account:\n%s", text)
	}
}
