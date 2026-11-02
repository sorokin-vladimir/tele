package tg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// The login is our own loop rather than gotd's auth.Flow, which asks for each
// value once and gives up on the first refusal: a mistyped code ended the login,
// and the screen went on taking input nobody read (#285). Here what the person
// got wrong is asked for again, with the reason, and only a refusal that is not
// theirs to fix ends it. Telegram limits guesses itself with a flood wait, which
// ends the login like any other refusal, so no limit is kept here.

// The reasons a step is asked for again, shown under the field.
const (
	reasonNumberInvalid   = "That number is not valid. Include the country code, e.g. +44..."
	reasonTooManyAttempts = "Too many login attempts for this number. Try again later."
	reasonCodeInvalid     = "That code is not right. Try again."
	reasonCodeExpired     = "That code has expired. A new one is on its way."
	reasonPasswordInvalid = "That password is not right. Try again."
	reasonAbandoned       = "The login was left unfinished in another window. Start again."
)

// The reasons the QR step shows no QR code, in its place.
const (
	reasonQRUnreachable = "Cannot reach Telegram. The QR code will appear once the connection is back."
	reasonQRWait        = "Telegram asks to wait before a new QR code. It will appear by itself."
)

const (
	// qrRenewAhead is how long before a token expires the next one is asked
	// for, so the one on screen is always good while it is replaced.
	qrRenewAhead = 5 * time.Second
	// qrRetry is how long a token Telegram could not be reached for waits to
	// be asked for again, when the link is not seen coming back first.
	qrRetry = 10 * time.Second
)

// ErrLoginAbandoned answers a step when the client answering the login went
// away in the middle of it. The login starts again from the QR code: a code is
// short-lived, and whoever answers next must not inherit one they never saw
// (#211).
var ErrLoginAbandoned = errors.New("the login was abandoned")

// loginClient is what the login asks Telegram: gotd's calls for the number,
// the code and the password, and the ones for the QR code it has no client
// for.
type loginClient interface {
	auth.FlowClient
	// ExportLoginToken issues a login token to show; an empty one means the
	// last one was accepted, and ImportLoginToken finishes the login.
	ExportLoginToken(ctx context.Context) (qrlogin.Token, error)
	ImportLoginToken(ctx context.Context) error
	// CancelCode tells Telegram the code sent will not be typed.
	CancelCode(ctx context.Context, phone, hash string) error
}

// authorize returns the logged-in user, logging in first when there is no
// session to speak of.
//
// It stands in for gotd's auth.Client.Status, which reads every 401 as "not
// logged in": a banned account was walked through number and code only to meet
// the same ban (#254). Here the mapped kind decides. A session that turns out
// to be logged out is not logged in on either: the key in it survives the log
// out, a login would bind to it, and the account's files would pass to whoever
// logged in. The error goes up instead, and the host ends the account and
// starts the next one with no session (#297). Only a start with no session -
// the first, or the one after an account ended - leads into the login.
func (af *AuthFlow) authorize(ctx context.Context, self func(context.Context) (*tg.User, error),
	client loginClient, hadSession bool) (*tg.User, error) {
	user, err := self(ctx)
	if telerr.Of(err) != telerr.Unauthorized || hadSession {
		return user, err
	}
	if err := af.login(ctx, client); err != nil {
		return nil, err
	}
	return self(ctx)
}

// login logs in through client, asking the person through af. It starts at
// the QR code, and the phone number is one step from it either way. An attempt
// abandoned in the middle starts again from the QR code, saying why.
func (af *AuthFlow) login(ctx context.Context, client loginClient) error {
	reason := ""
	byPhone := false
	for {
		var err error
		if byPhone {
			err = af.attempt(ctx, client)
		} else {
			err = af.qr(ctx, client, reason)
		}
		reason = ""
		switch {
		case errors.Is(err, ErrLoginByPhone):
			byPhone = true
		case errors.Is(err, ErrLoginBackToQR):
			byPhone = false
		case errors.Is(err, ErrLoginAbandoned):
			byPhone, reason = false, reasonAbandoned
		default:
			return err
		}
	}
}

