package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

const loginURL = "tg://login?token=AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="

// qrRoot is a window w by h on the QR step of the login, with a token to
// draw.
func qrRoot(t *testing.T, o *ownerStub, w, h int) RootModel {
	t.Helper()
	m := nextAccountRoot().WithOwner(o)
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m, _ = next.(RootModel).handleAccountDelta(project.AccountContents{
		Phase: project.AccountLoggingIn,
		Login: project.LoginStep{Kind: project.LoginQR, Seq: 4, Token: loginURL},
	})
	return m
}

// The QR code fits an ordinary 80 by 24 terminal: the logo gives way to it
// when both do not fit, and nothing is cut (#330).
func TestLoginQR_TheCodeFitsAnOrdinaryTerminal(t *testing.T) {
	m := qrRoot(t, newOwnerStub(store.NewMemory()), 80, 24)

	v := m.View().Content
	assert.True(t, strings.ContainsAny(ansi.Strip(v), "▀▄█"), "the code is drawn")
	lines := strings.Split(v, "\n")
	assert.LessOrEqual(t, len(lines), 24)
	for _, l := range lines {
		assert.LessOrEqual(t, ansi.StringWidth(l), 80)
	}
}

// Turning to the number reaches the owner naming the asking it leaves.
func TestLoginQR_TurningToTheNumberReachesTheOwner(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	m := qrRoot(t, o, 80, 24)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	require.NotNil(t, cmd)
	turn, ok := cmd().(screens.LoginSwitchMsg)
	require.True(t, ok)
	_, send := m.Update(turn)
	require.NotNil(t, send)
	send()

	assert.Equal(t, []loginSwitch{{seq: 4, to: project.LoginPhone}}, o.switches)
}

// A switch refused because another window drives the login says so, as an
// answer does (#211).
func TestLoginQR_ASwitchRefusedForAnotherWindowSaysSo(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	o.answerErr = &telerr.Error{Kind: telerr.Forbidden, Op: "login", Detail: "this login is being answered in another window"}
	m := nextAccountRoot().WithOwner(o)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = next.(RootModel).handleAccountDelta(project.AccountContents{
		Phase: project.AccountLoggingIn,
		Login: project.LoginStep{Kind: project.LoginCode, Seq: 3},
	})

	_, send := m.Update(screens.LoginSwitchMsg{Seq: 3, To: project.LoginQR})
	require.NotNil(t, send)
	next, _ = m.Update(send())

	assert.Contains(t, next.(RootModel).View().Content, "This login is being answered in another window.")
}
