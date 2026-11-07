package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
)

// throttleTickMsg moves the countdown of a flood wait on, carrying the time it
// was sent at.
type throttleTickMsg struct{ now time.Time }

func throttleTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(now time.Time) tea.Msg { return throttleTickMsg{now: now} })
}

// handleThrottle records a flood wait somebody's call ran into, ending at
// until, as of now (#212). It is a segment of the status bar, counted down
// every second until it is over; nothing says it ended, so its time coming is
// what takes it away.
func (m RootModel) handleThrottle(until, now time.Time) (RootModel, tea.Cmd) {
	m.throttleUntil = until
	return m.showThrottle(now)
}

// showThrottle draws what is left of the wait as of now, and keeps the
// countdown running while there is any. One countdown runs at a time.
func (m RootModel) showThrottle(now time.Time) (RootModel, tea.Cmd) {
	left := m.throttleUntil.Sub(now)
	if left <= 0 {
		m.statusBar.SetThrottle("")
		return m, nil
	}
	m.statusBar.SetThrottle("Telegram asks to wait " + waitWords(left))
	if m.throttleTicking {
		return m, nil
	}
	m.throttleTicking = true
	return m, throttleTickCmd()
}

func (m RootModel) handleThrottleTick(msg throttleTickMsg) (RootModel, tea.Cmd) {
	m.throttleTicking = false
	return m.showThrottle(msg.now)
}

// waitWords says how long is left of a wait, to the second it ends in: "33s",
// "9m12s".
func waitWords(d time.Duration) string {
	s := int((d + time.Second - 1) / time.Second)
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", s/60, s%60)
}
