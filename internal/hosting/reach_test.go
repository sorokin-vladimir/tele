package hosting_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/ipctest"
	"github.com/sorokin-vladimir/tele/internal/statedir"
)

const app = "tele-test"

// stateDir is a fresh state directory, with whatever socket its endpoint
// leaves outside it removed at the end.
func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ep, err := ipc.EndpointFor(app, dir)
	require.NoError(t, err)
	ipctest.ForgetEndpoint(t, app, dir, ep.Path)
	return dir
}

func reach(t *testing.T, dir string) hosting.Reached {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := hosting.Reach(ctx, app, dir)
	require.NoError(t, err)
	t.Cleanup(r.Close)
	return r
}

// With nobody at the endpoint, the one reaching it becomes the host: it holds
// the state directory and serves the endpoint.
func TestReach_WithNobodyThereBecomesTheHost(t *testing.T) {
	r := reach(t, stateDir(t))

	assert.NotNil(t, r.Listener, "it serves the endpoint")
	assert.Nil(t, r.Conn)
}

// With a host there, the one reaching it attaches to it.
func TestReach_WithAHostThereAttaches(t *testing.T) {
	dir := stateDir(t)
	host := reach(t, dir)
	require.NotNil(t, host.Listener)

	r := reach(t, dir)

	assert.NotNil(t, r.Conn, "it is connected to the host")
	assert.Nil(t, r.Listener)
}

// A process that holds the state directory and serves nothing - an older build,
// or one that never got as far as serving - is waited for, and then named
// rather than waited on for ever.
func TestReach_ALockHeldWithNothingServedIsNamed(t *testing.T) {
	hosting.WaitForHostFor(t, 200*time.Millisecond)
	dir := stateDir(t)
	held, err := statedir.Acquire(dir)
	require.NoError(t, err)
	defer func() { _ = held.Release() }()

	_, err = hosting.Reach(context.Background(), app, dir)

	require.ErrorIs(t, err, hosting.ErrHeldNotServing)
	assert.Contains(t, err.Error(), fmt.Sprintf("pid %d", os.Getpid()))
}

// A host that is still starting - it holds the lock and serves a moment later -
// is attached to once it does.
func TestReach_AHostStillStartingIsWaitedFor(t *testing.T) {
	dir := stateDir(t)
	held, err := statedir.Acquire(dir)
	require.NoError(t, err)
	ep, err := ipc.EndpointFor(app, dir)
	require.NoError(t, err)
	time.AfterFunc(300*time.Millisecond, func() {
		l, err := ipc.Listen(ep)
		if err == nil {
			t.Cleanup(func() { _ = l.Close(); _ = held.Release() })
		}
	})

	r := reach(t, dir)

	assert.NotNil(t, r.Conn)
}

// A socket a host left behind when it died is no host: whoever reaches the
// endpoint next removes it and serves the endpoint itself.
func TestReach_ASocketLeftBehindIsNoHost(t *testing.T) {
	dir := stateDir(t)
	ep, err := ipc.EndpointFor(app, dir)
	require.NoError(t, err)
	dead, err := ipc.Listen(ep)
	require.NoError(t, err)
	dead.SetUnlinkOnClose(false)
	require.NoError(t, dead.Close())
	_, err = os.Lstat(ep.Path)
	require.NoError(t, err, "the dead host's socket is still there")

	r := reach(t, dir)

	assert.NotNil(t, r.Listener)
}
