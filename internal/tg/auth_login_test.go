package tg

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// scriptedLogin answers each call with the next error in its script, and
// succeeds once the script runs out.
type scriptedLogin struct {
	sendCode, signIn, password []error
	sends, signIns, passwords  int
	// next and timeout are what each sent code says about the next way and
	// when it may be asked for; none by default.
	next    tg.AuthCodeTypeClass
	timeout int
}

func next(script []error, call int) error {
	if call < len(script) {
		return script[call]
	}
	return nil
}

func (s *scriptedLogin) SendCode(context.Context, string, auth.SendCodeOptions) (tg.AuthSentCodeClass, error) {
	s.sends++
	if err := next(s.sendCode, s.sends-1); err != nil {
		return nil, err
	}
	sent := &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{}, PhoneCodeHash: fmt.Sprint("hash", s.sends)}
	if s.next != nil {
		sent.SetNextType(s.next)
	}
	if s.timeout != 0 {
		sent.SetTimeout(s.timeout)
	}
	return sent, nil
}

func (s *scriptedLogin) SignIn(context.Context, string, string, string) (*tg.AuthAuthorization, error) {
	s.signIns++
	if err := next(s.signIn, s.signIns-1); err != nil {
		return nil, err
	}
	return &tg.AuthAuthorization{}, nil
}

func (s *scriptedLogin) Password(context.Context, string) (*tg.AuthAuthorization, error) {
	s.passwords++
	if err := next(s.password, s.passwords-1); err != nil {
		return nil, err
	}
	return &tg.AuthAuthorization{}, nil
}

func (s *scriptedLogin) SignUp(context.Context, auth.SignUp) (*tg.AuthAuthorization, error) {
	return nil, errors.New("sign up is not scripted")
}

// refused is a Telegram error as it reaches the login: gotd's own wrap around
// the error our middleware already mapped.
func refused(code int, typ string) error {
	mapped := (&GotdClient{}).mapError("auth", tgerr.New(code, typ))
	return fmt.Errorf("sign in: %w", mapped)
}

// ask is one request the login screen expects, and what the person types.
type ask struct {
	step   AuthStep
	reason string
	answer string
	// abandon answers the step with ErrLoginAbandoned rather than a value: the
	// client answering the login went away.
	abandon bool
}

// playScreen answers the login's requests the way the login screen would, and
// checks each one is the step and the reason expected.
func playScreen(t *testing.T, af *AuthFlow, asks []ask) {
	t.Helper()
	go func() {
		for _, a := range asks {
			select {
			case req := <-af.Requests:
				assert.Equal(t, a.step, req.Step)
				assert.Equal(t, a.reason, req.Err)
				if a.abandon {
					af.Responses <- AuthResponse{Err: ErrLoginAbandoned}
					continue
				}
				af.Responses <- AuthResponse{Value: a.answer}
			case <-time.After(2 * time.Second):
				assert.Fail(t, "login stopped asking", "expected step %d", a.step)
				return
			}
		}
	}()
}

// A mistyped code is asked for again, with the reason, and no new code is sent:
// the one on the person's phone is still good (#285).
func TestLogin_MistypedCodeIsAskedAgain(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{signIn: []error{refused(400, "PHONE_CODE_INVALID")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "11111"},
		{step: AuthStepCode, reason: reasonCodeInvalid, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 1, client.sends)
	assert.Equal(t, 2, client.signIns)
}

// A login left in the middle starts again from the number, saying why, rather
// than ending the account: whoever answers next starts fresh instead of
// inheriting a code they never saw (#211).
func TestLogin_AnAbandonedLoginStartsAgainFromTheNumber(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, abandon: true},
		{step: AuthStepPhone, reason: reasonAbandoned, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 2, client.sends, "the new attempt asks for a code of its own")
	assert.Equal(t, 1, client.signIns)
}

