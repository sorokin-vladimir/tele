package hosting_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// flowConn hands the login it is given to the test.
type flowConn struct {
	noConn
	flow chan *internaltg.AuthFlow
}

func (c flowConn) Connect(ctx context.Context, af *internaltg.AuthFlow, _ chan<- struct{}, _ func(domain.User)) error {
	c.flow <- af
	<-ctx.Done()
	return nil
}

// loginHost is a host whose owner is logging in, and the login it runs.
func loginHost(t *testing.T) (*hosting.Clients, *internaltg.AuthFlow) {
	t.Helper()
	conn := flowConn{flow: make(chan *internaltg.AuthFlow, 1)}
	o := core.New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), conn, nopNotifier{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		o.Stop()
	})
	go func() { _ = o.Start(ctx) }()
	clients := hosting.NewClients()
	clients.Join(o)
	select {
	case af := <-conn.flow:
		return clients, af
	case <-time.After(2 * time.Second):
		t.Fatal("the owner never connected")
		return nil, nil
	}
}

func watched(af *internaltg.AuthFlow) bool {
	select {
	case <-af.Watched():
		return true
	default:
		return false
	}
}

// A QR code is issued only while a window is attached: a command is not a
// person looking, and with the last window gone nobody is (#330).
func TestLogin_AWindowIsWhatTheQRCodeIsIssuedFor(t *testing.T) {
	clients, af := loginHost(t)
	ep, _ := host(t, clients)

	cmd := clientAs(t, ep, wire.KindCommand)
	require.Eventually(t, func() bool { return clients.Commands() == 1 }, 2*time.Second, 10*time.Millisecond)
	assert.False(t, watched(af), "a command is not watching")

	win := clientAs(t, ep, wire.KindWindow)
	require.Eventually(t, func() bool { return watched(af) }, 2*time.Second, 10*time.Millisecond,
		"a window is watching")

	_ = win.Close()
	require.Eventually(t, func() bool { return !watched(af) }, 2*time.Second, 10*time.Millisecond,
		"nobody is watching once the last window went")
	_ = cmd.Close()
}

// A window attached before the owner starts is watching that owner too.
func TestLogin_AWindowAlreadyThereWatchesTheNextOwner(t *testing.T) {
	clients := hosting.NewClients()
	ep, _ := host(t, clients)
	clientAs(t, ep, wire.KindWindow)
	require.Eventually(t, func() bool { return clients.Windows() == 1 }, 2*time.Second, 10*time.Millisecond)

	conn := flowConn{flow: make(chan *internaltg.AuthFlow, 1)}
	o := core.New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), conn, nopNotifier{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		o.Stop()
	})
	clients.Join(o)
	go func() { _ = o.Start(ctx) }()

	af := <-conn.flow
	assert.True(t, watched(af))
}

// A switch of the login crosses the socket and reaches the login.
func TestLogin_ASwitchCrossesTheSocket(t *testing.T) {
	clients, af := loginHost(t)
	ep, _ := host(t, clients)
	a := clientAs(t, ep, wire.KindWindow)
	a.Subscribe(project.AccountWindow{})

	go func() { af.Requests <- internaltg.AuthRequest{Step: internaltg.AuthStepQR} }()
	var seq int
	for seq == 0 {
		d, ok := next(t, a).(project.Delta)
		if ok && d.Account != nil && d.Account.Contents.Login.Kind == project.LoginQR {
			seq = d.Account.Contents.Login.Seq
		}
	}
	// The switch is answered once the login takes it, as an answer is.
	reached := make(chan internaltg.AuthResponse, 1)
	go func() { reached <- <-af.Responses }()

	require.NoError(t, a.SwitchLogin(context.Background(), seq, project.LoginPhone))

	select {
	case resp := <-reached:
		assert.ErrorIs(t, resp.Err, internaltg.ErrLoginByPhone)
	case <-time.After(2 * time.Second):
		t.Fatal("the switch never reached the login")
	}
}
