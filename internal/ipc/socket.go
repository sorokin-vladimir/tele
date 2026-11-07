package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
)

// ErrInUse is a socket file already at the endpoint. It may be a live host's
// or one a host left behind when it died; telling which is the hosting code's,
// under the state directory lock, and never Listen's.
var ErrInUse = errors.New("ipc: the endpoint is in use")

// ErrNoHost is nobody listening at the endpoint: no socket file, or one that
// nothing answers on. It is the answer that lets a client become the host.
var ErrNoHost = errors.New("ipc: no host at the endpoint")

// Listen serves ep. It never removes a file already there: a stale socket does
// not stop a bind, so binding over whatever is there could take the endpoint
// from a live host. The socket is the user's alone, and goes when the listener
// is closed.
func Listen(ep Endpoint) (*Listener, error) {
	if ep.own != "" {
		if err := os.MkdirAll(ep.own, 0o700); err != nil {
			return nil, fmt.Errorf("the socket directory: %w", err)
		}
	}
	if _, err := os.Lstat(ep.Path); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrInUse, ep.Path)
	}
	ul, err := net.ListenUnix("unix", &net.UnixAddr{Name: ep.Path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("serving %s: %w", ep.Path, err)
	}
	l := &Listener{UnixListener: ul}
	// Windows has no mode bits on a socket: the state directory's ACL, which
	// is the user's, protects it there.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(ep.Path, 0o600); err != nil {
			_ = l.Close()
			return nil, fmt.Errorf("making %s private: %w", ep.Path, err)
		}
	}
	return l, nil
}

// RemoveStale removes a socket a host left behind. Only the hosting code calls
// it, holding the state directory lock, after finding nobody answers there.
func RemoveStale(ep Endpoint) error {
	if err := os.Remove(ep.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Dial reaches the host at ep. Nobody listening is ErrNoHost; anything else -
// a host that is there and refuses, a socket that is not the user's - is
// returned as it is, since it is no invitation to become the host.
func Dial(ctx context.Context, ep Endpoint) (net.Conn, error) {
	// No socket file is nobody there, said before the dial rather than read
	// from what it fails with: on Windows, dialling into a directory that does
	// not exist yet - the socket's own, made only by a host - fails as a dead
	// network, which says nothing of a host.
	if _, err := os.Lstat(ep.Path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoHost, ep.Path)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", ep.Path)
	if err == nil {
		return wake(conn), nil
	}
	if noHost(err) {
		return nil, fmt.Errorf("%w: %s", ErrNoHost, ep.Path)
	}
	return nil, err
}
