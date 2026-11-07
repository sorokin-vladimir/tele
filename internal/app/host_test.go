package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/updates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/appkey"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// loggedOutConnection connects only to learn the session was logged out
// elsewhere, which is how a connection ends when the account does.
type loggedOutConnection struct {
	internaltg.Client
	updates chan store.Event
}

func (c *loggedOutConnection) Connect(context.Context, *internaltg.AuthFlow,
	chan<- struct{}, func(domain.User)) error {
	return &telerr.Error{Kind: telerr.Unauthorized, LogOut: telerr.LogOutElsewhere}
}

func (c *loggedOutConnection) Updates() <-chan store.Event { return c.updates }

// bannedConnection connects only to learn the account is banned: the
// connection ends, and the account stays until the person leaves it.
type bannedConnection struct {
	internaltg.Client
	updates chan store.Event
}

func (c *bannedConnection) Connect(context.Context, *internaltg.AuthFlow,
	chan<- struct{}, func(domain.User)) error {
	return &telerr.Error{Kind: telerr.AccountBanned}
}

func (c *bannedConnection) Updates() <-chan store.Event { return c.updates }

// testHost is a host over a fresh state directory whose first account is
// logged out as soon as it connects, and whose next one stays connected, with
// a client attached to it over a socket.
func testHost(t *testing.T) testHostRig {
	t.Helper()
	return testHostWith(t, &loggedOutConnection{updates: make(chan store.Event)})
}

// testHostWith is testHost with first as the first account's connection.
func testHostWith(t *testing.T, first core.Connection) testHostRig {
	t.Helper()
	return newTestHost(t, first, appkey.Key{}, "ui:\n  history_limit: 30\n")
}

// testHostRig is a host over a real config file and a fresh state directory,
// with what its accounts were handed, and a client attached to it the way
// every client is: over the endpoint (#208).
type testHostRig struct {
	host   *Host
	client *dial.Attachment
	// endpoint is where the host serves, for a test attaching more clients.
	endpoint ipc.Endpoint
	// endpoints receives what each account's connection was made with.
	endpoints chan internaltg.Endpoint
	// path is the config file, and stateDir the platform state directory.
	path     string
	stateDir string
}

// newTestHost builds a host the way New does, over a config file that says
// body, with key as the app key the process resolved. Its first account's
// connection is first; every later one stays connected.
func newTestHost(t *testing.T, first core.Connection, key appkey.Key, body string) testHostRig {
	t.Helper()
	stateDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	cfg, err := config.Load(path, stateDir)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(cfg.StateDir, 0o700))
	require.NoError(t, os.WriteFile(cfg.Telegram.SessionFile, []byte(`{}`), 0o600))
	start, err := newStartup(cfg, key)
	require.NoError(t, err)

	h := testHostRig{endpoints: make(chan internaltg.Endpoint, 8), path: path, stateDir: stateDir}
	var mu sync.Mutex
	opened := 0
	h.host = &Host{
		cfgStore: config.NewStoreOf(cfg, path, stateDir),
		startup:  start,
		log:      zap.NewNop(),
		notifier: newNotifier(zap.NewNop()),
		clients:  hosting.NewClients(),
		connect: func(endpoint internaltg.Endpoint, _ updates.StateStorage) core.Connection {
			h.endpoints <- endpoint
			mu.Lock()
			defer mu.Unlock()
			opened++
			if opened == 1 {
				return first
			}
			return &heldConnection{updates: make(chan store.Event)}
		},
	}
	_, err = h.host.openNext()
	require.NoError(t, err)
	// Runs last, before the temp directories are removed: an account's
	// database still open then cannot be removed on Windows.
	t.Cleanup(h.host.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reached, err := hosting.Reach(ctx, "tele-test", t.TempDir())
	require.NoError(t, err)
	t.Cleanup(reached.Close)
	end := NewHostEnd(h.host.cfgStore, testHello)
	end.ApplyTo(h.host.Apply)
	srv := hosting.Serve(context.Background(), reached.Listener, end, h.host.clients)
	t.Cleanup(func() { srv.Stop(time.Second, core.StopQuit) })
	conn, err := ipc.Dial(ctx, reached.Endpoint)
	require.NoError(t, err)
	h.client, err = dial.Dial(ctx, conn, windowHello)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.client.Close() })
	h.endpoint = reached.Endpoint
	return h
}

var testHello = wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev"}

// windowHello and commandHello are what a window and a command say.
var (
	windowHello  = wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kind: wire.KindWindow}
	commandHello = wire.Hello{Protocol: wire.ProtocolVersion, Binary: "dev", Kind: wire.KindCommand}
)

