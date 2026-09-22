package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// Clock skew is a state rather than a stream: a client that fell behind wants
// the latest value, not every one it missed, and the report comes from gotd's
// connection, which must never wait for a client (#277).
func TestOwner_ClockSkewKeepsOnlyTheLatest(t *testing.T) {
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), &stubConn{}, nopNotifier{})

	o.SetClockSkew(8 * time.Minute)
	o.SetClockSkew(9 * time.Minute)
	o.SetClockSkew(0)

	select {
	case got := <-o.ClockSkew():
		assert.Equal(t, ClockSkew{}, got)
	default:
		require.FailNow(t, "nothing published")
	}
	select {
	case got := <-o.ClockSkew():
		assert.FailNow(t, "a superseded value was kept", "%v", got)
	default:
	}
}
