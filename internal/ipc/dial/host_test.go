package dial_test

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/serve"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// hostEnd answers the calls about the host with what it was given, and keeps
// what it was asked.
type hostEnd struct {
	hello     wire.Hello
	settings  wire.HostSettings
	set       wire.SetResult
	reload    wire.ReloadResult
	status    wire.HostStatus
	restart   wire.RestartResult
	uninstall wire.UninstallResult

	mu          sync.Mutex
	uninstalled bool
	sets        []wire.SetParams
}

func (h *hostEnd) Hello() wire.Hello                     { return h.hello }
func (h *hostEnd) Settings() wire.HostSettings           { return h.settings }
func (h *hostEnd) Reload() wire.ReloadResult             { return h.reload }
func (h *hostEnd) Status() wire.HostStatus               { return h.status }
func (h *hostEnd) Restart() (wire.RestartResult, func()) { return h.restart, nil }
func (h *hostEnd) AccountCommands() wire.CommandAccess   { return wire.CommandsAll }
func (h *hostEnd) Uninstall() wire.UninstallResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.uninstalled = true
	return h.uninstall
}
func (h *hostEnd) SetSetting(p wire.SetParams) wire.SetResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sets = append(h.sets, p)
	return h.set
}

// connectTo is an attachment over a connection to host, with no owner behind
// the attachment: the calls about the host are answered all the same, as on
// the login screen or after a log out.
func connectTo(t *testing.T, host *hostEnd) *dial.Attachment {
	t.Helper()
	client, end := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = serve.Conn(ctx, end, core.NewAttachment(), host)
	}()
	a, err := dial.Dial(ctx, client, hello)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = a.Close()
		cancel()
		<-served
	})
	return a
}

// The host's settings cross as they are: the file it reads them from, and each
// one's value and whether the file names it (#315).
func TestHost_SettingsCross(t *testing.T) {
	host := &hostEnd{hello: hello, settings: wire.HostSettings{
		Config: "/home/a/.config/tele/config.yml",
		Settings: []wire.Setting{
			{Key: "ui.history_limit", Value: json.RawMessage("120")},
			{Key: "ui.notifications.desktop", Value: json.RawMessage("true"), Default: true},
		},
	}}
	a := connectTo(t, host)

	got, err := a.HostSettings(context.Background())

	require.NoError(t, err)
	assert.Equal(t, host.settings, got)
}

// A change to a host setting reaches the host as the key and the value, and
// what the host said of it comes back: warnings, or a refusal, which is an
// answer rather than a failure of the call.
func TestHost_SetCrossesBothWays(t *testing.T) {
	host := &hostEnd{hello: hello, set: wire.SetResult{
		Refused:  "must be at least 1",
		Warnings: []wire.Warning{{Text: "ui.history_limit: repaired", ID: "x"}},
	}}
	a := connectTo(t, host)

	got, err := a.SetHostSetting(context.Background(), "ui.history_limit", json.RawMessage("0"))

	require.NoError(t, err)
	assert.Equal(t, host.set, got)
	require.Len(t, host.sets, 1)
	assert.Equal(t, "ui.history_limit", host.sets[0].Key)
	assert.JSONEq(t, "0", string(host.sets[0].Value))
}

// A reset is a value of nothing, and it crosses as one.
func TestHost_ResetCrossesAsNothing(t *testing.T) {
	host := &hostEnd{hello: hello}
	a := connectTo(t, host)

	_, err := a.SetHostSetting(context.Background(), "ui.history_limit", nil)

	require.NoError(t, err)
	require.Len(t, host.sets, 1)
	assert.Nil(t, host.sets[0].Value)
}

// A reload is answered with what the host found in its file.
func TestHost_ReloadCrosses(t *testing.T) {
	host := &hostEnd{hello: hello, reload: wire.ReloadResult{
		Failed:   "yaml: line 3: mapping values are not allowed",
		Warnings: []wire.Warning{{Text: "proxy.server: ignored"}},
	}}
	a := connectTo(t, host)

	got, err := a.ReloadHost(context.Background())

	require.NoError(t, err)
	assert.Equal(t, host.reload, got)
}

// A newer client asks a service to restart, and hears whether it will (#183).
func TestHost_RestartCrosses(t *testing.T) {
	a := connectTo(t, &hostEnd{hello: hello})

	got, err := a.RestartHost(context.Background())

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
}

