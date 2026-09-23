package ui_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

const testLogPath = "/state/tele.log"

func sizedLoginRoot(t *testing.T) ui.RootModel {
	t.Helper()
	m := newLoginRoot().WithLogPath(testLogPath)
	return withLoginStep(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
}

// A failure to connect used to wait, unread, until the program exited - which
// nobody could make it do. It now lands on the login screen with what went
// wrong, the text to quote in a report, the log to look in and the way out
// (#283).
func TestLogin_ConnectFailureShowsCauseLogAndQuit(t *testing.T) {
	err := &telerr.Error{Kind: telerr.Network, Op: "dial", Detail: "connection refused"}

	view := withLoginStep(t, sizedLoginRoot(t), ui.ConnectFailedMsg{Err: err}).View().Content

	assert.Contains(t, view, "Could not connect to Telegram: no connection")
	assert.Contains(t, view, err.Error())
	assert.Contains(t, view, testLogPath)
	assert.Contains(t, view, "ctrl+c to quit")
	assert.NotContains(t, view, "connecting...")
}

// An error from the transport is not a domain error: it has no kind to name,
// so its own text is the only thing there is to say, and it is said once.
func TestLogin_ConnectFailureWithoutKindKeepsItsText(t *testing.T) {
	err := errors.New("dial tcp 149.154.167.50:443: connect: connection refused")

	view := withLoginStep(t, sizedLoginRoot(t), ui.ConnectFailedMsg{Err: err}).View().Content

	assert.Contains(t, view, "Could not connect to Telegram")
	assert.Contains(t, view, "connection refused")
}

// Quitting cancels the connection, and the cancellation is not news.
func TestLogin_CancelledConnectShowsNothing(t *testing.T) {
	err := fmt.Errorf("run: %w", context.Canceled)

	view := withLoginStep(t, sizedLoginRoot(t), ui.ConnectFailedMsg{Err: err}).View().Content

	assert.Contains(t, view, "connecting...")
}

// The error step used to be drawn as "connecting...", because the view asked
// whether the step was below zero and the error step is too. The one message
// that did reach the screen was never seen.
func TestLogin_AuthErrorIsDrawn(t *testing.T) {
	m := withLoginStep(t, sizedLoginRoot(t), screens.AuthErrorMsg{Text: "Login requires email verification"})

	view := m.View().Content

	assert.Contains(t, view, "Login requires email verification")
	assert.Contains(t, view, testLogPath)
	assert.NotContains(t, view, "connecting...")
}

// The hint names a binding that works on this screen, so a rebound quit is the
// one it shows.
func TestLogin_QuitHintFollowsTheKeyMap(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"q", "alt+x"}},
	})
	require.Empty(t, warns)
	m := sizedLoginRoot(t).WithKeyMap(km)

	view := withLoginStep(t, m, ui.ConnectFailedMsg{Err: errors.New("boom")}).View().Content

	assert.Contains(t, view, "alt+x to quit")
}

// After login the connection can still end for good. That used to be as silent
// as it was on the login screen.
func TestMain_ConnectFailureRaisesAToast(t *testing.T) {
	m := ui.NewRootModel(50, false).WithScreen(ui.ScreenMain)
	m = withLoginStep(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	next, _ := m.Update(ui.ConnectFailedMsg{Err: &telerr.Error{Kind: telerr.Network}})
	root := next.(ui.RootModel)
	root.SettleToastsForTest()

	// The toast wraps, so only the first line is asserted as one piece.
	assert.Contains(t, root.View().Content, "Could not connect to Telegram")
}
