package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// heardNotifier keeps every desktop notification it was asked to post.
type heardNotifier struct {
	mu    sync.Mutex
	heard []string
}

func (n *heardNotifier) Notify(title, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.heard = append(n.heard, title+": "+body)
	return nil
}

func (n *heardNotifier) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.heard...)
}

// alone is the rig with its client gone and the host's desktop notifications
// kept: a service nobody has open.
func alone(t *testing.T, h testHostRig) *heardNotifier {
	t.Helper()
	require.NoError(t, h.client.Close())
	require.Eventually(t, func() bool { return h.host.clients.Count() == 0 }, 2*time.Second, 5*time.Millisecond)
	n := &heardNotifier{}
	h.host.notifier = n
	return n
}

// A log out with nobody to see it is the one way to learn the service has
// stopped delivering: it raises a desktop notification saying so, and how to
// log in again (#211).
func TestHost_ALogOutWithNoWindowIsNotified(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	n := alone(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.host.Start(ctx)

	require.Eventually(t, func() bool { return h.host.current() != first }, 5*time.Second, 5*time.Millisecond)
	require.Len(t, n.all(), 1)
	assert.Contains(t, n.all()[0], "logged out")
	assert.Contains(t, n.all()[0], "log in")
}

// A log out a window is there to show is shown by the window; the desktop is
// not told too.
func TestHost_ALogOutAWindowShowsIsNotNotified(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	n := &heardNotifier{}
	h.host.notifier = n
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.host.Start(ctx)
	waitForEnd(t, h, first)

	assert.Empty(t, n.all())
}

// A command running is not a person looking: a log out while only a command
// is attached is told to the desktop, as with nobody at all (#213).
func TestHost_ALogOutWithOnlyACommandAttachedIsNotified(t *testing.T) {
	h := testHost(t)
	first := h.host.current()
	n := alone(t, h)
	dialCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	conn, err := ipc.Dial(dialCtx, h.endpoint)
	require.NoError(t, err)
	cmd, err := dial.Dial(dialCtx, conn, commandHello)
	require.NoError(t, err)
	defer func() { _ = cmd.Close() }()
	require.Eventually(t, func() bool { return h.host.clients.Commands() == 1 }, 2*time.Second, 5*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.host.Start(ctx)

	require.Eventually(t, func() bool { return h.host.current() != first }, 5*time.Second, 5*time.Millisecond)
	require.Len(t, n.all(), 1)
	assert.Contains(t, n.all()[0], "logged out")
}

// A banned account is said as such, and no login is offered: logging in with
// the same number meets the same refusal.
func TestHost_ABannedAccountWithNoWindowIsNotified(t *testing.T) {
	h := testHostWith(t, &bannedConnection{updates: make(chan store.Event)})
	n := alone(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h.host.Start(ctx)

	require.Eventually(t, func() bool { return len(n.all()) == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.Contains(t, n.all()[0], "banned")
	assert.False(t, strings.Contains(n.all()[0], "log in"), "a banned account is offered a login: %q", n.all()[0])
}
