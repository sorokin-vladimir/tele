package tg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// timer is one wait the login started: how long, and the channel the test
// fires to end it.
type timer struct {
	d time.Duration
	c chan time.Time
}

// stoppedClock makes af's waits the test's to end: each one is handed over on
// the returned channel and never ends by itself.
func stoppedClock(af *AuthFlow) <-chan timer {
	timers := make(chan timer, 16)
	af.after = func(d time.Duration) <-chan time.Time {
		t := timer{d: d, c: make(chan time.Time, 1)}
		timers <- t
		return t.c
	}
	return timers
}

// fire ends the next wait the login started, checking how long it was.
func fire(t *testing.T, timers <-chan timer, want time.Duration) {
	t.Helper()
	select {
	case w := <-timers:
		assert.InDelta(t, want.Seconds(), w.d.Seconds(), 1.5, "the wait")
		w.c <- time.Now()
	case <-time.After(2 * time.Second):
		assert.Fail(t, "the login started no wait")
	}
}

// watchedFlow is a login with a window to show its QR code.
func watchedFlow() *AuthFlow {
	af := NewAuthFlow(zap.NewNop())
	af.SetWatched(true)
	return af
}

// The login starts with the QR code, and a scan of it logs in: the step is
// asked first, the token follows as new content of the same step, and the
// update Telegram sends on the scan ends the login (#330).
func TestLogin_AScannedQRCodeLogsIn(t *testing.T) {
	af := watchedFlow()
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 1, client.importN)
	assert.Equal(t, 0, client.sends, "no code is sent")
}

// With no window to show it, no token is issued: a host running as a service
// with nobody looking does not ask Telegram for one every half minute. The
// first window to come starts it.
func TestLogin_NoTokenIsIssuedWithoutAWindow(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		{step: AuthStepQR, then: func() {
			time.Sleep(100 * time.Millisecond)
			assert.Equal(t, 0, client.exported(), "a token with nobody to show it to")
			af.SetWatched(true)
		}},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
}

// A token is replaced before it expires, so the one on screen is always good.
func TestLogin_ATokenIsRenewedBeforeItExpires(t *testing.T) {
	af := watchedFlow()
	timers := stoppedClock(af)
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: func() { fire(t, timers, 25*time.Second) }},
		{step: AuthStepQR, update: true, token: tokenURL(2), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
}

// A renewal that finds the token already accepted logs in, as the update
// would have: the scan is not lost when the update is.
func TestLogin_ATokenFoundAcceptedAtRenewalLogsIn(t *testing.T) {
	af := watchedFlow()
	timers := stoppedClock(af)
	client := &scriptedLogin{accepted: 2}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: func() { fire(t, timers, 25*time.Second) }},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 1, client.importN)
}

// A token that cannot be renewed is taken off the screen, never left there
// expired, and the reason takes its place. The next one is issued once the
// connection is back.
func TestLogin_AnUnreachableRenewalHidesTheQRCode(t *testing.T) {
	af := watchedFlow()
	timers := stoppedClock(af)
	client := &scriptedLogin{exports: []error{nil, &telerr.Error{Kind: telerr.Network, Op: "auth"}}}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: func() { fire(t, timers, 25*time.Second) }},
		{step: AuthStepQR, update: true, reason: reasonQRUnreachable, then: af.LinkReady},
		{step: AuthStepQR, update: true, token: tokenURL(3), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
}

// An unreachable Telegram is tried again after a while even when no
// reconnection is noticed: a call can fail with the link still up.
func TestLogin_AnUnreachableTokenIsTriedAgainLater(t *testing.T) {
	af := watchedFlow()
	timers := stoppedClock(af)
	client := &scriptedLogin{exports: []error{&telerr.Error{Kind: telerr.Network, Op: "auth"}}}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, reason: reasonQRUnreachable, then: func() { fire(t, timers, qrRetry) }},
		{step: AuthStepQR, update: true, token: tokenURL(2), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
}

// A flood wait on the token is waited out, with the reason on screen.
func TestLogin_AFloodWaitHoldsTheNextToken(t *testing.T) {
	af := watchedFlow()
	timers := stoppedClock(af)
	client := &scriptedLogin{exports: []error{refused(420, "FLOOD_WAIT_30")}}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, reason: reasonQRWait, then: func() { fire(t, timers, 30*time.Second) }},
		{step: AuthStepQR, update: true, token: tokenURL(2), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
}

// Any other refusal of the token ends the login like a refusal of the number
// does: asking again meets the same answer.
func TestLogin_AnotherRefusalOfTheTokenEndsTheLogin(t *testing.T) {
	af := watchedFlow()
	client := &scriptedLogin{exports: []error{refused(400, "API_ID_INVALID")}}
	playScreen(t, af, []ask{{step: AuthStepQR}})

	err := af.login(context.Background(), client)

	require.Error(t, err)
	assert.Equal(t, telerr.AppKeyBlocked, telerr.Of(err))
}

// A scan of an account with a 2FA password asks for the password, as a code
// does.
func TestLogin_AScanWithTwoFactorAsksForThePassword(t *testing.T) {
	af := watchedFlow()
	client := &scriptedLogin{imports: []error{refused(401, "SESSION_PASSWORD_NEEDED")}}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: af.TokenAccepted},
		{step: AuthStepPassword, answer: "right"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 1, client.passwords)
}

// The number is one step from the QR code, and the QR code one step back from
// the number.
func TestLogin_BackFromTheNumberReturnsToTheQRCode(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, back: true},
		turnToPhone,
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 1, client.sends)
}

// Leaving the code for the QR code cancels the code: nobody will type it.
func TestLogin_BackFromTheCodeCancelsIt(t *testing.T) {
	af := NewAuthFlow(zap.NewNop())
	client := &scriptedLogin{}
	playScreen(t, af, []ask{
		turnToPhone,
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, back: true},
		turnToPhone,
		{step: AuthStepPhone, answer: "+10000000000"},
		{step: AuthStepCode, answer: "12345"},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, []string{"hash1"}, client.cancelled)
}

// The password step goes back to the QR code too.
func TestLogin_BackFromThePasswordReturnsToTheQRCode(t *testing.T) {
	af := watchedFlow()
	client := &scriptedLogin{imports: []error{refused(401, "SESSION_PASSWORD_NEEDED")}}
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: af.TokenAccepted},
		{step: AuthStepPassword, back: true},
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(2), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), client))
	assert.Equal(t, 2, client.importN)
}

// The QR steps are in the log, and the token never is (#330).
func TestLogin_LogsTheQRCodeWithoutTheToken(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	af := NewAuthFlow(zap.New(core))
	af.SetWatched(true)
	playScreen(t, af, []ask{
		{step: AuthStepQR},
		{step: AuthStepQR, update: true, token: tokenURL(1), then: af.TokenAccepted},
	})

	require.NoError(t, af.login(context.Background(), &scriptedLogin{}))

	issued := logs.FilterMessage("login token issued").AllUntimed()
	require.Len(t, issued, 1)
	assert.Equal(t, zapcore.InfoLevel, issued[0].Level)
	assert.Contains(t, issued[0].ContextMap(), "expires_in")
	assert.Equal(t, 1, logs.FilterMessage("login token accepted").Len())
	for _, e := range logs.AllUntimed() {
		line := fmt.Sprint(e.Message, " ", e.ContextMap())
		assert.NotContains(t, line, tokenN(1).String())
	}
}
