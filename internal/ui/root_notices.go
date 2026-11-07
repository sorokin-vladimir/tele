package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// noticeTickMsg advances the dismissal countdown once per second.
type noticeTickMsg struct{}

// WithNotices seeds the pending one-time notices. Seen-state is written on
// dismissal, not on display, so quitting early shows the notice again.
// The ones that wait for the login go last, so they hold back none that do
// not.
func (m RootModel) WithNotices(pending []notices.Notice, seen notices.Seen) RootModel {
	queue := make([]notices.Notice, 0, len(pending))
	for _, n := range pending {
		if !n.AfterLogin {
			queue = append(queue, n)
		}
	}
	for _, n := range pending {
		if n.AfterLogin {
			queue = append(queue, n)
		}
	}
	m.noticeQueue = queue
	m.noticeSeen = seen
	m.noticeLeft = 0
	if len(queue) > 0 {
		m.noticeLeft = int(queue[0].Delay.Seconds())
	}
	return m
}

// noticeActive reports whether a notice is up: there is one, and it is not
// one still waiting for the login.
func (m RootModel) noticeActive() bool {
	return len(m.noticeQueue) > 0 && (!m.noticeQueue[0].AfterLogin || m.screen != ScreenLogin)
}

// noticeAfterLoginCmd starts the countdown of a notice that has waited for the
// login, which nothing was counting down while it waited.
func (m RootModel) noticeAfterLoginCmd() tea.Cmd {
	if m.noticeActive() && m.noticeQueue[0].AfterLogin && m.noticeLeft > 0 {
		return noticeTickCmd()
	}
	return nil
}

func (m RootModel) noticeRemaining() int { return m.noticeLeft }

// noticeTick decrements the countdown, never below zero.
func (m RootModel) noticeTick() RootModel {
	if m.noticeLeft > 0 {
		m.noticeLeft--
	}
	return m
}

// noticeTickCmd schedules the next countdown tick.
func noticeTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return noticeTickMsg{} })
}

// handleNoticeKey consumes a key press while a notice is up. The bool reports
// whether the key was swallowed, so the caller knows not to process it further.
// Before the countdown ends every key is swallowed and nothing happens; after
// it, any key dismisses the notice and advances the queue.
func (m RootModel) handleNoticeKey() (RootModel, bool) {
	if !m.noticeActive() {
		return m, false
	}
	if m.noticeLeft > 0 {
		return m, true
	}
	return m.dismissNotice(), true
}

// dismissNotice marks the head of the queue seen and brings up the next one,
// with its own countdown.
func (m RootModel) dismissNotice() RootModel {
	m.noticeSeen.MarkSeen(m.noticeQueue[0].ID)
	m.noticeQueue = m.noticeQueue[1:]
	m.noticeLeft = 0
	if len(m.noticeQueue) > 0 {
		m.noticeLeft = int(m.noticeQueue[0].Delay.Seconds())
	}
	return m
}

// handleNoticeAnswer consumes a key press while a notice that asks is up. Only
// y, n and Esc answer it, and an answer dismisses it either way; every other
// key is swallowed, since a question asked once must not be answered by a key
// pressed out of habit. The bool reports whether the head of the queue asks.
func (m RootModel) handleNoticeAnswer(msg tea.KeyPressMsg) (RootModel, tea.Cmd, bool) {
	if !m.noticeActive() || !m.noticeQueue[0].Asks() {
		return m, nil, false
	}
	var cmd tea.Cmd
	switch msg.String() {
	case "y":
		ctx, yes := m.ctx, m.noticeQueue[0].Yes
		// What the yes came to is said as a toast, a failure as an error.
		cmd = func() tea.Msg {
			text, err := yes(ctx)
			if err != nil {
				return StatusErrMsg{Text: err.Error(), Sev: components.SeverityError}
			}
			return StatusErrMsg{Text: text, Sev: components.SeverityInfo}
		}
	case "n", "esc":
	default:
		return m, nil, true
	}
	return m.dismissNotice(), cmd, true
}

// noticeView renders the head of the queue centred over dimmed content.
func (m RootModel) noticeView(content string) string {
	n := m.noticeQueue[0]
	maxW := m.width - 10
	if maxW > 72 {
		maxW = 72
	}
	if maxW < 24 {
		maxW = 24
	}
	box := components.RenderNoticeBox(n.Title, n.Body, m.noticeLeft, maxW)
	if n.Asks() {
		box = components.RenderDialog(n.Title, n.Body, "y / n", maxW)
	}
	return overlayCenter(dimBackground(content), box, m.width, m.height)
}
