package main

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// decoded is the JSON statusJSON wrote, read back as a script would.
func decoded(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	return v
}

// `tele daemon status --json` says what the text says, under the names the
// CLI documents, for a script to read (#213).
func TestStatusJSON_SaysHowTheHostIsDoing(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.9", PID: 4242,
		Kept: wire.KeptByService, Config: "/c.yml"}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0-beta.10"}
	until := statusNow.Add(32 * time.Second)
	status := wire.HostStatus{
		Since: statusNow.Add(-time.Hour), Windows: 2, Commands: 1, AccountCommands: wire.CommandsRead,
		Account: &wire.AccountStatus{
			Phase: wire.PhaseReady, User: "ada", Link: wire.LinkUp,
			LinkSince: statusNow.Add(-time.Minute), LastUpdate: statusNow.Add(-time.Second), Outbox: 3,
			Throttle: &wire.Throttle{Method: "messages.getHistory", Until: until},
		},
	}

	raw, err := statusJSON("/state", host, status, nil, mine, "the service runs /gone")
	if err != nil {
		t.Fatal(err)
	}
	v := decoded(t, raw)

	want := map[string]any{
		"state_dir": "/state",
		"host": map[string]any{
			"binary": "2.0.0-beta.9", "protocol": float64(wire.ProtocolVersion), "pid": float64(4242),
			"kept": "service", "config": "/c.yml",
		},
		"since":            statusNow.Add(-time.Hour).Format(time.RFC3339),
		"windows":          float64(2),
		"commands":         float64(1),
		"account_commands": "read",
		"account": map[string]any{
			"phase": "ready", "user": "ada", "link": "up",
			"link_since":  statusNow.Add(-time.Minute).Format(time.RFC3339),
			"last_update": statusNow.Add(-time.Second).Format(time.RFC3339),
			"outbox":      float64(3),
			"throttle":    map[string]any{"method": "messages.getHistory", "until": until.Format(time.RFC3339)},
		},
		"this":            map[string]any{"binary": "2.0.0-beta.10", "protocol": float64(wire.ProtocolVersion)},
		"service_warning": "the service runs /gone",
	}
	assertSameJSON(t, want, v)
}

// What is not there is left out rather than said as zero: no account between
// accounts, no wait, no warning, and with a host on another protocol nothing
// it was never asked.
func TestStatusJSON_LeavesOutWhatIsNotThere(t *testing.T) {
	host := wire.Hello{Protocol: wire.ProtocolVersion + 1, Binary: "3.0.0", Kept: wire.KeptByWindow}
	mine := wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0"}

	raw, err := statusJSON("/state", host, wire.HostStatus{}, &dial.Mismatch{}, mine, "")
	if err != nil {
		t.Fatal(err)
	}
	v := decoded(t, raw)

	for _, key := range []string{"since", "windows", "commands", "account_commands", "account", "service_warning"} {
		if _, ok := v[key]; ok {
			t.Errorf("%q is there: %s", key, raw)
		}
	}
	if v["host"].(map[string]any)["binary"] != "3.0.0" {
		t.Errorf("the host is not named: %s", raw)
	}
}

// With no host there is nothing to read: stdout stays empty and the exit code
// says it, as without --json.
func TestStatusJSON_NoHostPrintsNothing(t *testing.T) {
	bin := buildTele(t)
	isolatedHome(t)
	cfg := configAt(t, filepath.Join(t.TempDir(), "nobody"))

	out, err := exec.Command(bin, "-c", cfg, "daemon", "status", "--json").Output()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != noHostCode {
		t.Fatalf("exited %v", err)
	}
	if len(out) != 0 {
		t.Errorf("printed %q", out)
	}
}

// The flag is what turns it to JSON: asked of a host that answers, the
// command prints one object, naming that host.
func TestStatusJSON_TheFlagPrintsJSON(t *testing.T) {
	bin := buildTele(t)
	stateDir, cfg := isolatedHome(t)
	serveHeld(t, stateDir, wire.Hello{Protocol: wire.ProtocolVersion, Binary: "2.0.0", PID: 4242, Kept: wire.KeptByWindow})

	out, err := exec.Command(bin, "-c", cfg, "daemon", "status", "--json").Output()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}

	v := decoded(t, out)
	if v["host"].(map[string]any)["pid"] != float64(4242) {
		t.Errorf("another host: %s", out)
	}
}

func assertSameJSON(t *testing.T, want, got any) {
	t.Helper()
	w, _ := json.Marshal(want)
	g, _ := json.Marshal(got)
	if string(w) != string(g) {
		t.Errorf("got\n%s\nwant\n%s", g, w)
	}
}
