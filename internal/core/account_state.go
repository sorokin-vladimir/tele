package core

import (
	"context"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// accountState is where the account stands, as the account projection shows
// it. It is state rather than a stream of happenings: a client attaching in the
// middle of a login, or while the clock is skewed, sees the step and the skew,
// not nothing (#309).
type accountState struct {
	mu sync.Mutex
	c  project.AccountContents
	// waiting is set while the step in c.Login has not been answered. One
	// answer moves it on; a second for the same asking is refused.
	waiting bool
	// driver is the client driving the login: the one that answered its number,
	// and the only one whose answers are taken until the login ends or the
	// client goes. Zero while nobody drives it (#211).
	driver clientID
	// abandoned is set when the driver went while no step was waiting: the
	// login was talking to Telegram, and the step it asks next is answered as
	// abandoned rather than put to the clients.
	abandoned bool
}

// Account is where the account stands now.
func (o *Owner) Account() project.AccountContents {
	o.acct.mu.Lock()
	defer o.acct.mu.Unlock()
	return o.acct.c
}

// updateAccount changes where the account stands and tells every client
// watching it.
func (o *Owner) updateAccount(fn func(c *project.AccountContents)) {
	o.acct.mu.Lock()
	fn(&o.acct.c)
	o.acct.mu.Unlock()
	o.Refresh()
}

// runLogin follows the login from the connection into the account state, and
// once the account is logged in loads its chat list and its folders. It ends
// with the login failing for good, with the chat list loaded, or with ctx.
func (o *Owner) runLogin(ctx context.Context) {
	af := o.authFlow
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-af.Requests:
			if req.Update {
				// New content of the asking on screen - a token, or none
				// and why - under the same Seq, so an answer on its way still
				// names it (#330).
				o.updateAccount(func(c *project.AccountContents) {
					c.Login.Token, c.Login.Expires, c.Login.Err = req.Token, req.Expires, req.Err
				})
				continue
			}
			o.log.Debug("auth step requested", zap.Int("step", int(req.Step)))
			o.acct.mu.Lock()
			abandoned := o.acct.abandoned
			o.acct.abandoned = false
			o.acct.mu.Unlock()
			if abandoned {
				o.abandonLogin(ctx)
				continue
			}
			o.updateAccount(func(c *project.AccountContents) {
				c.Phase = project.AccountLoggingIn
				c.Login = project.LoginStep{Kind: loginStepKind(req.Step), Hint: req.Hint, Err: req.Err, Seq: c.Login.Seq + 1}
				o.acct.waiting = true
			})
		case text := <-af.Errors:
			o.log.Error("auth error", zap.String("reason", text))
			o.updateAccount(func(c *project.AccountContents) { c.LoginFailed = text })
			return
		case <-o.readyCh:
			o.log.Info("connected, loading dialogs")
			o.updateAccount(func(c *project.AccountContents) {
				c.Phase = project.AccountLoading
				c.Login = project.LoginStep{Seq: c.Login.Seq}
				o.acct.waiting = false
				o.acct.driver = 0
			})
			o.loadAccount(ctx)
			return
		}
	}
}

// loadAccount loads the chat list and, alongside it, the folders. The account
// is ready once the chat list is in: folders follow on their own, and the
// cached ones are on screen until they do.
func (o *Owner) loadAccount(ctx context.Context) {
	o.spawn(func() {
		if _, err := o.LoadFolderFilters(ctx); err != nil {
			o.log.Warn("GetDialogFilters failed", zap.Error(err))
			return
		}
		// Folders are written without a change to commit, so the chat lists
		// on screen are rebuilt here.
		o.Refresh()
	})
	if err := o.Bootstrap(ctx); err != nil {
		o.log.Error("GetDialogs failed", zap.Error(err))
		return
	}
	o.updateAccount(func(c *project.AccountContents) { c.Phase = project.AccountReady })
}

// loggedIn records who the account is logged in as, for every client watching
// it.
func (o *Owner) loggedIn(self domain.User) {
	name := strings.TrimSpace(self.FirstName + " " + self.LastName)
	o.updateAccount(func(c *project.AccountContents) {
		c.LoggedInUser = project.LoggedInUser{ID: self.ID, Username: self.Username, Name: name}
	})
}

// historyPageIs records how many messages the owner fetches at a time, for
// every client sizing its window by it.
func (o *Owner) historyPageIs(n int) {
	o.updateAccount(func(c *project.AccountContents) { c.HistoryPage = n })
}

// AnswerLogin answers, for the client from, the step of the login named by
// seq. An answer for an asking that is no longer the one waiting - another
// client answered first, or the step moved on - is refused, so two clients
// typing at once cannot answer one step twice. The client that answers the
// number drives the login from there, and another client's answer is refused
// while it does (#211).
func (s *seat) AnswerLogin(ctx context.Context, seq int, value string) error {
	return s.answerLogin(ctx, s.id, seq, value)
}

