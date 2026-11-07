package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// loginConn answers what loading a logged-in account asks: the two dialog
// lists and the folders.
type loginConn struct {
	internaltg.Client
	folders []domain.FolderFilter
}

func (loginConn) GetDialogs(context.Context) ([]domain.Chat, error) {
	return []domain.Chat{{ID: 1, Title: "Ada"}}, nil
}

func (loginConn) GetArchivedDialogs(context.Context) ([]domain.Chat, error) { return nil, nil }

func (c loginConn) GetDialogFilters(context.Context) ([]domain.FolderFilter, error) {
	return c.folders, nil
}

func (loginConn) Connect(context.Context, *internaltg.AuthFlow, chan<- struct{}, func(domain.User)) error {
	return nil
}

func (loginConn) Updates() <-chan store.Event { return nil }

// loginOwner is an owner running its login, with a client watching the
// account.
func loginOwner(t *testing.T, c Connection) (*Owner, *Attachment) {
	t.Helper()
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), c, nopNotifier{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		o.Stop()
	})
	o.spawn(func() { o.runLogin(ctx) })
	a := attach(o)
	a.Subscribe(project.AccountWindow{})
	first, ok := recvAccount(t, a)
	require.True(t, ok)
	require.Equal(t, project.AccountConnecting, first.Phase)
	return o, a
}

func recvAccount(t *testing.T, a *Attachment) (project.AccountContents, bool) {
	t.Helper()
	for {
		d, ok := recvDelta(t, a)
		if !ok {
			return project.AccountContents{}, false
		}
		if d.Account != nil {
			return d.Account.Contents, true
		}
	}
}

// ask puts a login step to the owner the way the connection does, and returns
// what it is answered with.
func ask(o *Owner, req internaltg.AuthRequest) <-chan internaltg.AuthResponse {
	answered := make(chan internaltg.AuthResponse, 1)
	go func() {
		o.authFlow.Requests <- req
		answered <- <-o.authFlow.Responses
	}()
	return answered
}

// A step waiting for an answer is state: every client sees it, including one
// that attaches while it waits (#309).
func TestLogin_AClientAttachingMidLoginSeesTheStep(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode, Hint: "Enter the SMS code:"})
	_, ok := recvAccount(t, a)
	require.True(t, ok)

	late := attach(o)
	late.Subscribe(project.AccountWindow{})

	got, ok := recvAccount(t, late)
	require.True(t, ok)
	assert.Equal(t, project.AccountLoggingIn, got.Phase)
	assert.Equal(t, project.LoginCode, got.Login.Kind)
	assert.Equal(t, "Enter the SMS code:", got.Login.Hint)
}

// One answer moves a step on. A second for the same asking - another client
// typing at the same time - is refused rather than taken for the next step.
func TestLogin_AStepIsAnsweredOnce(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepPhone})
	step, ok := recvAccount(t, a)
	require.True(t, ok)

	require.NoError(t, a.AnswerLogin(context.Background(), step.Login.Seq, "+100"))
	assert.Equal(t, "+100", (<-answered).Value)

	err := a.AnswerLogin(context.Background(), step.Login.Seq, "+200")
	assert.Equal(t, telerr.NotFound, telerr.Of(err))
}

// An answer naming an asking that is gone is refused even while another step
// waits: it was typed at a prompt that is no longer on screen.
func TestLogin_AnAnswerToAnOldAskingIsRefused(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepPhone})
	first, _ := recvAccount(t, a)
	require.NoError(t, a.AnswerLogin(context.Background(), first.Login.Seq, "+100"))
	<-answered
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	second, _ := recvAccount(t, a)

	err := a.AnswerLogin(context.Background(), first.Login.Seq, "12345")

	assert.Equal(t, telerr.NotFound, telerr.Of(err))
	assert.Greater(t, second.Login.Seq, first.Login.Seq)
}

// drive answers the phone step through a, making it the login's driver.
func drive(t *testing.T, o *Owner, a *Attachment) {
	t.Helper()
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepPhone})
	step, ok := recvAccount(t, a)
	require.True(t, ok)
	require.NoError(t, a.AnswerLogin(context.Background(), step.Login.Seq, "+100"))
	<-answered
}

// One client drives a login. Another one's answer is refused while it does,
// and says why, rather than mixing two people's typing into one login (#211).
func TestLogin_OnlyTheDriverAnswers(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	b := attach(o)
	drive(t, o, a)
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	step, ok := recvAccount(t, a)
	require.True(t, ok)

	err := b.AnswerLogin(context.Background(), step.Login.Seq, "11111")
	require.Error(t, err)
	assert.Equal(t, telerr.Forbidden, telerr.Of(err))
	assert.Contains(t, err.Error(), "another window")

	require.NoError(t, a.AnswerLogin(context.Background(), step.Login.Seq, "12345"))
	assert.Equal(t, "12345", (<-answered).Value)
}

// A driver that goes while a step waits abandons the login: the code it was
// about to type is on its person's phone, and nobody else should finish it.
// Whoever answers next starts again, and may be anyone (#211).
func TestLogin_TheDriverGoingAbandonsTheStepWaiting(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	b := attach(o)
	b.Subscribe(project.AccountWindow{})
	_, _ = recvAccount(t, b)
	drive(t, o, a)
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	code := recvStep(t, a, project.LoginCode, 0)

	a.Detach()

	assert.ErrorIs(t, answer(t, answered).Err, internaltg.ErrLoginAbandoned)
	again := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepPhone})
	step := recvStep(t, b, project.LoginPhone, code.Seq)
	require.NoError(t, b.AnswerLogin(context.Background(), step.Seq, "+200"))
	assert.Equal(t, "+200", answer(t, again).Value)
}

