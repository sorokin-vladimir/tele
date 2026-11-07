package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// The host counts its windows apart from its commands, leaving out the one
// asking: `tele daemon status` is a command itself (#208, #213).
func TestHostEnd_StatusCountsWindowsAndCommandsButTheOneAsking(t *testing.T) {
	e, _, _ := hostEndAt(t, "")
	e.StatusFrom(func() int { return 2 }, func() int { return 3 }, func() *wire.AccountStatus { return nil })

	got := e.Status()

	assert.Equal(t, 2, got.Windows)
	assert.Equal(t, 2, got.Commands)
	assert.False(t, got.Since.IsZero(), "the host says when it started")
	assert.Nil(t, got.Account, "between accounts there is no account to tell of")
}

// The host says what it lets commands do with the account, so that a script
// refused with exit 7 can be told why (#181).
func TestHostEnd_StatusSaysWhatCommandsMayDo(t *testing.T) {
	all, _, _ := hostEndAt(t, "")
	none, _, _ := hostEndAt(t, "cli:\n  account_commands: none\n")

	assert.Equal(t, wire.CommandsAll, all.Status().AccountCommands)
	assert.Equal(t, wire.CommandsNone, none.Status().AccountCommands)
}

// Asked before anything is there to count, the host says what it knows.
func TestHostEnd_StatusBeforeTheHostHasClients(t *testing.T) {
	e, _, _ := hostEndAt(t, "")

	got := e.Status()

	assert.Zero(t, got.Windows)
	assert.Zero(t, got.Commands)
	assert.Nil(t, got.Account)
}

// restartingEnd is the host's end over a config, kept running by kept, and
// how many times it was asked to restart.
func restartingEnd(t *testing.T, kept wire.Keeper) (*HostEnd, *int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0o600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	hello := testHello
	hello.Kept = kept
	e := NewHostEnd(store, hello)
	restarts := 0
	e.RestartWith(func() { restarts++ })
	return e, &restarts
}

// A service restarts when a newer client asks: its service manager starts it
// again, on the binary installed now (#183). It stops only once the client
// asking has its answer.
func TestHostEnd_AServiceRestartsWhenAsked(t *testing.T) {
	e, restarts := restartingEnd(t, wire.KeptByService)

	got, then := e.Restart()

	assert.Empty(t, got.Refused)
	require.NotNil(t, then)
	assert.Zero(t, *restarts, "not before the answer is written")
	then()
	assert.Equal(t, 1, *restarts)
}

// Any other host is somebody's session, and nothing outside it ends it: it
// refuses, and says what keeps it running.
func TestHostEnd_OnlyAServiceRestarts(t *testing.T) {
	for _, kept := range []wire.Keeper{wire.KeptByWindow, wire.KeptByDaemon} {
		e, restarts := restartingEnd(t, kept)

		got, then := e.Restart()

		assert.NotEmpty(t, got.Refused, kept)
		assert.Nil(t, then, kept)
		assert.Zero(t, *restarts, kept)
	}
}

// A service told it is being uninstalled says so when it stops, which is
// what its service manager makes it do next: its clients do not wait for a
// service that will not come back (#182).
func TestHostEnd_AServiceBeingUninstalledStopsAsUninstalled(t *testing.T) {
	e, _ := restartingEnd(t, wire.KeptByService)
	assert.Equal(t, core.StopQuit, e.StopReason(), "before anyone says otherwise")

	got := e.Uninstall()

	assert.Empty(t, got.Refused)
	assert.Equal(t, core.StopUninstalled, e.StopReason())
}

// Only a service is uninstalled: any other host refuses, and stops as it
// would have.
func TestHostEnd_OnlyAServiceIsUninstalled(t *testing.T) {
	for _, kept := range []wire.Keeper{wire.KeptByWindow, wire.KeptByDaemon} {
		e, _ := restartingEnd(t, kept)

		got := e.Uninstall()

		assert.NotEmpty(t, got.Refused, kept)
		assert.Equal(t, core.StopQuit, e.StopReason(), kept)
	}
}

// The host tells how its account is doing in the protocol's words: where the
// account stands, whether its link to Telegram is up, and what is waiting to
// be sent.
func TestHost_SaysHowItsAccountIsDoing(t *testing.T) {
	h := testHostWith(t, &heldConnection{updates: make(chan store.Event)})
	acct := h.host.current()

	got := h.host.AccountStatus()
	require.NotNil(t, got)
	assert.Equal(t, wire.PhaseConnecting, got.Phase)
	assert.Equal(t, wire.LinkUnknown, got.Link)
	assert.Zero(t, got.Outbox)

	acct.owner.SetLink(true)

	got = h.host.AccountStatus()
	assert.Equal(t, wire.LinkUp, got.Link)
	assert.False(t, got.LinkSince.IsZero())
}

// A flood wait somebody's call ran into is told while it lasts, and not once
// it is over (#212).
func TestHost_SaysAWaitTelegramAskedForWhileItLasts(t *testing.T) {
	h := testHostWith(t, &heldConnection{updates: make(chan store.Event)})
	acct := h.host.current()
	until := time.Now().Add(time.Minute)

	acct.owner.SetThrottle("messages.getHistory", until)
	got := h.host.AccountStatus()
	require.NotNil(t, got.Throttle)
	assert.Equal(t, "messages.getHistory", got.Throttle.Method)
	assert.True(t, got.Throttle.Until.Equal(until))

	over := testHostWith(t, &heldConnection{updates: make(chan store.Event)})
	over.host.current().owner.SetThrottle("messages.getHistory", time.Now().Add(-time.Second))
	assert.Nil(t, over.host.AccountStatus().Throttle, "a wait that is over holds nothing")
}

// A connection that ended says why.
func TestHost_SaysWhatTheConnectionEndedWith(t *testing.T) {
	h := testHostWith(t, &bannedConnection{updates: make(chan store.Event)})
	acct := h.host.current()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.host.Start(ctx)
	require.Eventually(t, func() bool { return acct.owner.Account().Ended != nil }, 2*time.Second, 5*time.Millisecond)

	got := h.host.AccountStatus()

	require.NotNil(t, got)
	assert.NotEmpty(t, got.Ended)
}
