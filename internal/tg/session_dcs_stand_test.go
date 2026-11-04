package tg_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	gotdtg "github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"

	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// Tripwire for the saved DC list (#312): gotd itself, given a session that
// saved its own addresses, still dials its static list. When this fails,
// gotd restores the saved config and savedDCList can go.
func TestStand_WithoutTheSavedListASessionDialsTheStaticAddresses(t *testing.T) {
	path := savedSession(t, []gotdtg.DCOption{{ID: 1, IPAddress: "10.31.2.1", Port: 443}})

	var mu sync.Mutex
	dialled := map[string]bool{}
	resolver := dcs.Plain(dcs.PlainOptions{Dial: func(_ context.Context, _, addr string) (net.Conn, error) {
		mu.Lock()
		dialled[addr] = true
		mu.Unlock()
		return nil, errors.New("unreachable")
	}})

	client := telegram.NewClient(1, "hash", telegram.Options{
		SessionStorage: internaltg.NewFileSession(path),
		Resolver:       resolver,
		NoUpdates:      true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = client.Run(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil })

	mu.Lock()
	defer mu.Unlock()
	assert.False(t, dialled["10.31.2.1:443"], "gotd dialled the saved address itself: the workaround can go")
	assert.NotEmpty(t, dialled, "gotd dialled nothing: the stand no longer reaches the dial")
}