func (o *Owner) answerLogin(ctx context.Context, from clientID, seq int, value string) error {
	o.acct.mu.Lock()
	if !o.acct.waiting || o.acct.c.Login.Seq != seq {
		o.acct.mu.Unlock()
		return &telerr.Error{Kind: telerr.NotFound, Op: "login", Detail: "this login step was already answered"}
	}
	if o.acct.driver != 0 && o.acct.driver != from {
		o.acct.mu.Unlock()
		return &telerr.Error{Kind: telerr.Forbidden, Op: "login", Detail: "this login is being answered in another window"}
	}
	if o.acct.c.Login.Kind == project.LoginPhone {
		o.acct.driver = from
	}
	o.acct.waiting = false
	o.acct.mu.Unlock()
	select {
	case o.authFlow.Responses <- internaltg.AuthResponse{Value: value}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SwitchLogin leaves, for the client, the step of the login named by seq for
// the other way to log in. It is held to what an answer is: one for an asking
// gone is refused, and only the driver leaves a step it drives (#330).
func (s *seat) SwitchLogin(ctx context.Context, seq int, to project.LoginStepKind) error {
	return s.switchLogin(ctx, s.id, seq, to)
}

func (o *Owner) switchLogin(ctx context.Context, from clientID, seq int, to project.LoginStepKind) error {
	o.acct.mu.Lock()
	kind := o.acct.c.Login.Kind
	var leave error
	switch {
	case kind == project.LoginQR && to == project.LoginPhone:
		leave = internaltg.ErrLoginByPhone
	case kind != project.LoginQR && to == project.LoginQR:
		leave = internaltg.ErrLoginBackToQR
	}
	if !o.acct.waiting || o.acct.c.Login.Seq != seq || leave == nil {
		o.acct.mu.Unlock()
		return &telerr.Error{Kind: telerr.NotFound, Op: "login", Detail: "this login step is no longer on screen"}
	}
	if o.acct.driver != 0 && o.acct.driver != from {
		o.acct.mu.Unlock()
		return &telerr.Error{Kind: telerr.Forbidden, Op: "login", Detail: "this login is being answered in another window"}
	}
	// The QR code belongs to nobody: back at it, the attempt by number has
	// ended, and with it the driver's hold.
	o.acct.driver = 0
	o.acct.waiting = false
	o.acct.mu.Unlock()
	select {
	case o.authFlow.Responses <- internaltg.AuthResponse{Err: leave}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SetWatched says whether a window is attached to the host, which is when the
// login issues a QR code to show (#330). It is the host's to say: only the
// host tells a window from a command.
func (o *Owner) SetWatched(on bool) { o.authFlow.SetWatched(on) }

// driverGone abandons the login driven by the client id, which has gone. A
// step waiting for its answer is answered as abandoned at once; with none
// waiting, the next one is (#211).
func (o *Owner) driverGone(id clientID) {
	o.acct.mu.Lock()
	if o.acct.driver != id {
		o.acct.mu.Unlock()
		return
	}
	o.acct.driver = 0
	waiting := o.acct.waiting
	o.acct.waiting = false
	o.acct.abandoned = !waiting
	o.acct.mu.Unlock()
	if waiting {
		o.spawn(func() { o.abandonLogin(o.ctx) })
	}
}

// abandonLogin answers the step the login is waiting on as abandoned, which
// starts the login again from the number.
func (o *Owner) abandonLogin(ctx context.Context) {
	select {
	case o.authFlow.Responses <- internaltg.AuthResponse{Err: internaltg.ErrLoginAbandoned}:
	case <-ctx.Done():
	}
}

// connectionEnded records what the connection ended with, for every client to
// see. A connection cancelled because the account is stopping ended with
// nothing worth saying, and a log out is the end of the account rather than of
// a connection: the host ends it and starts the next (#297).
func (o *Owner) connectionEnded(ctx context.Context, err error) {
	if err == nil || ctx.Err() != nil {
		return
	}
	if _, loggedOut := EndReasonOf(err); loggedOut {
		return
	}
	o.updateAccount(func(c *project.AccountContents) { c.Ended = telerr.From(err) })
}

func loginStepKind(s internaltg.AuthStep) project.LoginStepKind {
	switch s {
	case internaltg.AuthStepQR:
		return project.LoginQR
	case internaltg.AuthStepCode:
		return project.LoginCode
	case internaltg.AuthStepPassword:
		return project.LoginPassword
	default:
		return project.LoginPhone
	}
}