// An expired code is not the person's mistake: a new one is sent and asked for.
func TestLogin_ExpiredCodeSendsANewOne(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{signIn: []error{refused(400, "PHONE_CODE_EXPIRED")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "11111"},
		{step: AuthStepCode, reason: reasonCodeExpired, answer: "22222"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 2, client.sends)
}

func TestLogin_InvalidNumberIsAskedAgain(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{sendCode: []error{refused(400, "PHONE_NUMBER_INVALID")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "12"},
		{step: AuthStepPhone, reason: reasonNumberInvalid, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 2, client.sends)
}

// gotd turns a wrong 2FA password into its own sentinel, so that is what marks
// it rather than Telegram's error type.
func TestLogin_WrongPasswordIsAskedAgain(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{
		signIn:   []error{auth.ErrPasswordAuthNeeded},
		password: []error{auth.ErrPasswordInvalid},
	}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
		{step: AuthStepPassword, answer: "wrong"},
		{step: AuthStepPassword, reason: reasonPasswordInvalid, answer: "right"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 2, client.passwords)
}

// Anything that is not the person's typing ends the login and reaches the error
// screen with its kind intact. A flood wait is Telegram's own limit on guesses,
// which is why the login sets none of its own.
func TestLogin_OtherRefusalEndsTheLogin(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{signIn: []error{refused(420, "FLOOD_WAIT")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	err := af.login(context.Background(), client)

	require.Error(t, err)
	assert.Equal(t, telerr.RateLimited, telerr.Of(err))
}

// Too many attempts on a number carry no wait to count down, so the number is
// asked for again with the reason: the person may try later without a restart
// (#254). One is a 400 and the other a 406, which is why neither is a kind.
func TestLogin_TooManyAttemptsAsksForTheNumberAgain(t *testing.T) {
	for _, flood := range []error{refused(400, "PHONE_NUMBER_FLOOD"), refused(406, "PHONE_PASSWORD_FLOOD")} {
		af := NewAuthFlow(zap.NewNop())
		client := &scriptedLogin{sendCode: []error{flood}}
		playScreen(t, af, []ask{
			{step: AuthStepPhone, answer: "+10000000000"},
			{step: AuthStepPhone, reason: reasonTooManyAttempts, answer: "+10000000000"},
			{step: AuthStepCode, answer: "12345"},
		})

		require.NoError(t, af.login(context.Background(), client))
		assert.Equal(t, 2, client.sends)
	}
}

// A banned number is not the person's typing: asking again meets the same ban,
// so the login ends with the kind the login screen names it by (#254).
func TestLogin_BannedNumberEndsTheLogin(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{sendCode: []error{refused(400, "PHONE_NUMBER_BANNED")}}
	playScreen(t, af, []ask{{step: AuthStepPhone, answer: "+10000000000"}})

	err := af.login(context.Background(), client)

	require.Error(t, err)
	assert.Equal(t, telerr.AccountBanned, telerr.Of(err))
}

// scriptedSelf answers the question "who is logged in" with the next error in
// its script, and with the user once the script runs out.
type scriptedSelf struct {
	errs  []error
	calls int
}

func (s *scriptedSelf) self(context.Context) (*tg.User, error) {
	s.calls++
	if err := next(s.errs, s.calls-1); err != nil {
		return nil, err
	}
	return &tg.User{ID: 42}, nil
}

func TestAuthorize_ALoggedInSessionSkipsTheLogin(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	self := &scriptedSelf{}
	client := &scriptedLogin{}

	user, err := af.authorize(context.Background(), self.self, client, true)

	require.NoError(t, err)
	assert.Equal(t, int64(42), user.ID)
	assert.Zero(t, client.sends)
}

// A session found logged out ends its account rather than logging in on its
// key: a login would bind to the same key, and the account's files would pass
// to whoever logged in (#297). What it ends with says how it was logged out,
// which is what the person is told on the way back to the login.
func TestAuthorize_ALogOutEndsTheAccountRatherThanLoggingIn(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		logOut telerr.LogOut
	}{
		{"terminated elsewhere", refused(401, "AUTH_KEY_UNREGISTERED"), telerr.LogOutElsewhere},
		{"revoked", refused(401, "SESSION_REVOKED"), telerr.LogOutElsewhere},
		{"telegram account deleted", refused(401, "USER_DEACTIVATED"), telerr.LogOutDeleted},
		{"key dropped", refused(406, "AUTH_KEY_DUPLICATED"), telerr.LogOutKeyDropped},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			af := NewAuthFlow(zap.NewNop())
			client := &scriptedLogin{}

			_, err := af.authorize(context.Background(), (&scriptedSelf{errs: []error{tt.err}}).self, client, true)

			e, ok := telerr.As(err)
			require.True(t, ok)
			assert.Equal(t, telerr.Unauthorized, e.Kind)
			assert.Equal(t, tt.logOut, e.LogOut)
			assert.Zero(t, client.sends, "no login on the old key")
		})
	}
}

// With no session there was nothing to log out of: a first start logs in.
func TestAuthorize_NoSessionLogsInWithoutACause(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	self := &scriptedSelf{errs: []error{refused(401, "AUTH_KEY_UNREGISTERED")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	_, err := af.authorize(context.Background(), self.self, &scriptedLogin{}, false)

	require.NoError(t, err)
}

// A banned account is offered no login: gotd's own status check read the ban
// as "not logged in" and walked the person through number and code into the
// same ban (#254).
func TestAuthorize_ABannedAccountIsOfferedNoLogin(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	self := &scriptedSelf{errs: []error{refused(401, "USER_DEACTIVATED_BAN")}}
	client := &scriptedLogin{}

	_, err := af.authorize(context.Background(), self.self, client, true)

	require.Error(t, err)
	assert.Equal(t, telerr.AccountBanned, telerr.Of(err))
	assert.Zero(t, client.sends)
}
