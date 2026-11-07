package ipc

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"
)

// Listener serves an endpoint. It is the socket's own listener but for what it
// accepts, which reads the way every connection of this package does.
type Listener struct {
	*net.UnixListener
}

// Accept waits for the next client.
func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.UnixListener.Accept()
	if err != nil {
		return nil, err
	}
	return wake(c), nil
}

// wake makes c's reads see its peer go on every platform. On Windows a read
// already waiting on an AF_UNIX socket when the peer closes it can wait
// forever, although a read started afterwards finds the end at once; a client
// would never learn its host went, nor a host that a window closed. There a
// read waits wakeEvery at most and is then started again.
func wake(c net.Conn) net.Conn {
	if wakeEvery == 0 {
		return c
	}
	return newWakeful(c, wakeEvery)
}

func newWakeful(c net.Conn, every time.Duration) net.Conn {
	return &wakeful{Conn: c, every: every}
}

type wakeful struct {
	net.Conn
	every time.Duration

	mu       sync.Mutex
	deadline time.Time // the caller's own read deadline, zero for none
}

func (c *wakeful) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		asked := c.deadline
		c.mu.Unlock()
		until := time.Now().Add(c.every)
		if !asked.IsZero() && asked.Before(until) {
			until = asked
		}
		if err := c.Conn.SetReadDeadline(until); err != nil {
			return 0, err
		}
		n, err := c.Conn.Read(p)
		if n > 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
			return n, err
		}
		if !asked.IsZero() && !time.Now().Before(asked) {
			return n, err
		}
	}
}

func (c *wakeful) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	return nil
}

func (c *wakeful) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}