// A driver that goes while the login is waiting on Telegram abandons it too:
// the step asked next is never put to anyone, and the login starts again.
func TestLogin_TheDriverGoingBetweenStepsAbandonsTheNext(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	b := attach(o)
	b.Subscribe(project.AccountWindow{})
	_, _ = recvAccount(t, b)
	drive(t, o, a)

	a.Detach()

	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	assert.ErrorIs(t, answer(t, answered).Err, internaltg.ErrLoginAbandoned)
	again := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepPhone})
	step := recvStep(t, b, project.LoginPhone, 1)
	require.NoError(t, b.AnswerLogin(context.Background(), step.Seq, "+200"))
	assert.Equal(t, "+200", answer(t, again).Value)
	assert.Equal(t, 2, step.Seq, "the abandoned code step was never asked")
}

// answer is what the login is answered with, failing the test if nothing is.
func answer(t *testing.T, answered <-chan internaltg.AuthResponse) internaltg.AuthResponse {
	t.Helper()
	select {
	case r := <-answered:
		return r
	case <-time.After(time.Second):
		require.FailNow(t, "the step was never answered")
		return internaltg.AuthResponse{}
	}
}

// recvStep is the next asking of kind after the asking seq that a hears.
func recvStep(t *testing.T, a *Attachment, kind project.LoginStepKind, after int) project.LoginStep {
	t.Helper()
	for {
		got, ok := recvAccount(t, a)
		require.True(t, ok)
		if got.Phase == project.AccountLoggingIn && got.Login.Kind == kind && got.Login.Seq > after {
			return got.Login
		}
	}
}

func TestLogin_AFailureThatEndsTheLoginIsShown(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})

	o.authFlow.Errors <- "Login requires email verification."

	got, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, "Login requires email verification.", got.LoginFailed)
}

// Logged in, the account loads its chat list and is ready; the folders come
// with the chat list rather than on their own.
func TestLogin_LoggedInLoadsTheAccount(t *testing.T) {
	c := loginConn{folders: []domain.FolderFilter{{ID: 2, Title: "Work"}}}
	o, a := loginOwner(t, c)
	list := attach(o)
	list.Subscribe(project.ChatListWindow{Limit: 10})

	close(o.readyCh)

	loading, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, project.AccountLoading, loading.Phase)
	ready, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, project.AccountReady, ready.Phase)
	require.Eventually(t, func() bool {
		for _, d := range only[project.Delta](queued(list)) {
			if d.ChatList != nil && d.ChatList.Kind == project.ChatListFolders && len(d.ChatList.Folders.Filters) == 1 {
				return true
			}
		}
		return false
	}, time.Second, 10*time.Millisecond, "the folders reach the chat list")
}

// Who the account is logged in as is part of the account, for a client
// attached at any moment: it is told, never left to ask the host beside the
// endpoint (#208).
func TestLogin_TheLoggedInUserIsTheAccounts(t *testing.T) {
	o, a := loginOwner(t, loginConn{})

	o.onAuth(domain.User{ID: 42, Username: "ada", FirstName: "Ada", LastName: "Lovelace"})

	ada := project.LoggedInUser{ID: 42, Username: "ada", Name: "Ada Lovelace"}
	got, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, ada, got.LoggedInUser)

	late := attach(o)
	late.Subscribe(project.AccountWindow{})
	first, ok := recvAccount(t, late)
	require.True(t, ok)
	assert.Equal(t, ada, first.LoggedInUser, "a client attaching later sees it too")
}

// How much history a page is is the owner's to say, since it is what the owner
// fetches: a client sizes its window by it rather than by a setting of its own,
// and hears it again when the host's config changes (#208).
func TestAccount_SaysHowMuchHistoryAPageIs(t *testing.T) {
	o := New(&config.Config{UI: config.UIConfig{HistoryLimit: 40}}, zap.NewNop(), state.New(store.NewMemory()), loginConn{}, nopNotifier{})
	t.Cleanup(o.Stop)
	a := attach(o)
	a.Subscribe(project.AccountWindow{})

	first, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, 40, first.HistoryPage)

	o.SetConfig(&config.Config{UI: config.UIConfig{HistoryLimit: 70}})

	next, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, 70, next.HistoryPage)
}

// The login follows the account and ends with it: a login nobody finished must
// not hold the account's end up (#297).
func TestLogin_EndsWithTheAccount(t *testing.T) {
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), &stubConn{}, nopNotifier{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		o.runLogin(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "the login outlived the account")
	}
}

// A connection that ends is shown to every client. One cancelled because the
// account is stopping, and a log out, are not: the first ended nothing worth
// saying, the second ends the account rather than the connection.
func TestOwner_WhatTheConnectionEndedWithIsTheAccounts(t *testing.T) {
	o := New(&config.Config{}, zap.NewNop(), state.New(store.NewMemory()), &stubConn{}, nopNotifier{})
	ctx := context.Background()

	o.connectionEnded(ctx, &telerr.Error{Kind: telerr.Unauthorized, LogOut: telerr.LogOutElsewhere})
	assert.Nil(t, o.Account().Ended, "a log out is the host's to act on")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	o.connectionEnded(cancelled, errors.New("context canceled"))
	assert.Nil(t, o.Account().Ended, "a stop is not a failure")

	failed := &telerr.Error{Kind: telerr.Network, Detail: "connection refused"}
	o.connectionEnded(ctx, failed)
	assert.Equal(t, error(failed), o.Account().Ended)
}
