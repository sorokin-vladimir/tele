package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core/project"
)

// throttledRoot is the main screen told that a call ran into a flood wait
// ending at until.
func throttledRoot(t *testing.T, until time.Time) RootModel {
	t.Helper()
	m := NewRootModel(false).WithScreen(ScreenMain)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(RootModel)
	next, _ = m.Update(project.Delta{Account: &project.AccountDelta{
		Contents: project.AccountContents{Throttle: project.Throttle{Method: "messages.getHistory", Until: until}},
	}})
	return next.(RootModel)
}

func tickAt(t *testing.T, m RootModel, now time.Time) RootModel {
	t.Helper()
	next, _ := m.Update(throttleTickMsg{now: now})
	return next.(RootModel)
}

// A flood wait somebody's call ran into is said in the status bar with how
// long is left, so the window does not look frozen for it (#212).
func TestThrottle_TheStatusBarCountsTheWaitDown(t *testing.T) {
	until := time.Now().Add(32*time.Second + 500*time.Millisecond)
	m := throttledRoot(t, until)

	assert.Contains(t, xansi.Strip(m.View().Content), "Telegram asks to wait 33s")

	m = tickAt(t, m, until.Add(-10*time.Second))
	assert.Contains(t, xansi.Strip(m.View().Content), "Telegram asks to wait 10s")

	m = tickAt(t, m, until.Add(-(9*time.Minute + 12*time.Second)))
	assert.Contains(t, xansi.Strip(m.View().Content), "Telegram asks to wait 9m12s")
}

// Nothing says a wait ended: once its time has come, it is gone.
func TestThrottle_AWaitThatIsOverLeavesTheStatusBar(t *testing.T) {
	until := time.Now().Add(5 * time.Second)
	m := throttledRoot(t, until)

	m = tickAt(t, m, until)

	assert.NotContains(t, xansi.Strip(m.View().Content), "Telegram asks to wait")
}

// A wait already over when it is told, as for a window attaching late, never
// shows.
func TestThrottle_AWaitOverWhenToldNeverShows(t *testing.T) {
	m := throttledRoot(t, time.Now().Add(-time.Minute))

	assert.NotContains(t, xansi.Strip(m.View().Content), "Telegram asks to wait")
}
