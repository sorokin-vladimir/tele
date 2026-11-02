package project

import (
	"time"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// AccountWindow is the account as a whole: how far it has got since its owner
// started, the login step it is waiting on, how its connection ended, and the
// clock. There is one of it, so the window names nothing (#309).
type AccountWindow struct{}

func (AccountWindow) isWindow() {}

// AccountPhase is how far an account has got since its owner started.
type AccountPhase int

const (
	// AccountConnecting means nothing has been heard from Telegram yet.
	AccountConnecting AccountPhase = iota
	// AccountLoggingIn means Telegram is waiting on a step of the login; the
	// contents' Login says which.
	AccountLoggingIn
	// AccountLoading means the account is logged in and its chat list is being
	// loaded.
	AccountLoading
	// AccountReady means the chat list is loaded.
	AccountReady
)

// LoginStepKind names a step of the login.
type LoginStepKind int

const (
	LoginPhone LoginStepKind = iota
	LoginCode
	LoginPassword
	// LoginQR shows a QR code for an app already logged in to scan (#330).
	LoginQR
)

// LoginStep is the step of the login waiting for an answer. It is state, not a
// question put to one client: whoever attaches while it waits sees it, and
// whoever answers first moves it on. It carries nothing the person typed, only
// what the screen says about the step, and on the QR step the login token to
// draw: shown to whoever looks, and never logged.
type LoginStep struct {
	Kind LoginStepKind
	// Hint is how the code was sent, for the code step.
	Hint string
	// Err says why the step is asked again: the answer given last time was
	// refused. Empty the first time (#285). On the QR step it says why no QR
	// code is shown.
	Err string
	// Token is the URL the QR code draws, and Expires when it stops being
	// good; empty while there is none to draw. A new token is new content of
	// the same asking, under the same Seq.
	Token   string
	Expires time.Time
	// Seq tells one asking of a step from the next, so an answer names the
	// asking it answers and one for an asking already gone is refused.
	Seq int
}

// AccountContents is everything an account subscription shows.
type AccountContents struct {
	Phase AccountPhase
	// Login is the step waiting for an answer, while Phase is AccountLoggingIn.
	Login LoginStep
	// LoginFailed says, in full, why the login cannot go on from here; empty
	// while it can.
	LoginFailed string
	// Ended is what the connection ended with, nil while it runs. A domain
	// error, since it crosses to a client as a value (#309).
	Ended *telerr.Error
	// ClockSkew is how far the local clock is from Telegram's, when it is far
	// enough that messages between them are refused; zero otherwise (#277).
	ClockSkew time.Duration
	// LoggedInUser is who the account is logged in as, zero until Telegram
	// has said (#208).
	LoggedInUser LoggedInUser
	// HistoryPage is how many messages the owner fetches at a time. It is the
	// host's setting, and a client sizes its history window by it rather than
	// by a setting of its own (#208).
	HistoryPage int
	// Throttle is the flood wait somebody's call ran into that ends last; zero
	// before any, and over once its Until has passed (#212).
	Throttle Throttle
}

// Throttle is a method Telegram asked to wait for, and until when. Nothing
// says it ended: a client counts down to Until and stops there.
type Throttle struct {
	Method string
	Until  time.Time
}

// LoggedInUser is the user an account is logged in as.
type LoggedInUser struct {
	ID       int64
	Username string
	// Name is the user's first and last name, as a command names the
	// author of a message the account sent (#181).
	Name string
}

// AccountDelta carries the account whole: it is small, and a part of it
// means little without the rest.
type AccountDelta struct {
	Contents AccountContents
}

// BuildAccount answers the account window.
func BuildAccount(r Reader) AccountContents { return r.Account() }

// sameAccount reports whether two contents show the same thing. Errors are
// compared by what a client is told of them, which leaves out the cause that
// never crosses to one.
func sameAccount(a, b AccountContents) bool {
	if a.Phase != b.Phase || a.Login != b.Login || a.LoginFailed != b.LoginFailed || a.ClockSkew != b.ClockSkew ||
		a.LoggedInUser != b.LoggedInUser || a.HistoryPage != b.HistoryPage {
		return false
	}
	return sameError(a.Ended, b.Ended)
}

func sameError(a, b *telerr.Error) bool {
	if a == nil || b == nil {
		return a == b
	}
	x, y := *a, *b
	x.Cause, y.Cause = nil, nil
	return x == y
}
