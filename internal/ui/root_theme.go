package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// WithConfigReload installs what the reload action does. The app supplies it
// because reloading is wider than the UI: the file is re-read, and everything
// holding a config - the owner included - is handed the new one before the UI
// applies it to itself.
func (m RootModel) WithConfigReload(reload func() (*config.Config, error)) RootModel {
	m.reloadConfig = reload
	return m
}

// WithSettingsStore installs where the settings overlay reads from. Without one
// the overlay has nothing to show and the action does nothing, which is what
// tests and any headless caller want.
func (m RootModel) WithSettingsStore(store settings.Store) RootModel {
	m.settingsStore = store
	return m
}

// reloadFromDisk makes what is on disk current: the config file and the theme
// files, read again and applied together.
//
// One action rather than two, because "apply what I have written" is one thought
// and the two files are not separable in practice - a theme named in the config
// is only meaningful with the theme file it names. It is also the only way a
// change reaches a running tele, whether it was made in the settings overlay or
// in an editor, which is what keeps those two from drifting (ADR 0009).
//
// It is cheap and safe mid-session: no component caches a style, and the one
// cache there is holds line counts, which a colour cannot change. The applied
// background is kept, so reloading at noon does not snap the app to the dark
// slot.
func (m RootModel) reloadFromDisk() (RootModel, tea.Cmd) {
	m, loaded, warnings, err := m.applyFromDisk()
	if err != nil {
		// The file is still whatever it was, and so is the app: a config that
		// stopped parsing is a reason to say so, not a reason to lose the
		// settings that were working.
		return m, m.retiringToast(components.ToastError, "config not reloaded: "+err.Error())
	}
	if loaded == nil {
		return m, nil
	}

	kind, text := components.ToastInfo, fmt.Sprintf("reloaded: %s / %s",
		loaded.Dark.Theme.Name, loaded.Light.Theme.Name)
	if len(warnings) > 0 {
		// The first problem is the one worth reading; the rest are in the log,
		// and a reload is something you repeat until it is clean anyway. The
		// count still has to be said: without it the toast reads as "one thing
		// is wrong", and the one thing it happens to show is whichever was
		// found first — a stray key can hide the legibility audit entirely, in
		// the authoring loop this exists for.
		kind, text = components.ToastWarning, warnings[0]
		if rest := len(warnings) - 1; rest > 0 {
			text = fmt.Sprintf("%s (+%d more, see the log)", text, rest)
		}
		if m.log != nil {
			for _, w := range warnings {
				m.log.Warn("reload: " + w)
			}
		}
	}
	return m, m.retiringToast(kind, text)
}

// applyFromDisk is the whole of "make what is on disk current": re-read the
// config, hand it to everything that holds one, apply it here, and reinstall the
// themes it names. It returns what it loaded and everything that was wrong with
// it, and says nothing on screen - what to say depends on who asked.
//
// A reload says what it found, because somebody pressed a key and is waiting for
// an answer. A setting changed in the overlay says nothing, because the answer
// is the value on the row in front of them.
func (m RootModel) applyFromDisk() (RootModel, *theme.Loaded, []string, error) {
	cfg := m.cfg
	if m.reloadConfig != nil {
		reloaded, err := m.reloadConfig()
		if err != nil {
			return m, nil, nil, err
		}
		cfg = reloaded
		m = m.applyConfig(cfg)
	}
	if cfg == nil {
		return m, nil, nil, nil
	}

	loaded := theme.LoadSlots(cfg.ThemesDir, cfg.UI.ThemeSlots.Dark, cfg.UI.ThemeSlots.Light)
	theme.SetSlots(loaded.Slots())

	warnings := make([]string, 0, len(cfg.Warnings)+len(loaded.Warnings))
	for _, w := range cfg.Warnings {
		warnings = append(warnings, w.Text)
	}
	warnings = append(warnings, loaded.Warnings...)
	return m, &loaded, warnings, nil
}

// applySettingChange makes the running app agree with a setting the overlay has
// just written. It is quiet on success: the person is looking at the row they
// changed, and a toast telling them they changed it would be the app talking
// about itself.
func (m RootModel) applySettingChange() (RootModel, tea.Cmd) {
	m, _, _, err := m.applyFromDisk()
	if err != nil {
		return m, m.retiringToast(components.ToastError, "setting saved but not applied: "+err.Error())
	}
	return m, nil
}

// retiringToast shows a toast and returns the timer that takes it away again.
func (m RootModel) retiringToast(kind components.ToastKind, text string) tea.Cmd {
	serial := m.toasts.Add(kind, text)
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}
