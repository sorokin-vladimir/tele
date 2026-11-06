package screens_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

const loginURL = "tg://login?token=AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

// qrScreen is the login screen with room w by h, showing the QR step asked as
// msg.
func qrScreen(w, h int, msg screens.AuthRequestMsg) screens.LoginModel {
	m := screens.NewLoginModel()
	m.SetRoom(w, h)
	msg.Step = project.LoginQR
	next, _ := m.Update(msg)
	return next.(screens.LoginModel)
}

func view(m screens.LoginModel) string { return ansi.Strip(m.View().Content) }

func showsCode(s string) bool { return strings.ContainsAny(s, "▀▄█") }

// words is the text a view says, with the code taken out and every run of
// space made one: what reads beside the code, line after line.
func words(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("▀▄█", r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// The QR step draws the code with what to do with it and the way to the number
// beside it, on an ordinary 80 by 24 terminal (#330).
func TestLoginQR_DrawsTheCodeAndWhatToDo(t *testing.T) {
	m := qrScreen(70, 20, screens.AuthRequestMsg{Token: loginURL, Seq: 1})

	v := view(m)
	assert.True(t, showsCode(v))
	assert.Contains(t, words(v), "Settings > Devices > Link Desktop Device")
	assert.Contains(t, words(v), "Press p or Enter to log in by phone number.")
	for _, l := range strings.Split(m.View().Content, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(l), 70)
	}
	assert.LessOrEqual(t, len(strings.Split(m.View().Content, "\n")), 20)
}

// A tall narrow room takes the text under the code rather than beside it.
func TestLoginQR_ANarrowRoomStacksTheText(t *testing.T) {
	m := qrScreen(42, 34, screens.AuthRequestMsg{Token: loginURL, Seq: 1})

	v := view(m)
	assert.True(t, showsCode(v))
	for _, l := range strings.Split(m.View().Content, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(l), 42)
	}
}

// A room too small for the code says so, and keeps the way to the number; it
// does not switch to the number by itself.
func TestLoginQR_ARoomTooSmallSaysSo(t *testing.T) {
	m := qrScreen(40, 12, screens.AuthRequestMsg{Token: loginURL, Seq: 1})

	v := strings.Join(strings.Fields(view(m)), " ")
	assert.False(t, showsCode(v))
	assert.Contains(t, v, "Make the window larger to show the QR code, or press p to log in by phone number.")
	assert.Equal(t, project.LoginQR, m.CurrentStep())
}

// The code appears once the room grows to fit it.
func TestLoginQR_TheCodeAppearsOnceItFits(t *testing.T) {
	m := qrScreen(40, 12, screens.AuthRequestMsg{Token: loginURL, Seq: 1})

	m.SetRoom(70, 20)

	assert.True(t, showsCode(view(m)))
}

// With no token yet the step says one is coming; with a reason, the reason
// takes the code's place. A code is never drawn without a token.
func TestLoginQR_NoTokenDrawsNoCode(t *testing.T) {
	waiting := view(qrScreen(70, 20, screens.AuthRequestMsg{Seq: 1}))
	assert.False(t, showsCode(waiting))
	assert.Contains(t, waiting, "Getting a QR code...")

	why := view(qrScreen(70, 20, screens.AuthRequestMsg{Seq: 1, Err: "Cannot reach Telegram."}))
	assert.False(t, showsCode(why))
	assert.Contains(t, why, "Cannot reach Telegram.")
}

func press(t *testing.T, m screens.LoginModel, k tea.Key) tea.Msg {
	t.Helper()
	_, cmd := m.Update(tea.KeyPressMsg(k))
	if cmd == nil {
		return nil
	}
	return cmd()
}

// p or Enter on the QR step turn to the phone number; nothing else does.
func TestLoginQR_POrEnterTurnToTheNumber(t *testing.T) {
	m := qrScreen(70, 20, screens.AuthRequestMsg{Token: loginURL, Seq: 3})
	want := screens.LoginSwitchMsg{Seq: 3, To: project.LoginPhone}

	assert.Equal(t, want, press(t, m, tea.Key{Code: 'p', Text: "p"}))
	assert.Equal(t, want, press(t, m, tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, press(t, m, tea.Key{Code: 'x', Text: "x"}))
	assert.Nil(t, press(t, m, tea.Key{Code: tea.KeyEscape}))
}

// Esc on any other step goes back to the QR code, and the step says so.
func TestLoginQR_EscGoesBackFromEveryOtherStep(t *testing.T) {
	for _, step := range []project.LoginStepKind{project.LoginPhone, project.LoginCode, project.LoginPassword} {
		m := screens.NewLoginModel()
		next, _ := m.Update(screens.AuthRequestMsg{Step: step, Seq: 5})
		m = next.(screens.LoginModel)

		assert.Equal(t, screens.LoginSwitchMsg{Seq: 5, To: project.LoginQR}, press(t, m, tea.Key{Code: tea.KeyEscape}))
		assert.Contains(t, view(m), "Press Esc to go back to the QR code.")
	}
}

// The number asked after the password is typed in the clear again: a field
// masked for the password stays masked only as long as the password is asked.
func TestLogin_TheNumberAfterThePasswordIsNotMasked(t *testing.T) {
	m := screens.NewLoginModel()
	next, _ := m.Update(screens.AuthRequestMsg{Step: project.LoginPassword, Seq: 1})
	next, _ = next.(screens.LoginModel).Update(screens.AuthRequestMsg{Step: project.LoginPhone, Seq: 2})
	m = next.(screens.LoginModel)
	next, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: '7', Text: "7"}))

	require.Contains(t, view(next.(screens.LoginModel)), "7")
}
