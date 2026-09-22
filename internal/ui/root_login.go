package ui

import (
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// connectFailedAction is how a failed connection is named, on the login screen
// and after it. It fits both: someone with a saved session is not logging in,
// and someone who is has still not reached Telegram.
const connectFailedAction = "Could not connect to Telegram"

// isLoginQuit reports whether a key pressed on the login screen quits. Only a
// quit binding that carries ctrl or alt does: a bare key such as q is a
// character someone may be typing into the phone or password field, and on this
// screen it goes to the field (#283).
func (m RootModel) isLoginQuit(msg tea.KeyPressMsg) bool {
	if !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
		return false
	}
	return m.keyMap.Resolve(keys.ContextGlobal, msg.String()) == keys.ActionQuit
}

// loginQuitKey is the quit binding to name on the login screen: one that works
// there, so one with a modifier. Shortest first, then alphabetical, the way
// KeyMap.KeyFor picks. Empty when every quit binding is a bare key.
func (m RootModel) loginQuitKey() string {
	var found []string
	for key, action := range m.keyMap[keys.ContextGlobal] {
		if action == keys.ActionQuit && (strings.HasPrefix(key, "ctrl+") || strings.HasPrefix(key, "alt+")) {
			found = append(found, key)
		}
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if len(found[i]) != len(found[j]) {
			return len(found[i]) < len(found[j])
		}
		return found[i] < found[j]
	})
	return found[0]
}

// loginErrorText is what the login screen says when it cannot go on: the cause,
// the evidence behind it when there is any, where the log is and how to leave.
func (m RootModel) loginErrorText(cause, evidence string) string {
	var b strings.Builder
	b.WriteString(cause)
	if evidence != "" && evidence != cause {
		b.WriteString("\n" + evidence)
	}
	if m.logPath != "" {
		b.WriteString("\n\nLog: " + m.logPath)
	}
	if key := m.loginQuitKey(); key != "" {
		b.WriteString("\n\nPress " + key + " to quit.")
	}
	return b.String()
}

// connectingText is what sits under the logo while Telegram has not answered.
// Past screens.SlowConnectAfter it grows into what is worth checking, where the
// log is and how to leave; the attempt itself goes on either way. Each line is
// rendered on its own, so the centring pads between styled runs rather than
// inside one.
func (m RootModel) connectingText() string {
	if m.clockSkew != 0 {
		return m.clockSkewText()
	}
	body := theme.S().Body
	if !m.login.Slow() {
		return body.Render("connecting...")
	}
	lines := []string{
		body.Render("still connecting..."),
		"",
		body.Render("Check your network, proxy settings and system clock."),
	}
	return strings.Join(append(lines, m.loginFooterLines()...), "\n")
}

// loginFooterLines are where the log is and which key quits, rendered, for the
// bottom of the connecting screen when it has more to say than "connecting...".
func (m RootModel) loginFooterLines() []string {
	body := theme.S().Body
	var lines []string
	if m.logPath != "" {
		lines = append(lines, body.Render("Log: "+m.logPath))
	}
	if key := m.loginQuitKey(); key != "" {
		lines = append(lines, body.Render("Press "+key+" to quit."))
	}
	return lines
}

// handleConnectFailed shows a connection that ended for good. On the login
// screen it becomes the error step. After login it is a toast, since the chats
// already on screen are still worth reading, and one that stays until it is
// dismissed: the connection will not come back by waiting, so a toast that timed
// out would leave a dead app with nothing on screen saying so.
func (m RootModel) handleConnectFailed(msg ConnectFailedMsg) (RootModel, tea.Cmd) {
	text, sev, ok := errText(connectFailedAction, msg.Err)
	if !ok {
		return m, nil
	}
	if m.screen != ScreenLogin {
		m.toasts.Add(components.ToastKindOf(sev), text)
		return m, nil
	}
	// An error with a kind has a phrase for it, and its own text goes underneath
	// as the evidence. One without a kind has nothing to name beyond that text,
	// so the cause stays plain and the text is said once.
	cause := text
	if _, typed := telerr.As(msg.Err); !typed {
		cause = connectFailedAction + "."
	}
	return m.routeLoginError(m.loginErrorText(cause, msg.Err.Error()))
}

// routeLoginError hands the finished text to the login model as its error step.
func (m RootModel) routeLoginError(text string) (RootModel, tea.Cmd) {
	newLogin, cmd := m.login.Update(screens.AuthErrorMsg{Text: text})
	m.login = newLogin.(screens.LoginModel)
	return m, cmd
}
