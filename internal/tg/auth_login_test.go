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

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// scriptedLogin answers each call with the next error in its script, and
// succeeds once the script runs out.
type scriptedLogin struct {
	sendCode, signIn, password []error
	sends, signIns, passwords  int
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
	return &tg.AuthSentCode{Type: &tg.AuthSentCodeTypeApp{}, PhoneCodeHash: fmt.Sprint("hash", s.sends)}, nil
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
	af := NewAuthFlow()
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

// An expired code is not the person's mistake: a new one is sent and asked for.
func TestLogin_ExpiredCodeSendsANewOne(t *testing.T) {
	af := NewAuthFlow()
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
	af := NewAuthFlow()
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
	af := NewAuthFlow()
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
	af := NewAuthFlow()
	client := &scriptedLogin{signIn: []error{refused(420, "FLOOD_WAIT")}}
	playScreen(t, af, []ask{
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	err := af.login(context.Background(), client)

	require.Error(t, err)
	assert.Equal(t, telerr.RateLimited, telerr.Of(err))
}
