package tg_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/crypto"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram/dcs"
	gotdtg "github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// savedSession writes a session file for DC 1 whose config lists the given
// DC options, the way gotd leaves it after a login.
func savedSession(t *testing.T, opts []gotdtg.DCOption) string {
	t.Helper()
	var key crypto.Key
	_, err := rand.Read(key[:])
	require.NoError(t, err)
	ak := key.WithID()

	path := filepath.Join(t.TempDir(), "session.json")
	loader := session.Loader{Storage: internaltg.NewFileSession(path)}
	require.NoError(t, loader.Save(context.Background(), &session.Data{
		Config:    session.ConfigFromTG(gotdtg.Config{ThisDC: 1, DCOptions: opts}),
		DC:        1,
		AuthKey:   ak.Value[:],
		AuthKeyID: ak.ID[:],
	}))
	return path
}

// dialledOnStart starts the client on the session file with every dial
// refused, and reports the addresses it tried.
func dialledOnStart(t *testing.T, sessionFile string) map[string]bool {
	t.Helper()
	var mu sync.Mutex
	dialled := map[string]bool{}
	resolver := dcs.Plain(dcs.PlainOptions{Dial: func(_ context.Context, _, addr string) (net.Conn, error) {
		mu.Lock()
		dialled[addr] = true
		mu.Unlock()
		return nil, errors.New("unreachable")
	}})

	c := internaltg.NewGotdClient(zap.NewNop(), internaltg.Endpoint{SessionFile: sessionFile, Resolver: resolver}, nil, false)
	c.SetOnLink(func(bool) {})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = c.Connect(ctx, internaltg.NewAuthFlow(zap.NewNop()), make(chan struct{}), nil)

	mu.Lock()
	defer mu.Unlock()
	return dialled
}

// gotd keeps the addresses Telegram sent in the session but starts from its
// own static list, which can name addresses Telegram no longer serves (#312).
func TestConnect_DialsTheAddressesTheSessionSaved(t *testing.T) {
	path := savedSession(t, []gotdtg.DCOption{{ID: 1, IPAddress: "10.31.2.1", Port: 443}})

	dialled := dialledOnStart(t, path)

	assert.True(t, dialled["10.31.2.1:443"], "saved address not dialled; dialled %v", dialled)
	for _, opt := range dcs.Prod().Options {
		assert.False(t, dialled[net.JoinHostPort(opt.IPAddress, "443")], "static address %s dialled", opt.IPAddress)
	}
}

func TestConnect_WithNoSavedAddressesDialsTheStaticList(t *testing.T) {
	path := savedSession(t, nil)

	dialled := dialledOnStart(t, path)

	for _, opt := range dcs.FindPrimaryDCs(dcs.Prod().Options, 1, false) {
		assert.True(t, dialled[net.JoinHostPort(opt.IPAddress, "443")], "static address %s not dialled; dialled %v", opt.IPAddress, dialled)
	}
}
