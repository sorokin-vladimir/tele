package ui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func settingsRoot(t *testing.T) RootModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	m := NewRootModel(nil, 50, false).WithConfig(store.Current()).WithSettingsStore(store)
	m.screen = ScreenMain
	m.width, m.height = 100, 40
	return m
}

func press(t *testing.T, m RootModel, r rune) RootModel {
	t.Helper()
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: r, Text: string(r)})
	return next.(RootModel)
}

func TestRoot_CommaOpensTheSettingsOverlay(t *testing.T) {
	m := settingsRoot(t)
	require.False(t, m.SettingsOpen())

	m = press(t, m, ',')

	assert.True(t, m.SettingsOpen())
	require.NotNil(t, m.Settings())
	assert.NotEmpty(t, m.Settings().LinesForTest())
}

// While it is open it owns the keys, the way the help modal does: it is a
// reference you scroll, and a stray j must not move the chat list behind it.
func TestRoot_TheOpenOverlayOwnsTheKeys(t *testing.T) {
	m := press(t, settingsRoot(t), ',')

	m = press(t, m, 'j')
	assert.True(t, m.SettingsOpen(), "still open")

	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, next.(RootModel).SettingsOpen(), "and esc closes it")
}

// Without a store there is nothing to show, so the key does nothing rather than
// opening an empty box.
func TestRoot_TheKeyDoesNothingWithoutAStore(t *testing.T) {
	m := NewRootModel(nil, 50, false)
	m.screen = ScreenMain
	m.width, m.height = 100, 40

	m = press(t, m, ',')

	assert.False(t, m.SettingsOpen())
}

// Opened again after a change, it shows the change: it is built from the store
// each time rather than kept from the first opening.
func TestRoot_ReopeningShowsWhatChangedMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	m := NewRootModel(nil, 50, false).WithConfig(store.Current()).WithSettingsStore(store)
	m.screen = ScreenMain
	m.width, m.height = 100, 40

	m = press(t, m, ',')
	require.Contains(t, joined(m), "50 messages")
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(RootModel)

	require.NoError(t, store.Set("ui.history_limit", 175))
	m = press(t, m, ',')

	assert.Contains(t, joined(m), "175 messages")
}

// The point of the whole overlay: a setting changed here reaches the running
// app, not only the file. The path is the same one a reload takes.
func TestRoot_AnEditInTheOverlayReachesTheRunningApp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(
		"telegram:\n  api_id: 1\n  api_hash: x\nui:\n  toasts:\n    max_visible: 3\n"), 0600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	m := NewRootModel(nil, 50, false).
		WithConfig(store.Current()).
		WithSettingsStore(store).
		WithConfigReload(func() (*config.Config, error) {
			if err := store.Reload(); err != nil {
				return nil, err
			}
			return store.Current(), nil
		})
	m.screen = ScreenMain
	m.width, m.height = 100, 40
	require.Equal(t, 3, m.toasts.MaxVisible())

	m = press(t, m, ',')
	for m.Settings().CursorLabelForTest() != "Max visible" {
		m = press(t, m, 'j')
	}
	next, _ := m.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(RootModel)
	m = press(t, m, '7')
	next, _ = m.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(RootModel)

	assert.Equal(t, 7, m.toasts.MaxVisible(), "the stack the app is running on")
	assert.Equal(t, 7, m.cfg.UI.Toasts.MaxVisible, "and the config it is running on")
	assert.True(t, m.SettingsOpen(), "the overlay stays open")
}

// Nothing is said on screen for a change the person just made and is looking
// at. A toast telling them they changed it would be the app talking about
// itself.
func TestRoot_ApplyingASettingIsQuiet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	m := NewRootModel(nil, 50, false).
		WithConfig(store.Current()).
		WithSettingsStore(store).
		WithConfigReload(func() (*config.Config, error) { return store.Current(), store.Reload() })
	m.screen = ScreenMain
	m.width, m.height = 100, 40

	m = press(t, m, ',')
	for m.Settings().CursorLabelForTest() != "Notification preview" {
		m = press(t, m, 'j')
	}
	next, cmd := m.handleMainKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(RootModel)

	assert.Nil(t, cmd, "no toast timer, because there is no toast")
	m.SettleToastsForTest()
	assert.Empty(t, m.toasts.Zones())
}

// The binding a person will look for first is the one that opened this.
func TestSettingsAction_IsBoundAndDescribed(t *testing.T) {
	assert.Equal(t, keys.ActionShowSettings, keys.DefaultKeyMap()[keys.ContextGlobal][","])

	label, ok := keys.Describe(keys.ContextGlobal, keys.ActionShowSettings)
	require.True(t, ok, "an action nobody can describe cannot appear in the help")
	assert.NotEmpty(t, label.Short)
}

func joined(m RootModel) string {
	out := ""
	for _, l := range m.Settings().LinesForTest() {
		out += l + "\n"
	}
	return out
}
