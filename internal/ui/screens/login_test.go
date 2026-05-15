package screens_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
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

// ensure tea import is used (Blink cmd returns tea.Cmd)
var _ tea.Cmd = screens.NewLoginModel(nil).Init()
