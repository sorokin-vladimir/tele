package hosting_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

var hello = wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"}

// helloHost is a host that says hello and holds no settings.
type helloHost struct{}

func (helloHost) Hello() wire.Hello                        { return hello }
func (helloHost) Settings() wire.HostSettings              { return wire.HostSettings{} }
func (helloHost) SetSetting(wire.SetParams) wire.SetResult { return wire.SetResult{} }
func (helloHost) Reload() wire.ReloadResult                { return wire.ReloadResult{} }
func (helloHost) Status() wire.HostStatus                  { return wire.HostStatus{} }
func (helloHost) Restart() (wire.RestartResult, func())    { return wire.RestartResult{}, nil }
func (helloHost) Uninstall() wire.UninstallResult          { return wire.UninstallResult{} }
func (helloHost) AccountCommands() wire.CommandAccess      { return wire.CommandsAll }

// noConn is a connection to Telegram nothing in these tests reaches.
type noConn struct{ internaltg.Client }

func (noConn) Connect(context.Context, *internaltg.AuthFlow, chan<- struct{}, func(domain.User)) error {
	return nil
}

func (noConn) Updates() <-chan store.Event { return nil }

type nopNotifier struct{}

func (nopNotifier) Notify(string, string) error { return nil }

// newOwner is an owner that is not connected to anything: enough to answer a
// subscription to its account.
func newOwner(t *testing.T) *core.Owner {
	t.Helper()
	o := core.New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), noConn{}, nopNotifier{})
	t.Cleanup(o.Stop)
	return o
}

// host serves a fresh endpoint for clients, and stops when the test ends.
func host(t *testing.T, clients *hosting.Clients) (ipc.Endpoint, *hosting.Server) {
	t.Helper()
	r := reach(t, stateDir(t))
	require.NotNil(t, r.Listener)
	s := hosting.Serve(context.Background(), r.Listener, helloHost{}, clients)
	t.Cleanup(func() { s.Stop(time.Second, core.StopQuit) })
	return r.Endpoint, s
}

// client attaches to the host at ep over the socket.
func client(t *testing.T, ep ipc.Endpoint) *dial.Attachment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := ipc.Dial(ctx, ep)
	require.NoError(t, err)
	a, err := dial.Dial(ctx, conn, hello)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// next is the next thing a hears, failing the test if nothing comes.
func next(t *testing.T, a *dial.Attachment) any {
	t.Helper()
	select {
	case v, ok := <-a.Stream():
		require.True(t, ok, "the connection ended")
		return v
	case <-time.After(2 * time.Second):
		require.FailNow(t, "nothing arrived")
		return nil
	}
}

// A client that connects is seated at the owner of the moment: what it
// subscribes to is answered by that owner.
func TestServe_AClientIsSeatedAtTheOwner(t *testing.T) {
	clients := hosting.NewClients()
	clients.Join(newOwner(t))
	ep, _ := host(t, clients)
	a := client(t, ep)

	a.Subscribe(project.AccountWindow{})

	d, ok := next(t, a).(project.Delta)
	require.True(t, ok)
	assert.NotNil(t, d.Account, "the owner answered the subscription")
}

// When an account ends, every client is told, and every client is seated at
// the next owner: the subscriptions each makes then are the next owner's.
func TestServe_EveryClientMovesToTheNextOwner(t *testing.T) {
	clients := hosting.NewClients()
	clients.Join(newOwner(t))
	ep, _ := host(t, clients)
	a, b := client(t, ep), client(t, ep)
	require.Eventually(t, func() bool { return clients.Count() == 2 }, 2*time.Second, 10*time.Millisecond)

	clients.Leave(core.AccountEnded{Reason: core.EndLoggedOutHere})
	clients.Join(newOwner(t))

	for _, c := range []*dial.Attachment{a, b} {
		assert.Equal(t, core.AccountEnded{Reason: core.EndLoggedOutHere}, next(t, c))
		c.Subscribe(project.AccountWindow{})
		d, ok := next(t, c).(project.Delta)
		require.True(t, ok)
		assert.NotNil(t, d.Account, "the next owner answered")
	}
}

// A host that stops takes its endpoint away first, so nobody new connects, and
// tells every client still there that it is stopping, which is the last thing
// each hears (#208).
func TestServe_AStoppingHostSaysSoAndGoes(t *testing.T) {
	clients := hosting.NewClients()
	clients.Join(newOwner(t))
	ep, s := host(t, clients)
	a := client(t, ep)
	require.Eventually(t, func() bool { return clients.Count() == 1 }, 2*time.Second, 10*time.Millisecond)

	s.Stop(time.Second, core.StopQuit)

	assert.Equal(t, core.HostStopping{Reason: core.StopQuit}, next(t, a))
	select {
	case _, open := <-a.Stream():
		assert.False(t, open, "the connection ends on it")
	case <-time.After(2 * time.Second):
		t.Fatal("the connection outlived the host")
	}
	_, err := os.Lstat(ep.Path)
	assert.ErrorIs(t, err, os.ErrNotExist, "the socket is gone")
	assert.Zero(t, clients.Count())
}

// A client that never takes what it is told - here one that never even said
// hello - does not keep the host from stopping past the grace it is given.
func TestServe_AStalledClientDoesNotHoldTheStop(t *testing.T) {
	clients := hosting.NewClients()
	ep, s := host(t, clients)
	conn, err := ipc.Dial(context.Background(), ep)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.Eventually(t, func() bool { return clients.Count() == 1 }, 2*time.Second, 10*time.Millisecond)

	stopped := make(chan struct{})
	start := time.Now()
	go func() {
		s.Stop(200*time.Millisecond, core.StopQuit)
		close(stopped)
	}()

	select {
	case <-stopped:
		assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond, "the client was given its grace")
	case <-time.After(2 * time.Second):
		t.Fatal("the stalled client held the stop")
	}
}

// A client asking for the account to end is heard by the host, whichever
// client it is.
func TestServe_AClientAskingForTheEndIsHeard(t *testing.T) {
	clients := hosting.NewClients()
	clients.Join(newOwner(t))
	ep, _ := host(t, clients)
	_ = client(t, ep)
	b := client(t, ep)

	b.EndAccount(core.EndBanned)

	select {
	case reason := <-clients.EndRequests():
		assert.Equal(t, core.EndBanned, reason)
	case <-time.After(2 * time.Second):
		t.Fatal("the host never heard it")
	}
}

// clientAs attaches to the host at ep as a client of kind.
func clientAs(t *testing.T, ep ipc.Endpoint, kind wire.ClientKind) *dial.Attachment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := ipc.Dial(ctx, ep)
	require.NoError(t, err)
	h := hello
	h.Kind = kind
	a, err := dial.Dial(ctx, conn, h)
	require.NoError(t, err)
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// A host counts its windows apart from its commands, by what each says it is
// in its hello: a command running is not a person looking (#213).
func TestServe_WindowsAndCommandsAreCountedApart(t *testing.T) {
	clients := hosting.NewClients()
	ep, _ := host(t, clients)

	clientAs(t, ep, wire.KindWindow)
	clientAs(t, ep, wire.KindWindow)
	cmd := clientAs(t, ep, wire.KindCommand)

	require.Eventually(t, func() bool { return clients.Windows() == 2 && clients.Commands() == 1 },
		2*time.Second, 10*time.Millisecond, "windows %d, commands %d", clients.Windows(), clients.Commands())

	_ = cmd.Close()
	require.Eventually(t, func() bool { return clients.Commands() == 0 }, 2*time.Second, 10*time.Millisecond,
		"a command that went is not counted")
	assert.Equal(t, 2, clients.Windows())
}
