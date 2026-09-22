package tg

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/log/logzap"
	"github.com/gotd/td/clock"
	"github.com/gotd/td/session"
	"github.com/gotd/td/tdsync"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tgtest/cluster"
	"github.com/gotd/td/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// skewedClock is the system clock moved by ahead.
type skewedClock struct {
	clock.Clock
	ahead time.Duration
}

func (c skewedClock) Now() time.Time { return c.Clock.Now().Add(c.ahead) }

// Tripwire for the clock-skew watch (#277, gotd/td#1856). A real gotd client
// whose clock is ten minutes ahead talks to an in-process Telegram. Two things
// are asserted, and a gotd release that changes either fails this test:
//
//   - The defect: the client never gets past connecting, because gotd drops
//     every message the server sends. If gotd learns to correct for the
//     difference, the run reaches its function and the watch can be retired.
//   - The evidence: gotd still writes the rejected-message entry the watch reads,
//     in words the watch understands, so the skew comes out right. If gotd
//     rewords it, the watch goes blind silently; this is where that shows.
func TestStand_WithoutCalibrationASkewedClockHangs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	c := cluster.NewCluster(cluster.Options{Protocol: transport.Intermediate})
	c.Dispatch(2, "server")
	g := tdsync.NewCancellableGroup(ctx)
	g.Go(c.Up)
	select {
	case <-c.Ready():
	case <-ctx.Done():
		t.Fatal("cluster did not come up")
	}

	const ahead = 10 * time.Minute
	clk := skewedClock{Clock: clock.System, ahead: ahead}
	r := &skewReports{}
	skew := newClockSkew()
	// The watch measures against the clock gotd checks with. In the app both are
	// the system clock; here only gotd's is moved, so the watch is given it too.
	skew.now = clk.Now
	skew.SetReport(r.report)
	client := telegram.NewClient(1, "hash", telegram.Options{
		PublicKeys:     c.Keys(),
		Resolver:       c.Resolver(),
		DCList:         c.List(),
		SessionStorage: &session.StorageMemory{},
		NoUpdates:      true,
		Clock:          clk,
		Logger:         logzap.New(watchClockSkew(zap.NewNop(), skew)),
	})

	runCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	var reached atomic.Bool
	err := client.Run(runCtx, func(context.Context) error {
		reached.Store(true)
		return nil
	})

	assert.False(t, reached.Load(), "gotd connected with a skewed clock: it may correct for it now, see docs/gotd-workarounds.md")
	assert.Error(t, err)
	got := r.all()
	require.NotEmpty(t, got, "gotd dropped messages without the entry the watch reads: its wording changed")
	assert.InDelta(t, float64(ahead), float64(got[0]), float64(5*time.Second))

	cancel()
	_ = g.Wait()
}
