package tg

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

type AuthStep int

const (
	AuthStepPhone AuthStep = iota
	AuthStepCode
	AuthStepPassword
	// AuthStepQR shows a QR code for an app already logged in to scan. It has
	// no field: it is answered only by leaving it (#330).
	AuthStepQR
)

// String names the step the way the log does.
func (s AuthStep) String() string {
	switch s {
	case AuthStepQR:
		return "qr"
	case AuthStepPhone:
		return "phone"
	case AuthStepCode:
		return "code"
	case AuthStepPassword:
		return "password"
	default:
		return fmt.Sprintf("step(%d)", int(s))
	}
}

type AuthRequest struct {
	Step AuthStep
	Hint string
	// Err says why the step is asked for again: the value given last time was
	// refused. Empty the first time a step is asked (#285).
	Err string
	// Token is the URL of the login token the QR step shows, and Expires when
	// it stops being good. Empty while there is none to show: no window, or
	// none could be issued, and Err says why (#330).
	Token   string
	Expires time.Time
	// Update says this is the step already asked, with new content - a new
	// token, or none - rather than a new asking.
	Update bool
}

type AuthResponse struct {
	Value string
	Err   error
}

// ErrLoginByPhone answers the QR step: the person logs in by phone number
// instead. ErrLoginBackToQR answers any other step: the person goes back to
// the QR code (#330).
var (
	ErrLoginByPhone  = errors.New("the login turned to the phone number")
	ErrLoginBackToQR = errors.New("the login went back to the QR code")
)

// AuthFlow bridges the login's blocking questions (see login) with bubbletea's
// event loop via unbuffered channels. gotd's auth.Flow no longer drives it
// (#285).
type AuthFlow struct {
	Requests  chan AuthRequest
	Responses chan AuthResponse
	// Errors carries fatal auth errors to the UI (buffered 1 so Code() never blocks).
	Errors chan string
	// log records each step of the login, never what the person typed (#330).
	log *zap.Logger

	// watched is closed while a window is there to show a QR code; a token is
	// issued only then.
	watchMu sync.Mutex
	watched chan struct{}
	// accepted says Telegram reported the token scanned, and ready that the
	// link to Telegram came up. One waiting is as good as ten.
	accepted chan struct{}
	ready    chan struct{}
	// after is how the login waits; tests replace it.
	after func(time.Duration) <-chan time.Time
}

func NewAuthFlow(log *zap.Logger) *AuthFlow {
	return &AuthFlow{
		Requests:  make(chan AuthRequest),
		Responses: make(chan AuthResponse),
		Errors:    make(chan string, 1),
		log:       log,
		watched:   make(chan struct{}),
		accepted:  make(chan struct{}, 1),
		ready:     make(chan struct{}, 1),
		after:     time.After,
	}
}

// SetWatched says whether a window is there to show a QR code.
func (af *AuthFlow) SetWatched(on bool) {
	af.watchMu.Lock()
	defer af.watchMu.Unlock()
	select {
	case <-af.watched:
		if !on {
			af.watched = make(chan struct{})
		}
	default:
		if on {
			close(af.watched)
		}
	}
}

// Watched is closed while a window is there, or once one comes.
func (af *AuthFlow) Watched() <-chan struct{} {
	af.watchMu.Lock()
	defer af.watchMu.Unlock()
	return af.watched
}

func (af *AuthFlow) isWatched() bool {
	select {
	case <-af.Watched():
		return true
	default:
		return false
	}
}

// TokenAccepted is told that Telegram reported the login token scanned. It
// never blocks.
func (af *AuthFlow) TokenAccepted() { signal(af.accepted) }

// LinkReady is told that the link to Telegram came up. It never blocks.
func (af *AuthFlow) LinkReady() { signal(af.ready) }

func signal(c chan struct{}) {
	select {
	case c <- struct{}{}:
	default:
	}
}

// show puts a step, or new content of the step already asked, on the login
// screen without waiting for an answer.
func (af *AuthFlow) show(ctx context.Context, req AuthRequest) error {
	if !req.Update {
		af.log.Info("login step asked", zap.String("step", req.Step.String()), zap.Bool("again", req.Err != ""))
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case af.Requests <- req:
		return nil
	}
}

// request puts one step to the login screen and waits for the answer.
func (af *AuthFlow) request(ctx context.Context, req AuthRequest) (string, error) {
	if err := af.show(ctx, req); err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case resp := <-af.Responses:
		return resp.Value, resp.Err
	}
}

func (af *AuthFlow) Phone(ctx context.Context) (string, error) {
	return af.request(ctx, AuthRequest{Step: AuthStepPhone})
}

func (af *AuthFlow) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return af.askCode(ctx, sentCode, "")
}

// askCode asks for the code Telegram sent, saying how it was sent and, when it
// is asked again, why.
func (af *AuthFlow) askCode(ctx context.Context, sentCode *tg.AuthSentCode, reason string) (string, error) {
	if _, ok := sentCode.Type.(*tg.AuthSentCodeTypeSetUpEmailRequired); ok {
		af.Errors <- "Login requires email verification, which is not yet supported.\nPlease log in via the official Telegram app first, then relaunch tele."
		return "", fmt.Errorf("authSentCodeTypeSetUpEmailRequired: email verification required")
	}
	return af.request(ctx, AuthRequest{Step: AuthStepCode, Hint: codeHint(sentCode.Type), Err: reason})
}

func codeHint(t tg.AuthSentCodeTypeClass) string {
	switch v := t.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "Enter the code from your Telegram app:"
	case *tg.AuthSentCodeTypeEmailCode:
		if v.EmailPattern != "" {
			return fmt.Sprintf("Enter the code sent to %s:", v.EmailPattern)
		}
		return "Enter the code sent to your email:"
	case *tg.AuthSentCodeTypeSMS:
		return "Enter the SMS code:"
	case *tg.AuthSentCodeTypeSMSWord:
		return "Enter the word sent to you via SMS:"
	case *tg.AuthSentCodeTypeSMSPhrase:
		return "Enter the phrase sent to you via SMS:"
	case *tg.AuthSentCodeTypeCall:
		return "Answer the incoming call — the code will be read aloud:"
	case *tg.AuthSentCodeTypeFragmentSMS:
		return "Enter the code from Fragment (fragment.com):"
	default:
		return "Enter the login code:"
	}
}

func (af *AuthFlow) Password(ctx context.Context) (string, error) {
	return af.request(ctx, AuthRequest{Step: AuthStepPassword})
}

func (af *AuthFlow) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported — create account in official Telegram app")
}
