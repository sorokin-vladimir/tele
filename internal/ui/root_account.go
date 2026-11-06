package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// handleAccountDelta draws where the account stands. The contents arrive whole,
// and the screen acts on what changed since the last: a login step asked, a
// login that cannot go on, the account logged in and then loaded, a connection
// that ended, the clock. The first contents are compared with a fresh start, so
// a client attaching in the middle of a login is asked the step at once (#309).
func (m RootModel) handleAccountDelta(next project.AccountContents) (RootModel, tea.Cmd) {
	prev := m.account
	m.account = next
	var cmds []tea.Cmd
	route := func(msg tea.Msg) {
		updated, cmd := m.updateInner(msg)
		m = updated.(RootModel)
		cmds = append(cmds, cmd)
	}

	if next.HistoryPage != prev.HistoryPage {
		// The window grows by a page from the next time it does; the one on
		// screen stays as it was asked for.
		m.historyLimit = next.HistoryPage
	}
	if next.LoggedInUser != prev.LoggedInUser {
		components.SetSelfIdentity(next.LoggedInUser.ID, next.LoggedInUser.Username)
	}
	if next.ClockSkew != prev.ClockSkew {
		m = m.handleClockSkew(next.ClockSkew)
	}
	if next.Throttle != prev.Throttle {
		updated, cmd := m.handleThrottle(next.Throttle.Until, time.Now())
		m = updated
		cmds = append(cmds, cmd)
	}
	if next.Phase == project.AccountLoggingIn && next.Login != prev.Login {
		route(screens.AuthRequestMsg{Step: next.Login.Kind, Hint: next.Login.Hint, Err: next.Login.Err,
			Seq: next.Login.Seq, Token: next.Login.Token})
	}
	if next.LoginFailed != "" && next.LoginFailed != prev.LoginFailed {
		route(screens.AuthErrorMsg{Text: next.LoginFailed})
	}
	if next.Phase >= project.AccountLoading && prev.Phase < project.AccountLoading {
		route(screens.ConnectedMsg{})
	}
	if next.Phase == project.AccountReady && prev.Phase != project.AccountReady {
		route(screens.TransitionToMainMsg{})
	}
	if next.Ended != nil && (prev.Ended == nil || prev.Ended.Error() != next.Ended.Error()) {
		updated, cmd := m.handleConnectFailed(next.Ended)
		m = updated
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// handleLoginAnswer hands what was typed at a login step to the owner. An
// answer refused because another window drives the login says so under the
// field (#211). Any other refusal says the step was already answered, here or
// by another client, and the account projection is already on its way with
// whatever comes next; any other failure is the account ending, which arrives
// the same way. Neither has anything to say here.
func (m RootModel) handleLoginAnswer(msg screens.LoginAnswerMsg) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	ctx, owner := m.ctx, m.owner
	return m, func() tea.Msg {
		if err := owner.AnswerLogin(ctx, msg.Seq, msg.Value); telerr.Of(err) == telerr.Forbidden {
			return screens.LoginRefusedMsg{Seq: msg.Seq, Text: loginElsewhere}
		}
		return nil
	}
}

// loginElsewhere is what an answer or a switch refused for another window's
// login says under the field (#211).
const loginElsewhere = "This login is being answered in another window."

// handleLoginSwitch hands a switch of the login to the owner, refused the way
// an answer is: only another window driving it is worth saying (#330).
func (m RootModel) handleLoginSwitch(msg screens.LoginSwitchMsg) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	ctx, owner := m.ctx, m.owner
	return m, func() tea.Msg {
		if err := owner.SwitchLogin(ctx, msg.Seq, msg.To); telerr.Of(err) == telerr.Forbidden {
			return screens.LoginRefusedMsg{Seq: msg.Seq, Text: loginElsewhere}
		}
		return nil
	}
}
