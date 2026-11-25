package dial

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// heldConn holds the first write until released, and says when it began, so a
// test can change what a call will find before the call goes on to wait.
type heldConn struct {
	once    sync.Once
	wrote   chan struct{}
	release chan struct{}
}

func (c *heldConn) Write(p []byte) (int, error) {
	first := false
	c.once.Do(func() { first = true })
	if first {
		close(c.wrote)
		<-c.release
	}
	return len(p), nil
}

func (c *heldConn) Read([]byte) (int, error) { select {} }
func (c *heldConn) Close() error             { return nil }

// A host that answers and then goes, as a service asked to restart does,
// leaves the answer waiting and the connection ended at the same time. The
// answer is what the call reports: it came, so the call succeeded, whatever
// happened to the connection after it.
func TestRoundTrip_AnAnswerThatCameBeforeTheConnectionEndedIsReported(t *testing.T) {
	for range 100 {
		conn := &heldConn{wrote: make(chan struct{}), release: make(chan struct{})}
		l := newLink(conn)
		a := &Attachment{waiting: map[uint64]chan wire.Frame{}, link: l}

		type outcome struct {
			result json.RawMessage
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := a.roundTrip(context.Background(), wire.MethodRestart, json.RawMessage(`null`))
			done <- outcome{result, err}
		}()

		<-conn.wrote
		// What the reader does: the answer goes to the call waiting for it,
		// and only then is the connection found ended.
		a.mu.Lock()
		for id, ch := range a.waiting {
			delete(a.waiting, id)
			ch <- wire.Frame{ID: id, Result: json.RawMessage(`{}`)}
		}
		l.why = hostLost()
		close(l.lost)
		a.mu.Unlock()
		close(conn.release)

		got := <-done
		require.NoError(t, got.err, "the answer had come before the connection ended")
		require.JSONEq(t, `{}`, string(got.result))
	}
}
