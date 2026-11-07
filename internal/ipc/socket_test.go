package ipc_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ipc"
	"github.com/sorokin-vladimir/tele/internal/ipc/ipctest"
)

// endpoint is the real endpoint of a fresh state directory, on whatever
// platform the test runs: on Windows this is the test that says a unix socket
// works there at all.
func endpoint(t *testing.T) ipc.Endpoint {
	t.Helper()
	dir := t.TempDir()
	ep, err := ipc.EndpointFor("tele-test", dir)
	require.NoError(t, err)
	ipctest.ForgetEndpoint(t, "tele-test", dir, ep.Path)
	return ep
}

func listen(t *testing.T, ep ipc.Endpoint) net.Listener {
	t.Helper()
	l, err := ipc.Listen(ep)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func dial(t *testing.T, ep ipc.Endpoint) (net.Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return ipc.Dial(ctx, ep)
}

// Bytes cross both ways between a host and a client.
func TestSocket_BytesCrossBothWays(t *testing.T) {
	ep := endpoint(t)
	l := listen(t, ep)
	served := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err == nil {
			served <- c
		}
	}()

	client, err := dial(t, ep)
	require.NoError(t, err)
	defer func() { _ = client.Close() }()
	host := <-served
	defer func() { _ = host.Close() }()

	_, err = client.Write([]byte("hello\n"))
	require.NoError(t, err)
	got := make([]byte, 6)
	_, err = io.ReadFull(host, got)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(got))

	_, err = host.Write([]byte("back\n"))
	require.NoError(t, err)
	got = make([]byte, 5)
	_, err = io.ReadFull(client, got)
	require.NoError(t, err)
	assert.Equal(t, "back\n", string(got))
}

// Nobody there is the one answer that lets a client become the host.
func TestSocket_NobodyThereIsNoHost(t *testing.T) {
	_, err := dial(t, endpoint(t))

	assert.ErrorIs(t, err, ipc.ErrNoHost)
}

// A state directory too deep for a socket path gets its socket in a directory
// of its own, which only a host makes. Before one has, nobody is there: on
// Windows, dialling into a directory that does not exist reads as a dead
// network rather than as a missing file, and a client must not take that for
// a host that refused it.
func TestSocket_NobodyThereBeforeTheSocketsDirectoryExists(t *testing.T) {
	// Without a runtime directory, so that the socket goes to a directory of
	// this state directory's own rather than to the one every build shares.
	t.Setenv("XDG_RUNTIME_DIR", "")
	deep := filepath.Join(t.TempDir(), strings.Repeat("d", 120))
	require.NoError(t, os.MkdirAll(deep, 0o700))
	ep, err := ipc.EndpointFor("tele-test", deep)
	require.NoError(t, err)
	ipctest.ForgetEndpoint(t, "tele-test", deep, ep.Path)
	_, err = os.Stat(filepath.Dir(ep.Path))
	require.ErrorIs(t, err, os.ErrNotExist, "the socket's own directory is not made yet")

	_, err = dial(t, ep)

	assert.ErrorIs(t, err, ipc.ErrNoHost)
}

// A socket a dead host left behind is nobody there too, and it is still in the
// way of a bind until the hosting code removes it.
func TestSocket_AStaleSocketIsNoHostAndStaysInTheWay(t *testing.T) {
	ep := endpoint(t)
	l, err := ipc.Listen(ep)
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	require.NoError(t, l.Close())

	_, err = dial(t, ep)
	assert.ErrorIs(t, err, ipc.ErrNoHost)

	_, err = ipc.Listen(ep)
	assert.ErrorIs(t, err, ipc.ErrInUse)

	require.NoError(t, ipc.RemoveStale(ep))
	listen(t, ep)
}

// Listen never takes the endpoint from a live host: binding over a socket file
// would leave that host serving nobody.
func TestSocket_ListenLeavesALiveHostAlone(t *testing.T) {
	ep := endpoint(t)
	l := listen(t, ep)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	_, err := ipc.Listen(ep)
	require.ErrorIs(t, err, ipc.ErrInUse)

	conn, err := dial(t, ep)
	require.NoError(t, err, "the live host still answers")
	_ = conn.Close()
}

// A host that closes takes its socket with it, so the next one finds nothing
// in the way.
func TestSocket_ClosingRemovesTheSocket(t *testing.T) {
	ep := endpoint(t)
	l, err := ipc.Listen(ep)
	require.NoError(t, err)

	require.NoError(t, l.Close())

	_, err = os.Lstat(ep.Path)
	assert.True(t, os.IsNotExist(err))
}

// Removing a socket that is not there is nothing to report.
func TestSocket_RemovingNothingIsFine(t *testing.T) {
	assert.NoError(t, ipc.RemoveStale(endpoint(t)))
}

// The socket is the user's alone: phone numbers, codes and passwords cross it.
func TestSocket_IsTheUsersAlone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a socket has no mode bits on Windows; the state directory's ACL protects it")
	}
	ep := endpoint(t)
	listen(t, ep)

	info, err := os.Stat(ep.Path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// Two state directories are two hosts, and each client reaches its own.
func TestSocket_EachClientReachesItsOwnHost(t *testing.T) {
	a, b := endpoint(t), endpoint(t)
	require.NotEqual(t, a.Path, b.Path)
	la, lb := listen(t, a), listen(t, b)
	answer := func(l net.Listener, name string) {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte(name))
			_ = c.Close()
		}
	}
	go answer(la, "a")
	go answer(lb, "b")

	for ep, want := range map[ipc.Endpoint]string{a: "a", b: "b"} {
		conn, err := dial(t, ep)
		require.NoError(t, err)
		got, err := io.ReadAll(conn)
		require.NoError(t, err)
		_ = conn.Close()
		assert.Equal(t, want, string(got))
	}
}
