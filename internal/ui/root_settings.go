package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// openSettings shows what tele can be configured with.
//
// The overlay is built fresh each time rather than kept, so it always shows what
// the app is running on now - including a change made in an editor and reloaded
// while it was closed.
//
// It is handed both the keymap in force and the defaults, because a binding is
// worth two different things to a reader: what it is now, and whether that is
// something this config changed.
func (m RootModel) openSettings() (RootModel, tea.Cmd) {
	if m.settingsStore == nil {
		return m, nil
	}
	m.settings = components.NewSettingsModal(m.settingsStore, m.keyMap, keys.DefaultKeyMap(), m.width, m.height)
	return m, nil
}

// SettingsOpen reports whether the settings overlay is showing.
func (m RootModel) SettingsOpen() bool { return m.settings != nil }

// Settings is the open overlay, or nil.
func (m RootModel) Settings() *components.SettingsModal { return m.settings }
