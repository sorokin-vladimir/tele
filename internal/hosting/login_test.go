package hosting_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/hosting"
	"github.com/sorokin-vladimir/tele/internal/ipc/dial"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// loginConn asks the three steps of a login the way the connection does, and
// keeps what it was answered with.
type loginConn struct {
	noConn
	answered chan []string
}

func (c loginConn) Connect(ctx context.Context, af *internaltg.AuthFlow, ready chan<- struct{}, _ func(domain.User)) error {
	phone, err := af.Phone(ctx)
	if err != nil {
		return err
	}
	code, err := af.Code(ctx, &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{}})
	if err != nil {
		return err
	}
	password, err := af.Password(ctx)
	if err != nil {
		return err
	}
	c.answered <- []string{phone, code, password}
	<-ctx.Done()
	return nil
}

// answer answers the next step of kind a client is asked.
func answer(t *testing.T, a *dial.Attachment, kind project.LoginStepKind, value string) {
	t.Helper()
	for {
		d, ok := next(t, a).(project.Delta)
		if !ok || d.Account == nil {
			continue
		}
		c := d.Account.Contents
		if c.Phase == project.AccountLoggingIn && c.Login.Kind == kind {
			require.NoError(t, a.AnswerLogin(context.Background(), c.Login.Seq, value))
			return
		}
	}
}

// What a person types to log in crosses the socket, and none of it is ever
// logged, at any level: a convention about secrets is the kind that erodes, so
// it is a test (#211). gotd's own trace output is outside what this reaches.
func TestLogin_WhatIsTypedIsNeverLogged(t *testing.T) {
	logged, logs := observer.New(zap.DebugLevel)
	conn := loginConn{answered: make(chan []string, 1)}
	o := core.New(&config.Config{}, zap.New(logged), state.New(store.NewMemory()), conn, nopNotifier{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		o.Stop()
	})
	go func() { _ = o.Start(ctx) }()
	clients := hosting.NewClients()
	clients.Join(o)
	ep, _ := host(t, clients)
	a := client(t, ep)
	a.Subscribe(project.AccountWindow{})

	const phone, code, password = "+15550001111", "54321", "correct horse battery"
	answer(t, a, project.LoginPhone, phone)
	answer(t, a, project.LoginCode, code)
	answer(t, a, project.LoginPassword, password)

	select {
	case got := <-conn.answered:
		require.Equal(t, []string{phone, code, password}, got, "the answers reached the login")
	case <-time.After(2 * time.Second):
		t.Fatal("the login was never answered")
	}
	require.NotZero(t, logs.Len(), "the login logged something to look through")
	for _, e := range logs.All() {
		line := fmt.Sprint(e.Message, e.ContextMap())
		for _, secret := range []string{phone, code, password} {
			assert.NotContains(t, line, secret)
		}
	}
}