// qr shows login tokens until one is scanned, the person turns to the number,
// or the attempt is abandoned; reason is why the step is asked, empty the
// first time. A token is issued only while a window is there to show it, and
// one that could not be replaced in time is taken off the screen rather than
// left there expired (#330).
func (af *AuthFlow) qr(ctx context.Context, client loginClient, reason string) error {
	if err := af.show(ctx, AuthRequest{Step: AuthStepQR, Err: reason}); err != nil {
		return err
	}
	var (
		// issue is closed when a token is to be issued: set while one is
		// wanted, it waits for a window.
		issue = af.whenWatched()
		// renew ends when the token on screen is to be replaced, or when a
		// token that could not be issued is to be tried again.
		renew <-chan time.Time
		// shownUntil is when the token on screen expires; zero with none.
		shownUntil time.Time
		// unreachable is set while Telegram could not be reached, so the
		// link coming back is a reason to try at once.
		unreachable bool
	)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case resp := <-af.Responses:
			if resp.Err != nil {
				return resp.Err
			}
			// The step has no field, so a value answers nothing.
		case <-af.accepted:
			return af.importToken(ctx, client)
		case <-af.ready:
			if unreachable {
				renew, issue = nil, af.whenWatched()
			}
		case <-renew:
			renew = nil
			if !af.isWatched() && !shownUntil.IsZero() {
				// Nobody to show the next one to: the one on screen is taken
				// off before it expires there.
				shownUntil = time.Time{}
				if err := af.show(ctx, AuthRequest{Step: AuthStepQR, Update: true}); err != nil {
					return err
				}
			}
			issue = af.whenWatched()
		case <-issue:
			issue = nil
			token, err := af.export(ctx, client, shownUntil)
			if err == nil && token.Empty() {
				return af.importToken(ctx, client)
			}
			if err == nil {
				renewIn := tokenRenewal(token)
				shownUntil, unreachable = time.Now().Add(renewIn+qrRenewAhead), false
				renew = af.after(renewIn)
				af.log.Info("login token issued", zap.Duration("expires_in", renewIn+qrRenewAhead))
				err := af.show(ctx, AuthRequest{Step: AuthStepQR, Update: true, Token: token.URL(), Expires: shownUntil})
				if err != nil {
					return err
				}
				continue
			}
			af.refused(AuthStepQR, err)
			var why string
			switch e, _ := telerr.As(err); {
			case telerr.Of(err) == telerr.RateLimited && e != nil:
				why, unreachable, renew = reasonQRWait, false, af.after(e.RetryAfter)
			case telerr.Of(err) == telerr.Network, errors.Is(err, context.DeadlineExceeded):
				why, unreachable, renew = reasonQRUnreachable, true, af.after(qrRetry)
			default:
				return fmt.Errorf("export login token: %w", err)
			}
			shownUntil = time.Time{}
			if err := af.show(ctx, AuthRequest{Step: AuthStepQR, Update: true, Err: why}); err != nil {
				return err
			}
		}
	}
}

// export issues a login token. While one is on screen, Telegram has until it
// expires to answer: past that, the one on screen is no good, and the call is
// taken as Telegram unreachable.
func (af *AuthFlow) export(ctx context.Context, client loginClient, shownUntil time.Time) (qrlogin.Token, error) {
	if !shownUntil.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, shownUntil)
		defer cancel()
	}
	return client.ExportLoginToken(ctx)
}

// tokenRenewal is how long until the token is to be replaced. The expiry is
// Telegram's clock read against ours, so it is kept within reason: a skewed
// clock must neither replace a token at once nor leave one on screen long
// after it expired (#277).
func tokenRenewal(token qrlogin.Token) time.Duration {
	d := time.Until(token.Expires()) - qrRenewAhead
	return min(max(d, time.Second), time.Minute)
}

// importToken finishes a login whose token was scanned, asking for the 2FA
// password when the account has one.
func (af *AuthFlow) importToken(ctx context.Context, client loginClient) error {
	af.log.Info("login token accepted")
	err := client.ImportLoginToken(ctx)
	switch {
	case err == nil:
		return nil
	case tgerr.Is(err, "SESSION_PASSWORD_NEEDED"):
		return af.password(ctx, client)
	default:
		af.refused(AuthStepQR, err)
		return fmt.Errorf("import login token: %w", err)
	}
}

// attempt is one attempt at the login by phone number.
func (af *AuthFlow) attempt(ctx context.Context, client loginClient) error {
	phone, sent, err := af.sendCode(ctx, client, "")
	if err != nil {
		return err
	}
	switch s := sent.(type) {
	case *tg.AuthSentCode:
		return af.signIn(ctx, client, phone, s)
	case *tg.AuthSentCodeSuccess:
		// Telegram let the session in without a code, as gotd's flow allows.
		switch a := s.Authorization.(type) {
		case *tg.AuthAuthorization:
			return nil
		case *tg.AuthAuthorizationSignUpRequired:
			_, err := af.SignUp(ctx)
			return err
		default:
			return fmt.Errorf("unexpected authorization type: %T", a)
		}
	default:
		return fmt.Errorf("unexpected sent code type: %T", sent)
	}
}

