package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// A flood wait somebody ran into is state: a client attaching while it lasts
// has to see it, or that client looks frozen for the rest of it (#212).
func TestOwner_AClientAttachingDuringAThrottleSeesIt(t *testing.T) {
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), &stubConn{}, nopNotifier{})
	until := time.Unix(1_700_000_600, 0)
	o.SetThrottle("messages.getHistory", until)

	c := attach(o)
	c.Subscribe(project.AccountWindow{})

	d, ok := recvDelta(t, c)
	require.True(t, ok)
	require.NotNil(t, d.Account)
	assert.Equal(t, project.Throttle{Method: "messages.getHistory", Until: until}, d.Account.Contents.Throttle)
}

// Only the wait that ends last is kept: it is the one a client is still
// waiting out when the others are over.
func TestOwner_AThrottleEndingSoonerDoesNotReplaceOneEndingLater(t *testing.T) {
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), &stubConn{}, nopNotifier{})
	later := time.Unix(1_700_000_600, 0)

	o.SetThrottle("messages.getHistory", later)
	o.SetThrottle("messages.sendMessage", later.Add(-time.Minute))

	assert.Equal(t, project.Throttle{Method: "messages.getHistory", Until: later}, o.Account().Throttle)
}
