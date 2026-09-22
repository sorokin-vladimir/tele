package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// handleClockSkew records how far the clock is from Telegram's (#277). On the
// login screen it is read by connectingText; after login it is a segment of the
// status bar, which stays until the skew is over.
func (m RootModel) handleClockSkew(msg core.ClockSkew) (RootModel, tea.Cmd) {
	m.clockSkew = msg.Skew
	text := ""
	if msg.Skew != 0 {
		text = "clock " + skewWords(msg.Skew, true) + " Telegram"
	}
	m.statusBar.SetClockSkew(text)
	return m, nil
}

// clockSkewText is what the connecting screen says while the clock is skewed:
// the cause is known, so it is said at once and in place of the causes the
// screen would otherwise suggest.
func (m RootModel) clockSkewText() string {
	body := theme.S().Body
	lines := []string{
		body.Render("Your clock is " + skewWords(m.clockSkew, false) + " Telegram's."),
		body.Render("Telegram refuses every message until it is set right."),
	}
	return strings.Join(append(lines, m.loginFooterLines()...), "\n")
}

// skewWords says how far a skew is and which way, the way a person would, up to
// the word that names what it is measured against: "8 minutes ahead of", "45
// seconds behind", or "8m ahead of" when short. The preposition comes with the
// direction because the two directions take different ones. Seconds under a
// minute, minutes under an hour, hours above.
func skewWords(d time.Duration, short bool) string {
	dir := "ahead of"
	if d < 0 {
		dir = "behind"
	}
	abs := d.Abs()
	var n int
	var unit, abbr string
	switch {
	case abs.Round(time.Second) < time.Minute:
		n, unit, abbr = int(abs.Round(time.Second)/time.Second), "second", "s"
	case abs.Round(time.Minute) < time.Hour:
		n, unit, abbr = int(abs.Round(time.Minute)/time.Minute), "minute", "m"
	default:
		n, unit, abbr = int(abs.Round(time.Hour)/time.Hour), "hour", "h"
	}
	n = max(n, 1)
	if short {
		return fmt.Sprintf("%d%s %s", n, abbr, dir)
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s %s", n, unit, dir)
}
