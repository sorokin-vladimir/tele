package ui

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func logOutModel(t *testing.T, unsent int) (RootModel, *ownerStub) {
	t.Helper()
	o := newOwnerStub(store.NewMemory())
	o.logOutPreview = core.LogOutPreview{Name: "@ada", Unsent: unsent}
	m := mainScreenModel().WithOwner(o).WithContext(context.Background())
	return m, o
}

// openLogOut asks for a log out and opens the confirmation with what the owner
// says it would take.
func openLogOut(t *testing.T, m RootModel) RootModel {
	t.Helper()
	m, cmd := m.askLogOut()
	require.NotNil(t, cmd)
	m, _ = m.handleLogOutPreview(cmd().(logOutPreviewMsg))
	require.NotNil(t, m.logOut, "no confirmation opened")
	return m
}

func screenText(m RootModel) string { return xansi.Strip(m.View().Content) }

// A log out removes what cannot be had back from Telegram, so it is confirmed
// first, naming whose account it is and listing what goes and what stays
// (#297).
func TestLogOut_IsConfirmedFirstWithWhatGoesAndWhatStays(t *testing.T) {
	m, o := logOutModel(t, 3)

	m = openLogOut(t, m)

	view := screenText(m)
	assert.Contains(t, view, "Log out @ada?")
	assert.Contains(t, view, "the session, here and at Telegram")
	assert.Contains(t, view, "local history, cached media and avatars")
	assert.Contains(t, view, "3 unsent messages")
	assert.Contains(t, view, "Kept: config, themes, log.")
	assert.Empty(t, o.loggedOut, "nothing happens before it is confirmed")
}

// g L is a global chord, and the person presses it from whichever pane has
// the focus. Each pane resolves its keys through the matcher, which finishes
// the chord there; the log out it names was handed to the pane, which ignored
// it, so the chord did nothing anywhere (#297).
func TestLogOut_TheChordAsksFromEveryPane(t *testing.T) {
	for name, focus := range map[string]Focus{"chat list": FocusChatList, "chat": FocusChat, "folders": FocusFolders} {
		t.Run(name, func(t *testing.T) {
			m, _ := logOutModel(t, 0)
			m.focus = focus

			model, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
			_, cmd := model.(RootModel).Update(tea.KeyPressMsg{Code: 'L', Text: "L"})

			require.NotNil(t, cmd, "g L did nothing")
			_, ok := cmd().(logOutPreviewMsg)
			assert.True(t, ok, "g L did not ask what a log out would take")
		})
	}
}

func TestLogOut_NothingUnsentIsNotMentioned(t *testing.T) {
	m, _ := logOutModel(t, 0)

	assert.NotContains(t, screenText(openLogOut(t, m)), "unsent")
}

func TestLogOut_EscCancels(t *testing.T) {
	m, o := logOutModel(t, 0)
	m = openLogOut(t, m)

	m, _, _ = m.handleLogOutKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.Nil(t, m.logOut)
	assert.Empty(t, o.loggedOut)
}

// Confirmed, the log out tells Telegram, and the dialog stays until the host
// puts the next login on screen.
func TestLogOut_ConfirmedTellsTelegram(t *testing.T) {
	m, o := logOutModel(t, 0)
	m = openLogOut(t, m)

	m, cmd, handled := m.handleLogOutKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.True(t, handled)
	require.NotNil(t, cmd)
	m, _ = m.handleLogOutDone(cmd().(logOutDoneMsg))

	assert.Equal(t, []bool{true}, o.loggedOut)
	assert.Contains(t, screenText(m), "Logging out")
}

// Telegram out of reach leaves the session in the person's devices list, so
// that is said, and logging out here anyway is theirs to choose.
// Yes is the key under y on whatever layout is on, as every other key in tele
// is: on a Russian layout that is н, at both questions.
func TestLogOut_ConfirmedOnAnyLayout(t *testing.T) {
	m, o := logOutModel(t, 0)
	o.logOutErrs = []error{errors.New("connection refused")}
	m = openLogOut(t, m)

	m, cmd, _ := m.handleLogOutKey(tea.KeyPressMsg{Code: 'н', Text: "н"})
	require.NotNil(t, cmd, "н did not confirm the log out")
	m, _ = m.handleLogOutDone(cmd().(logOutDoneMsg))
	require.Contains(t, screenText(m), "Log out here anyway?")

	m, cmd, _ = m.handleLogOutKey(tea.KeyPressMsg{Code: 'н', Text: "н"})
	require.NotNil(t, cmd, "н did not confirm a log out here only")
	_, _ = m.handleLogOutDone(cmd().(logOutDoneMsg))
	assert.Equal(t, []bool{true, false}, o.loggedOut)
}

func TestLogOut_TelegramOutOfReachAsksAgain(t *testing.T) {
	m, o := logOutModel(t, 0)
	o.logOutErrs = []error{errors.New("connection refused")}
	m = openLogOut(t, m)

	m, cmd, _ := m.handleLogOutKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m, _ = m.handleLogOutDone(cmd().(logOutDoneMsg))

	view := screenText(m)
	assert.Contains(t, view, "Telegram could not be reached")
	assert.Contains(t, view, "stays in your devices list")
	assert.Contains(t, view, "Log out here anyway?")

	m, cmd, _ = m.handleLogOutKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	_, _ = m.handleLogOutDone(cmd().(logOutDoneMsg))
	assert.Equal(t, []bool{true, false}, o.loggedOut)
}

// A banned account offers no login for its number, but the person can leave it
// and log in with another one: Enter asks for the account to end (#297).
func TestBanned_EnterLeavesForAnotherNumber(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	m := nextAccountRoot().WithOwner(o)
	m, _ = m.handleConnectFailed(ConnectFailedMsg{Err: &telerr.Error{Kind: telerr.AccountBanned}})
	require.Contains(t, screenText(m), "Press Enter to log in with another number.")

	m, cmd, handled := m.handleBannedKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.True(t, handled)
	require.NotNil(t, cmd)
	cmd()
	_, _, again := m.handleBannedKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.False(t, again, "asked once")

	assert.Equal(t, []core.EndReason{core.EndBanned}, o.endedWith)
}

// Any other failure on the login screen leaves Enter alone: there is nothing
// to leave.
func TestBanned_EnterDoesNothingElsewhere(t *testing.T) {
	o := newOwnerStub(store.NewMemory())
	m := nextAccountRoot().WithOwner(o)
	m, _ = m.handleConnectFailed(ConnectFailedMsg{Err: &telerr.Error{Kind: telerr.Network}})

	_, _, handled := m.handleBannedKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, handled)
	assert.Empty(t, o.endedWith)
}

// While the dialog is open it owns the keys: nothing reaches the chat behind.
func TestLogOut_TheDialogOwnsTheKeys(t *testing.T) {
	m, _ := logOutModel(t, 0)
	m = openLogOut(t, m)

	_, _, handled := m.handleLogOutKey(tea.KeyPressMsg{Code: 'j', Text: "j"})

	assert.True(t, handled)
}
