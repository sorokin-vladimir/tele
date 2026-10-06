package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// confirmModel builds a main-screen model with the quit confirmation switched
// on or off, and the chat seeded so key handling has somewhere to land.
func confirmModel(t *testing.T, confirm bool) RootModel {
	t.Helper()
	m := NewRootModel(false).WithScreen(ScreenMain)
	m.cfg = &config.Config{}
	m.cfg.UI.ConfirmQuit = confirm
	m.Chat().SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	return m
}

func isQuitCmd(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if isQuitCmd(t, c) {
				return true
			}
		}
	}
	return false
}

// With the setting on, a quit key opens the confirmation instead of quitting.
func TestRoot_QuitConfirmation_OpensModal(t *testing.T) {
	m := confirmModel(t, true)

	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	root := newM.(RootModel)

	require.NotNil(t, root.confirmQuit, "a quit key must open the confirmation")
	assert.False(t, isQuitCmd(t, cmd), "no quit while the confirmation is open")
}

// y or enter confirms the quit.
func TestRoot_QuitConfirmation_ConfirmQuits(t *testing.T) {
	m := confirmModel(t, true)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	root := newM.(RootModel)
	require.NotNil(t, root.confirmQuit)

	newM, cmd := root.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	assert.Nil(t, newM.(RootModel).confirmQuit, "the modal closes on confirm")
	assert.True(t, isQuitCmd(t, cmd), "confirming must quit")
}

// n, esc or q cancels: the modal closes and tele stays open.
func TestRoot_QuitConfirmation_CancelKeepsRunning(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'n', Text: "n"},
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
	} {
		m := confirmModel(t, true)
		newM, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		root := newM.(RootModel)
		require.NotNil(t, root.confirmQuit)

		newM, cmd := root.Update(key)
		assert.Nil(t, newM.(RootModel).confirmQuit, "the modal closes on cancel")
		assert.False(t, isQuitCmd(t, cmd), "cancel must not quit")
	}
}

// A modifier quit binding pressed again is the escape hatch: it quits without
// asking, so the confirmation can never trap someone.
func TestRoot_QuitConfirmation_EscapeQuits(t *testing.T) {
	m := confirmModel(t, true)
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	root := newM.(RootModel)
	require.NotNil(t, root.confirmQuit)

	_, cmd := root.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	assert.True(t, isQuitCmd(t, cmd), "ctrl+c must quit past the confirmation")
}

// With the setting off, quit is immediate, as it always has been.
func TestRoot_QuitConfirmation_DisabledQuitsImmediately(t *testing.T) {
	m := confirmModel(t, false)

	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	assert.Nil(t, newM.(RootModel).confirmQuit)
	assert.True(t, isQuitCmd(t, cmd), "with confirmation off, quit is immediate")
}

// A quit binding rebound to a plain key must not force-quit: pressing it again
// while the dialog is open does nothing, so rebinding quit to a bare key cannot
// bypass the confirmation.
func TestRoot_QuitConfirmation_PlainReboundQuitDoesNotForceQuit(t *testing.T) {
	m := confirmModel(t, true)
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"x"}},
	})
	require.Empty(t, warns)
	m = m.WithKeyMap(km)

	// x is the quit binding now, so it opens the dialog.
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	root := newM.(RootModel)
	require.NotNil(t, root.confirmQuit, "the rebound quit key must open the confirmation")

	// x again must not quit.
	newM, cmd := root.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.NotNil(t, newM.(RootModel).confirmQuit, "a plain quit key must not force-quit")
	assert.False(t, isQuitCmd(t, cmd), "a plain quit key must not quit past the confirmation")
}

// When quit is rebound to a key the dialog uses, the dialog wins: n cancels
// instead of quitting. n is used rather than esc because esc is bound to cancel
// in the chatlist context, and a context binding wins over a global one (#132),
// so a quit binding on esc would never reach this code.
func TestRoot_QuitConfirmation_DialogKeyWinsOverQuitBinding(t *testing.T) {
	m := confirmModel(t, true)
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"n"}},
	})
	require.Empty(t, warns)
	m = m.WithKeyMap(km)

	// n is the quit binding now, so it opens the dialog.
	newM, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	root := newM.(RootModel)
	require.NotNil(t, root.confirmQuit, "the rebound quit key must open the confirmation")

	// n again cancels (the dialog's meaning), it does not quit.
	newM, cmd := root.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	assert.Nil(t, newM.(RootModel).confirmQuit, "n cancels the dialog")
	assert.False(t, isQuitCmd(t, cmd), "n must cancel, not quit")
}

// ui.confirm_quit is read at the key press, so turning it on in the file and
// reloading changes what the next quit key does without a restart.
func TestTakesHold_ui_confirm_quit(t *testing.T) {
	m, store := applyModel(t, "")
	m = m.WithScreen(ScreenMain)

	require.NoError(t, store.Set("ui.confirm_quit", true))
	m, _ = m.reloadFromDisk()

	newM, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	assert.NotNil(t, newM.(RootModel).confirmQuit, "q must ask before quitting once the setting is on")
	assert.False(t, isQuitCmd(t, cmd), "no quit while the confirmation is open")
}
