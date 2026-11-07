package ipc

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// stuckConn behaves as a Windows AF_UNIX connection whose peer has closed it
// while a read was waiting: that read never wakes, though a read started
// afterwards finds the end at once. A read waits only for its deadline.
type stuckConn struct {
	net.Conn
	mu       sync.Mutex
	deadline time.Time
	reads    int
	peerGone bool
}

func (c *stuckConn) Read([]byte) (int, error) {
	c.mu.Lock()
	c.reads++
	first, gone, deadline := c.reads == 1, c.peerGone, c.deadline
	c.mu.Unlock()
	if gone && !first {
		return 0, io.EOF
	}
	if deadline.IsZero() {
		select {} // the read Windows never wakes
	}
	time.Sleep(time.Until(deadline))
	return 0, os.ErrDeadlineExceeded
}

func (c *stuckConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadline = t
	return nil
}

func readWithin(t *testing.T, c net.Conn, within time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := c.Read(make([]byte, 8))
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("the read did not return within %v", within)
		return nil
	}
}

// A read waiting when the peer closed is started again, and the new read
// finds the end.
func TestWake_AReadThePeersCloseDidNotWakeFindsTheEnd(t *testing.T) {
	c := newWakeful(&stuckConn{peerGone: true}, 10*time.Millisecond)

	err := readWithin(t, c, 2*time.Second)

	assert.ErrorIs(t, err, io.EOF)
}

// A deadline the caller sets still ends the read, with the timeout it asked
// for, instead of the read being started again past it.
func TestWake_TheCallersDeadlineStillEndsTheRead(t *testing.T) {
	c := newWakeful(&stuckConn{}, 10*time.Millisecond)
	assert.NoError(t, c.SetReadDeadline(time.Now().Add(50*time.Millisecond)))

	err := readWithin(t, c, 2*time.Second)

	assert.True(t, errors.Is(err, os.ErrDeadlineExceeded), "err = %v", err)
}
