package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/telegram/updates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/proxy"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// heldConnection stands in for Telegram: it connects and stays connected until
// it is told to stop, which is all a test of the account's lifetime needs. The
// embedded interface is nil, so anything else the account reaches for panics.
type heldConnection struct {
	internaltg.Client
	updates chan store.Event
	ended   atomic.Bool
}

func (c *heldConnection) Connect(ctx context.Context, _ *internaltg.AuthFlow,
	_ chan<- struct{}, _ func(domain.User)) error {
	<-ctx.Done()
	c.ended.Store(true)
	return ctx.Err()
}

func (c *heldConnection) Updates() <-chan store.Event { return c.updates }

func testAccount(t *testing.T) (*account, *heldConnection) {
	t.Helper()
	conn := &heldConnection{updates: make(chan store.Event)}
	acct, err := openAccount(accountDeps{
		cfg:      &config.Config{},
		startup:  startup{stateDir: t.TempDir(), route: proxy.Route{Type: proxy.TypeDirect}},
		log:      zap.NewNop(),
		tmpDir:   t.TempDir(),
		notifier: newNotifier(zap.NewNop()),
		connect:  func(internaltg.Endpoint, updates.StateStorage) core.Connection { return conn },
	})
	require.NoError(t, err)
	return acct, conn
}

// An account is stopped whole: its connection ends, its loops return and its
// database closes, so the host can remove its files and start the next one
// with nothing of this one still running (#297).
func TestAccount_StopEndsItsConnectionAndClosesItsDatabase(t *testing.T) {
	acct, conn := testAccount(t)
	ended := make(chan struct{}, 1)
	acct.run(context.Background(), func(error) { ended <- struct{}{} })

	done := make(chan struct{})
	go func() { acct.stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "stop did not return")
	}

	assert.True(t, conn.ended.Load(), "the connection was still running")
	select {
	case <-ended:
	default:
		assert.Fail(t, "the end of the connection was not reported")
	}
	assert.Error(t, acct.store.DB().Ping(), "the database is still open")
}

// The work the host runs for an account - the bridges to the client, the
// dialog load after login - is the account's too: stop waits for it, since it
// writes to the database stop is about to close (#297).
func TestAccount_StopWaitsForTheWorkRunAlongside(t *testing.T) {
	acct, _ := testAccount(t)
	var finished atomic.Bool
	acct.run(context.Background(), nil, func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond)
		finished.Store(true)
	})

	acct.stop()

	assert.True(t, finished.Load())
}

// The process ending stops the account the same way: the host's context is
// the parent of the account's.
func TestAccount_EndsWithTheHost(t *testing.T) {
	acct, conn := testAccount(t)
	ctx, cancel := context.WithCancel(context.Background())
	ended := make(chan struct{}, 1)
	acct.run(ctx, func(error) { ended <- struct{}{} })

	cancel()

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		require.Fail(t, "the connection outlived the host")
	}
	assert.True(t, conn.ended.Load())
	acct.stop()
}
