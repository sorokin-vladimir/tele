package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ui"
)

// A skewed clock is a known cause, so the connecting screen names it the moment
// it is found rather than after the wait that suggests causes (#277).
func TestLogin_ClockSkewIsNamedAtOnce(t *testing.T) {
	view := withLoginStep(t, sizedLoginRoot(t), core.ClockSkew{Skew: 8 * time.Minute}).View().Content

	assert.Contains(t, view, "Your clock is 8 minutes ahead of Telegram's.")
	assert.Contains(t, view, "Telegram refuses every message until it is set right.")
	assert.Contains(t, view, testLogPath)
	assert.Contains(t, view, "ctrl+c to quit")
	assert.NotContains(t, view, "connecting...")
}

func TestLogin_ClockBehindIsSaidInSeconds(t *testing.T) {
	view := withLoginStep(t, sizedLoginRoot(t), core.ClockSkew{Skew: -45 * time.Second}).View().Content

	assert.Contains(t, view, "Your clock is 45 seconds behind Telegram's.")
}

// Setting the clock right lets the same attempt through, so the screen goes
// back to connecting rather than staying on a cause that is gone.
func TestLogin_ClockSetRightGoesBackToConnecting(t *testing.T) {
	m := withLoginStep(t, sizedLoginRoot(t), core.ClockSkew{Skew: 8 * time.Minute})

	view := withLoginStep(t, m, core.ClockSkew{}).View().Content

	assert.Contains(t, view, "connecting...")
	assert.NotContains(t, view, "Your clock")
}

// After login the same skew stops the chat from updating with nothing else to
// show for it. It stays in the status bar, through key presses that clear the
// ordinary status, until messages get through again.
func TestMain_ClockSkewStaysInTheStatusBar(t *testing.T) {
	m := ui.NewRootModel(nil, 50, false).WithScreen(ui.ScreenMain)
	m = withLoginStep(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = withLoginStep(t, m, core.ClockSkew{Skew: 8 * time.Minute})

	assert.Contains(t, m.View().Content, "clock 8m ahead of Telegram")

	m = withLoginStep(t, m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	assert.Contains(t, m.View().Content, "clock 8m ahead of Telegram")

	m = withLoginStep(t, m, core.ClockSkew{})
	assert.NotContains(t, m.View().Content, "clock 8m ahead")
}
