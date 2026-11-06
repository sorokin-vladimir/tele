package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

// update puts new content of the step already asked to the owner, the way the
// login does with a new token: nothing answers it.
func update(o *Owner, req internaltg.AuthRequest) {
	req.Update = true
	go func() { o.authFlow.Requests <- req }()
}

// The QR code is a step of the login like the others, and every client sees
// it (#330).
func TestLogin_TheQRCodeIsAStep(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR})

	step := recvStep(t, a, project.LoginQR, 0)

	assert.Empty(t, step.Token, "no token until one is issued")
}

// A new token is new content of the asking on screen, not a new asking: an
// answer typed while it was replaced still names the step it answers.
func TestLogin_ANewTokenIsTheSameAsking(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR})
	asked := recvStep(t, a, project.LoginQR, 0)
	expires := time.Now().Add(30 * time.Second).Truncate(time.Second)

	update(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR, Token: "tg://login?token=AAAA", Expires: expires})

	got, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Equal(t, asked.Seq, got.Login.Seq)
	assert.Equal(t, project.LoginQR, got.Login.Kind)
	assert.Equal(t, "tg://login?token=AAAA", got.Login.Token)
	assert.True(t, expires.Equal(got.Login.Expires))

	require.NoError(t, a.SwitchLogin(context.Background(), asked.Seq, project.LoginPhone))
	assert.ErrorIs(t, answer(t, answered).Err, internaltg.ErrLoginByPhone)
}

// A token that could not be issued leaves the step with no token and the
// reason in its place.
func TestLogin_ATokenTakenOffSaysWhy(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR})
	recvStep(t, a, project.LoginQR, 0)
	update(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR, Token: "tg://login?token=AAAA"})
	recvAccount(t, a)

	update(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR, Err: "Cannot reach Telegram."})

	got, ok := recvAccount(t, a)
	require.True(t, ok)
	assert.Empty(t, got.Login.Token)
	assert.Equal(t, "Cannot reach Telegram.", got.Login.Err)
}

// Going back to the QR code ends the attempt by number, and with it the
// driver's hold: whoever is looking may log in from the QR code (#211).
func TestLogin_BackToTheQRCodeReleasesTheDriver(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	b := attach(o)
	b.Subscribe(project.AccountWindow{})
	_, _ = recvAccount(t, b)
	drive(t, o, a)
	answered := ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	code := recvStep(t, a, project.LoginCode, 0)

	require.NoError(t, a.SwitchLogin(context.Background(), code.Seq, project.LoginQR))
	assert.ErrorIs(t, answer(t, answered).Err, internaltg.ErrLoginBackToQR)

	answered = ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR})
	qr := recvStep(t, b, project.LoginQR, code.Seq)
	require.NoError(t, b.SwitchLogin(context.Background(), qr.Seq, project.LoginPhone))
	assert.ErrorIs(t, answer(t, answered).Err, internaltg.ErrLoginByPhone)
}

// Only the driver leaves a step it drives, as only it answers one.
func TestLogin_OnlyTheDriverGoesBack(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	b := attach(o)
	drive(t, o, a)
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepCode})
	code := recvStep(t, a, project.LoginCode, 0)

	err := b.SwitchLogin(context.Background(), code.Seq, project.LoginQR)

	assert.Equal(t, telerr.Forbidden, telerr.Of(err))
}

// A switch is to the other way from the step on screen: the QR code leads to
// the number, and every other step back to the QR code. One that does not fit
// the step, or names an asking gone, was pressed at a screen that is no longer
// there.
func TestLogin_ASwitchThatDoesNotFitTheStepIsRefused(t *testing.T) {
	o, a := loginOwner(t, &stubConn{})
	ask(o, internaltg.AuthRequest{Step: internaltg.AuthStepQR})
	qr := recvStep(t, a, project.LoginQR, 0)

	assert.Equal(t, telerr.NotFound, telerr.Of(a.SwitchLogin(context.Background(), qr.Seq, project.LoginQR)))
	assert.Equal(t, telerr.NotFound, telerr.Of(a.SwitchLogin(context.Background(), qr.Seq+1, project.LoginPhone)))
	assert.Equal(t, telerr.NotFound, telerr.Of(a.SwitchLogin(context.Background(), qr.Seq, project.LoginCode)))
}

// Whether a window is there to show a QR code is the host's to say, and the
// login hears it.
func TestOwner_SaysWhetherAWindowIsWatching(t *testing.T) {
	o, _ := loginOwner(t, &stubConn{})
	watched := func() bool {
		select {
		case <-o.authFlow.Watched():
			return true
		default:
			return false
		}
	}
	require.False(t, watched())

	o.SetWatched(true)
	assert.True(t, watched())

	o.SetWatched(false)
	assert.False(t, watched())
}