// A host that is somebody's session refuses, and says why: that is an answer,
// not a failure of the call.
func TestHost_ARestartRefusedIsAnAnswer(t *testing.T) {
	a := connectTo(t, &hostEnd{hello: hello, restart: wire.RestartResult{Refused: "kept running by its window"}})

	got, err := a.RestartHost(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "kept running by its window", got.Refused)
}

// Uninstalling tells the service first, over a connection of its own, so that
// it stops as uninstalled when its service manager stops it (#182).
func TestAskUninstall_TheServiceIsTold(t *testing.T) {
	host := &hostEnd{hello: service("2.0.0")}
	client, end := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = serve.Conn(ctx, end, core.NewAttachment(), host)
	}()
	t.Cleanup(func() {
		cancel()
		<-served
	})

	got, err := dial.AskUninstall(ctx, client, hello)

	require.NoError(t, err)
	assert.Empty(t, got.Refused)
	host.mu.Lock()
	defer host.mu.Unlock()
	assert.True(t, host.uninstalled)
}

// askStatusOf asks host how it is doing over a connection of its own, the way
// `tele daemon status` does, as a client saying mine.
func askStatusOf(t *testing.T, host *hostEnd, mine wire.Hello) (wire.Hello, wire.HostStatus, error) {
	t.Helper()
	client, end := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = serve.Conn(ctx, end, core.NewAttachment(), host)
	}()
	t.Cleanup(func() {
		cancel()
		<-served
	})
	return dial.AskStatus(ctx, client, mine)
}

// How the host is doing crosses as it is, with what the host said of itself
// (#208).
func TestAskStatus_TheHostSaysHowItIsDoing(t *testing.T) {
	said := hello
	said.Binary, said.PID, said.Kept = "2.0.0-beta.9", 4242, wire.KeptByService
	since := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	host := &hostEnd{hello: said, status: wire.HostStatus{
		Since:   since,
		Windows: 2,
		Account: &wire.AccountStatus{
			Phase: wire.PhaseReady, User: "ada", Link: wire.LinkUp, LinkSince: since.Add(time.Minute),
			LastUpdate: since.Add(time.Hour), Outbox: 3,
		},
	}}

	gotHello, got, err := askStatusOf(t, host, hello)

	require.NoError(t, err)
	assert.Equal(t, said, gotHello)
	assert.True(t, got.Since.Equal(since))
	assert.Equal(t, 2, got.Windows)
	require.NotNil(t, got.Account)
	assert.Equal(t, wire.PhaseReady, got.Account.Phase)
	assert.Equal(t, "ada", got.Account.User)
	assert.Equal(t, wire.LinkUp, got.Account.Link)
	assert.True(t, got.Account.LastUpdate.Equal(since.Add(time.Hour)))
	assert.Equal(t, 3, got.Account.Outbox)
}

// A host on another protocol is not asked anything it might misread, but it
// is still named by what it said in hello: knowing which tele is in the way is
// the point of asking.
func TestAskStatus_AHostOnAnotherProtocolIsOnlyNamed(t *testing.T) {
	said := hello
	said.Protocol, said.Binary = hello.Protocol+1, "3.0.0"
	host := &hostEnd{hello: said, status: wire.HostStatus{Windows: 7}}

	gotHello, _, err := askStatusOf(t, host, hello)

	var other *dial.Mismatch
	require.ErrorAs(t, err, &other)
	assert.Equal(t, "3.0.0", gotHello.Binary)
}

// Asking how a service is doing never restarts it, whatever the versions:
// only a window does that.
func TestAskStatus_AnOlderServiceIsAskedNotRestarted(t *testing.T) {
	said := hello
	said.Binary, said.Kept = "2.0.0-beta.1", wire.KeptByService
	mine := hello
	mine.Binary = "2.0.0"
	host := &hostEnd{hello: said, status: wire.HostStatus{Windows: 1}}

	_, got, err := askStatusOf(t, host, mine)

	require.NoError(t, err)
	assert.Equal(t, 1, got.Windows)
}

// What was wrong with the host's settings when it started is said in its
// hello, so the first window to attach shows it.
func TestHost_HelloCarriesTheHostsWarnings(t *testing.T) {
	said := hello
	said.Warnings = []wire.Warning{{Text: "telegram.session_file is deprecated"}}
	a := connectTo(t, &hostEnd{hello: said})

	assert.Equal(t, said.Warnings, a.Host().Warnings)
}
