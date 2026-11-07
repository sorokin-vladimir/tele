package hosting_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
)

// serviceHost is a host the service manager keeps running: it says it is one,
// and restarts when asked by calling restart.
type serviceHost struct {
	hello   wire.Hello
	restart func()
}

func (s serviceHost) Hello() wire.Hello                      { return s.hello }
func (serviceHost) Settings() wire.HostSettings              { return wire.HostSettings{} }
func (serviceHost) SetSetting(wire.SetParams) wire.SetResult { return wire.SetResult{} }
func (serviceHost) Reload() wire.ReloadResult                { return wire.ReloadResult{} }
func (serviceHost) Status() wire.HostStatus                  { return wire.HostStatus{} }
func (serviceHost) Uninstall() wire.UninstallResult          { return wire.UninstallResult{} }
func (serviceHost) AccountCommands() wire.CommandAccess      { return wire.CommandsAll }
func (s serviceHost) Restart() (wire.RestartResult, func()) {
	return wire.RestartResult{}, func() { go s.restart() }
}

// serviceManager runs a service over a state directory, and starts it again
// whenever it is restarted, as the binary in next says by then.
type serviceManager struct {
	t   *testing.T
	dir string

	mu       sync.Mutex
	next     string
	restarts int
	stop     func()
}

func runService(t *testing.T, dir, binary string) *serviceManager {
	t.Helper()
	m := &serviceManager{t: t, dir: dir, next: binary}
	m.start()
	t.Cleanup(func() {
		m.mu.Lock()
		stop := m.stop
		m.mu.Unlock()
		stop()
	})
	return m
}

// installed is what the service manager starts on its next restart.
func (m *serviceManager) installed(binary string) {
	m.mu.Lock()
	m.next = binary
	m.mu.Unlock()
}

func (m *serviceManager) start() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := hosting.Reach(ctx, app, m.dir)
	if err != nil || r.Listener == nil {
		m.t.Errorf("the service could not take its state directory: %v", err)
		return
	}
	m.mu.Lock()
	hello := wire.Hello{Protocol: wire.ProtocolVersion, Binary: m.next, Kept: wire.KeptByService}
	m.mu.Unlock()
	var s *hosting.Server
	var once sync.Once
	stop := func(reason core.StopReason) {
		once.Do(func() {
			s.Stop(time.Second, reason)
			r.Close()
		})
	}
	s = hosting.Serve(context.Background(), r.Listener, serviceHost{hello: hello, restart: func() {
		stop(core.StopRestart)
		m.mu.Lock()
		m.restarts++
		m.mu.Unlock()
		m.start()
	}}, hosting.NewClients())
	m.mu.Lock()
	m.stop = func() { stop(core.StopQuit) }
	m.mu.Unlock()
}

func (m *serviceManager) restarted() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.restarts
}

// attach attaches to the host of dir the way a window does, saying binary.
func attach(t *testing.T, dir, binary string) (*dial.Attachment, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := hosting.Reach(ctx, app, dir)
	require.NoError(t, err)
	require.NotNil(t, r.Conn, "a host was there to reach")
	a, err := hosting.Attach(ctx, r, wire.Hello{Protocol: wire.ProtocolVersion, Binary: binary})
	if a != nil {
		t.Cleanup(func() { _ = a.Close() })
	}
	return a, err
}

// A window newer than the service asks it to restart, and attaches to it once
// the service manager has started it again on the binary installed now (#183).
func TestAttach_ANewerWindowRestartsAnOlderService(t *testing.T) {
	dir := stateDir(t)
	m := runService(t, dir, "2.0.0-beta.1")
	m.installed("2.0.0")

	a, err := attach(t, dir, "2.0.0")

	require.NoError(t, err)
	assert.Equal(t, "2.0.0", a.Host().Binary)
	assert.Equal(t, 1, m.restarted())
}

// A service started again on the old binary - its unit names a file the
// upgrade did not replace - is not restarted in a loop: the window says so.
func TestAttach_AServiceThatComesBackOldIsNotRestartedAgain(t *testing.T) {
	dir := stateDir(t)
	m := runService(t, dir, "2.0.0-beta.1")

	_, err := attach(t, dir, "2.0.0")

	assert.ErrorIs(t, err, hosting.ErrServiceStillOlder)
	assert.Equal(t, 1, m.restarted())
}

// A window older than the service is told tele was updated, and the service
// is left alone.
func TestAttach_AnOlderWindowLeavesANewerServiceAlone(t *testing.T) {
	dir := stateDir(t)
	m := runService(t, dir, "2.0.0")

	_, err := attach(t, dir, "2.0.0-beta.1")

	var mismatch *dial.Mismatch
	require.True(t, errors.As(err, &mismatch), "got %v", err)
	assert.Equal(t, wire.RestartClient, mismatch.Verdict)
	assert.Zero(t, m.restarted())
}

// A window attached to a service waits for it through a restart, and carries
// on with the service started again (#210).
func TestAttach_AWindowOutlivesItsServiceRestarting(t *testing.T) {
	dir := stateDir(t)
	m := runService(t, dir, "2.0.0")
	a, err := attach(t, dir, "2.0.0")
	require.NoError(t, err)

	got, err := a.RestartHost(context.Background())
	require.NoError(t, err)
	require.Empty(t, got.Refused)

	deadline := time.After(5 * time.Second)
	for back := false; !back; {
		select {
		case v, ok := <-a.Stream():
			require.True(t, ok, "the window gave up on its service: %v", a.Lost())
			_, back = v.(dial.Reconnected)
		case <-deadline:
			require.FailNow(t, "the window never got its service back")
		}
	}
	assert.Equal(t, 1, m.restarted())
}

// The same version attaches as it always did.
func TestAttach_TheSameVersionAttaches(t *testing.T) {
	dir := stateDir(t)
	m := runService(t, dir, "2.0.0")

	a, err := attach(t, dir, "2.0.0")

	require.NoError(t, err)
	assert.Equal(t, "2.0.0", a.Host().Binary)
	assert.Zero(t, m.restarted())
}