// waitForEnd waits for the client to hear that an account ended, and then for
// the next one to be open in its place. Whatever the host could not do on the
// way - remove the account, open the next - fails the test with what it said,
// rather than with a timeout that says nothing.
func waitForEnd(t *testing.T, h testHostRig, ended *account) core.AccountEnded {
	t.Helper()
	deadline := time.After(5 * time.Second)
	var news core.AccountEnded
	for heard := false; !heard; {
		select {
		case msg := <-h.client.Stream():
			if failed, ok := msg.(core.HostFailure); ok {
				require.FailNow(t, "the host failed before the account ended", "%v", failed.Err)
			}
			news, heard = msg.(core.AccountEnded)
		case <-deadline:
			require.FailNow(t, "the account never ended")
		}
	}
	for h.host.current() == ended {
		select {
		case msg := <-h.client.Stream():
			if failed, ok := msg.(core.HostFailure); ok {
				require.FailNow(t, "the next account was not opened", "%v", failed.Err)
			}
		case <-deadline:
			require.FailNow(t, "the next account never opened")
		case <-time.After(5 * time.Millisecond):
		}
	}
	return news
}

// A log out ends the account in the running process: the next account starts
// in its place, with nothing of the last one on disk - not its history, not its
// session - and the client hears why (#297).
func TestHost_ALogOutStartsTheNextAccountWithNothingOfTheLast(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	_, err := first.store.DB().Exec(`INSERT OR REPLACE INTO metadata (key, value) VALUES ('mark', 'first')`)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.launch(ctx, first)
	ended := waitForEnd(t, h, first)

	assert.Equal(t, core.AccountEnded{Reason: core.EndLoggedOutElsewhere}, ended)
	assert.NoFileExists(t, h.host.startup.sessionFile, "a login would bind to the old key")

	next := h.host.current()
	var v string
	err = next.store.DB().QueryRow(`SELECT value FROM metadata WHERE key = 'mark'`).Scan(&v)
	assert.Error(t, err, "the next account found the last one's data")
}

// The client stays attached across the end of an account: the same attachment
// is seated at the next owner, and what the client asks reaches that one
// (#309).
func TestHost_TheClientIsSeatedAtTheNextAccount(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.launch(ctx, first)
	waitForEnd(t, h, first)

	h.client.Subscribe(project.AccountWindow{})

	select {
	case msg := <-h.client.Stream():
		d, ok := msg.(project.Delta)
		require.True(t, ok, "the next owner answers the subscription, got %T", msg)
		require.NotNil(t, d.Account)
	case <-time.After(3 * time.Second):
		require.Fail(t, "the next owner never answered")
	}
}

// A banned account's connection is over before the person decides anything.
// Leaving it to log in with another number is the client's request, made over
// its connection, and ends the account like any log out (#297).
func TestHost_LeavingABannedAccountStartsTheNextOne(t *testing.T) {
	h := testHostWith(t, &bannedConnection{updates: make(chan store.Event)})
	first := h.host.current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.Start(ctx)
	require.Eventually(t, func() bool { return first.owner.Account().Ended != nil }, 2*time.Second, 5*time.Millisecond)

	h.client.EndAccount(core.EndBanned)
	ended := waitForEnd(t, h, first)

	assert.Equal(t, core.EndBanned, ended.Reason)
}

// Only the account running now can end. Asked twice - the connection and the
// client both, or a request arriving late - the second must not take the next
// account with it.
func TestHost_AnAccountEndsOnce(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.launch(ctx, first)
	waitForEnd(t, h, first)
	next := h.host.current()

	h.host.endAccount(ctx, first, core.EndBanned)

	assert.Same(t, next, h.host.current(), "the next account is still the one running")
	_, err := next.store.DB().Exec(`SELECT 1`)
	assert.NoError(t, err, "and it was not stopped")
}

// The app key a build carries is the process's, not the file's: an account
// that starts after a log out connects with it even when the config was
// reloaded in between from a file that names no key (#239).
func TestHost_TheNextAccountConnectsWithTheBuiltInKeyAfterAReload(t *testing.T) {
	builtIn := appkey.Key{ID: 4242, Hash: "built-in"}
	h := newTestHost(t, &loggedOutConnection{updates: make(chan store.Event)}, builtIn, "ui:\n  history_limit: 30\n")
	<-h.endpoints // the first account's
	_, err := hostReload(h.client)()
	require.NoError(t, err)

	first := h.host.current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.launch(ctx, first)
	waitForEnd(t, h, first)

	select {
	case endpoint := <-h.endpoints:
		assert.Equal(t, builtIn, endpoint.Key, "the next account connected without the key the build carries")
	case <-time.After(3 * time.Second):
		require.Fail(t, "the next account never connected")
	}
}
