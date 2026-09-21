package ui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// A connection that is merely slow is not a failure, so nothing is given up. The
// screen stops saying only "connecting..." and says what is worth checking,
// where the log is and how to leave (#283).
func TestLogin_StillConnectingSaysWhatToCheck(t *testing.T) {
	view := withLoginStep(t, sizedLoginRoot(t), screens.SlowConnectMsg{}).View().Content

	assert.Contains(t, view, "still connecting...")
	assert.Contains(t, view, "Check your network, proxy settings and system clock.")
	assert.Contains(t, view, testLogPath)
	assert.Contains(t, view, "ctrl+c to quit")
}

func TestLogin_ConnectingBeforeTheThresholdIsPlain(t *testing.T) {
	view := sizedLoginRoot(t).View().Content

	assert.Contains(t, view, "connecting...")
	assert.NotContains(t, view, "still connecting")
	assert.NotContains(t, view, "proxy")
}