// sendCode asks for the number until Telegram accepts it and sends a code.
func (af *AuthFlow) sendCode(ctx context.Context, client auth.FlowClient, reason string) (string, tg.AuthSentCodeClass, error) {
	for {
		phone, err := af.request(ctx, AuthRequest{Step: AuthStepPhone, Err: reason})
		if err != nil {
			return "", nil, fmt.Errorf("get phone: %w", err)
		}
		sent, err := client.SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			af.refused(AuthStepPhone, err)
		}
		if tgerr.Is(err, "PHONE_NUMBER_INVALID") {
			reason = reasonNumberInvalid
			continue
		}
		// Telegram's limit on attempts carries no wait to count down, so the
		// number is asked for again rather than the login ended: the person can
		// try later without a restart (#254).
		if tgerr.Is(err, "PHONE_NUMBER_FLOOD", "PHONE_PASSWORD_FLOOD") {
			reason = reasonTooManyAttempts
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("send code: %w", err)
		}
		af.codeSent(sent)
		return phone, sent, nil
	}
}

// codeSent records how Telegram sent the code and how it would send the next
// one, which is what a report of a code that never came turns on (#330).
func (af *AuthFlow) codeSent(sent tg.AuthSentCodeClass) {
	s, ok := sent.(*tg.AuthSentCode)
	if !ok {
		af.log.Info("login needs no code", zap.String("type", sent.TypeName()))
		return
	}
	next := "none"
	if t, ok := s.GetNextType(); ok {
		next = t.TypeName()
	}
	timeout, _ := s.GetTimeout()
	af.log.Info("login code sent",
		zap.String("type", s.Type.TypeName()),
		zap.String("next_type", next),
		zap.Int("timeout", timeout))
}

// refused records what Telegram, or gotd for it, refused at step: the error
// and, when it is Telegram's own, its type and code. The error never carries
// what the person typed.
func (af *AuthFlow) refused(step AuthStep, err error) {
	fields := []zap.Field{zap.String("step", step.String()), zap.Error(err)}
	if rpc, ok := tgerr.As(err); ok {
		fields = append(fields, zap.String("rpc_error", rpc.Type), zap.Int("rpc_code", rpc.Code))
	}
	af.log.Info("login refused", fields...)
}

// signIn asks for the code until Telegram accepts it, sending a new one when
// the one asked about has expired.
func (af *AuthFlow) signIn(ctx context.Context, client loginClient, phone string, sent *tg.AuthSentCode) error {
	reason := ""
	for {
		code, err := af.askCode(ctx, sent, reason)
		if errors.Is(err, ErrLoginBackToQR) {
			// Nobody will type the code now, so Telegram is told: what it
			// answers changes nothing for the person.
			if err := client.CancelCode(ctx, phone, sent.PhoneCodeHash); err != nil {
				af.log.Info("login code not cancelled", zap.Error(err))
			}
		}
		if err != nil {
			return fmt.Errorf("get code: %w", err)
		}
		_, err = client.SignIn(ctx, phone, code, sent.PhoneCodeHash)
		var signUp *auth.SignUpRequired
		if err != nil && !errors.Is(err, auth.ErrPasswordAuthNeeded) && !errors.As(err, &signUp) {
			af.refused(AuthStepCode, err)
		}
		switch {
		case err == nil:
			return nil
		case errors.Is(err, auth.ErrPasswordAuthNeeded):
			return af.password(ctx, client)
		case tgerr.Is(err, "PHONE_CODE_INVALID"):
			reason = reasonCodeInvalid
		case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
			again, err := client.SendCode(ctx, phone, auth.SendCodeOptions{})
			if err != nil {
				af.refused(AuthStepCode, err)
				return fmt.Errorf("send code again: %w", err)
			}
			af.codeSent(again)
			next, ok := again.(*tg.AuthSentCode)
			if !ok {
				return fmt.Errorf("unexpected sent code type: %T", again)
			}
			sent, reason = next, reasonCodeExpired
		case errors.As(err, &signUp):
			_, err := af.SignUp(ctx)
			return err
		default:
			return fmt.Errorf("sign in: %w", err)
		}
	}
}

// password asks for the 2FA password until Telegram accepts it. gotd reports a
// wrong one as its own auth.ErrPasswordInvalid rather than Telegram's error.
func (af *AuthFlow) password(ctx context.Context, client auth.FlowClient) error {
	reason := ""
	for {
		pw, err := af.request(ctx, AuthRequest{Step: AuthStepPassword, Err: reason})
		if err != nil {
			return fmt.Errorf("get password: %w", err)
		}
		_, err = client.Password(ctx, pw)
		if err != nil {
			af.refused(AuthStepPassword, err)
		}
		if errors.Is(err, auth.ErrPasswordInvalid) {
			reason = reasonPasswordInvalid
			continue
		}
		if err != nil {
			return fmt.Errorf("check password: %w", err)
		}
		return nil
	}
}
