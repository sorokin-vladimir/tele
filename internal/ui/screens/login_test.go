package screens_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogin_AuthRequest_UpdatesPrompt(t *testing.T) {
	af := internaltg.NewAuthFlow()
	m := screens.NewLoginModel(af)
	newM, _ := m.Update(screens.AuthRequestMsg{Step: internaltg.AuthStepPhone})
	lm := newM.(screens.LoginModel)
	assert.Equal(t, internaltg.AuthStepPhone, lm.CurrentStep())
	assert.Contains(t, lm.View().Content, "phone")
}

func TestLogin_Connected_EmitsTransition(t *testing.T) {
	af := internaltg.NewAuthFlow()
	m := screens.NewLoginModel(af)
	_, cmd := m.Update(screens.ConnectedMsg{})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.IsType(t, screens.TransitionToMainMsg{}, msg)
}

func TestLogin_InitialView_ShowsConnecting(t *testing.T) {
	af := internaltg.NewAuthFlow()
	m := screens.NewLoginModel(af)
	assert.Contains(t, m.View().Content, "onnect") // "Connecting..." or "Connect"
}

// The slow mark belongs to the wait for Telegram's first answer. Once a prompt
// is up the wait is over, and a tick that arrives late must not bring it back.
func TestLogin_SlowConnectMarksOnlyTheWait(t *testing.T) {
	m := screens.NewLoginModel(internaltg.NewAuthFlow())
	assert.False(t, m.Slow())

	slow, _ := m.Update(screens.SlowConnectMsg{})
	assert.True(t, slow.(screens.LoginModel).Slow())

	prompted, _ := m.Update(screens.AuthRequestMsg{Step: internaltg.AuthStepPhone})
	late, _ := prompted.(screens.LoginModel).Update(screens.SlowConnectMsg{})
	assert.False(t, late.(screens.LoginModel).Slow())
}

// A prompt arriving after the mark ends the wait too.
func TestLogin_PromptEndsTheSlowWait(t *testing.T) {
	m := screens.NewLoginModel(internaltg.NewAuthFlow())
	slow, _ := m.Update(screens.SlowConnectMsg{})

	prompted, _ := slow.(screens.LoginModel).Update(screens.AuthRequestMsg{Step: internaltg.AuthStepPhone})

	assert.False(t, prompted.(screens.LoginModel).Slow())
}

// A step asked again says why under the field, and the reason goes away with
// the next step that has none (#285).
func TestLogin_ReasonShowsUnderTheField(t *testing.T) {
	m := screens.NewLoginModel(internaltg.NewAuthFlow())

	again, _ := m.Update(screens.AuthRequestMsg{Step: internaltg.AuthStepCode, Err: "That code is not right. Try again."})
	assert.Contains(t, again.(screens.LoginModel).View().Content, "That code is not right. Try again.")

	onward, _ := again.(screens.LoginModel).Update(screens.AuthRequestMsg{Step: internaltg.AuthStepPassword})
	assert.NotContains(t, onward.(screens.LoginModel).View().Content, "not right")
}

// ensure tea import is used (Blink cmd returns tea.Cmd)
var _ tea.Cmd = screens.NewLoginModel(nil).Init()
