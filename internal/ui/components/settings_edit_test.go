package components_test

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// editable builds the overlay over a real file and hands back the store and the
// path, because what an edit is for is the file.
func editable(t *testing.T, body string) (*components.SettingsModal, *config.Store, string) {
	t.Helper()
	path := writeConfig(t, body)
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	km := keys.DefaultKeyMap()
	return components.NewSettingsModal(store, km, km, 100, 40), store, path
}

func key(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }

func typed(s string) []tea.KeyPressMsg {
	out := make([]tea.KeyPressMsg, 0, len(s))
	for _, r := range s {
		out = append(out, key(r))
	}
	return out
}

// press sends keys in order and returns the overlay and whether anything was
// written along the way.
func press(m *components.SettingsModal, msgs ...tea.KeyPressMsg) (*components.SettingsModal, bool) {
	changed := false
	for _, msg := range msgs {
		var res components.SettingsResult
		m, res = m.Update(msg)
		changed = changed || res.Changed
	}
	return m, changed
}

// moveTo puts the cursor on the row with this label.
func moveTo(t *testing.T, m *components.SettingsModal, label string) *components.SettingsModal {
	t.Helper()
	for range 60 {
		if m.CursorLabelForTest() == label {
			return m
		}
		m, _ = press(m, key('j'))
	}
	t.Fatalf("never reached the %q row", label)
	return nil
}

func fileContains(t *testing.T, path, needle string) bool {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Contains(string(raw), needle)
}

// A toggle has no half-pressed state, so it commits as soon as it is pressed.
func TestSettingsEdit_ATogglePressedWritesTheFile(t *testing.T) {
	m, store, path := editable(t, "")
	m = moveTo(t, m, "Notification preview")

	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, changed, "the app is told to apply it")
	assert.False(t, store.Current().UI.NotificationPreview)
	assert.True(t, fileContains(t, path, "notification_preview: false"))
	assert.Contains(t, rowFor(t, m, "Notification preview"), "off", "and the row shows it")
}

// A choice moves along its list, wrapping, so one key reaches any of them.
func TestSettingsEdit_AChoiceCyclesAndWraps(t *testing.T) {
	m, store, _ := editable(t, "")
	m = moveTo(t, m, "Error zone")
	require.Equal(t, "bottom-right", store.Current().UI.Toasts.ErrorZone)

	m, changed := press(m, key('l'))
	assert.True(t, changed)
	assert.Equal(t, "top-right", store.Current().UI.Toasts.ErrorZone)

	m, _ = press(m, key('l'))
	assert.Equal(t, "bottom-right", store.Current().UI.Toasts.ErrorZone, "wrapped")

	_, _ = press(m, key('h'))
	assert.Equal(t, "top-right", store.Current().UI.Toasts.ErrorZone, "and goes back the other way")
}

// A number is typed and then confirmed. Writing per keystroke would put 1, 12
// and 120 through the file in turn and apply each of them.
func TestSettingsEdit_ANumberIsWrittenOnConfirmAndNotBefore(t *testing.T) {
	m, store, _ := editable(t, "")
	m = moveTo(t, m, "History limit")

	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	require.False(t, changed, "opening the field writes nothing")
	m, changed = press(m, typed("120")...)
	require.False(t, changed, "and neither does typing into it")
	assert.Equal(t, 50, store.Current().UI.HistoryLimit)

	m, changed = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, changed)
	assert.Equal(t, 120, store.Current().UI.HistoryLimit)
	assert.Contains(t, rowFor(t, m, "History limit"), "120 messages")
}

// Escape puts the value back. Nothing was written, because nothing is written
// until it is confirmed.
func TestSettingsEdit_EscapeAbandonsWhatWasTyped(t *testing.T) {
	m, store, _ := editable(t, "ui:\n  history_limit: 50\n")
	m = moveTo(t, m, "History limit")

	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, typed("999")...)
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEscape})

	assert.False(t, changed)
	assert.Equal(t, 50, store.Current().UI.HistoryLimit)
	assert.Contains(t, rowFor(t, m, "History limit"), "50 messages")
}

// Refused where it was typed, rather than written and complained about
// afterwards. The store refuses before it opens the file.
func TestSettingsEdit_AnIllegalValueIsRefusedAtTheWidget(t *testing.T) {
	m, store, path := editable(t, "")
	m = moveTo(t, m, "Max visible")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, changed2 := press(m, typed("99")...)
	m, changed3 := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, changed || changed2 || changed3)
	assert.Contains(t, m.StatusForTest(), "at most 10", "and it says what is allowed")
	assert.Equal(t, 3, store.Current().UI.Toasts.MaxVisible)

	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, before, after, "the file was never opened")

	// The field is still open on what was typed, so the typo can be corrected
	// rather than started again.
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	_, changed = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.True(t, changed)
	assert.Equal(t, 9, store.Current().UI.Toasts.MaxVisible)
}

func TestSettingsEdit_SomethingThatIsNotANumberIsRefused(t *testing.T) {
	m, _, _ := editable(t, "")
	m = moveTo(t, m, "History limit")

	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, typed("lots")...)
	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, changed)
	assert.Contains(t, m.StatusForTest(), "whole number")
}

// A size is shown and typed in the unit a person picks it in.
func TestSettingsEdit_ASizeIsTypedInMegabytes(t *testing.T) {
	m, store, _ := editable(t, "")
	m = moveTo(t, m, "Avatar cache size")

	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, typed("32")...)
	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, changed)
	assert.Equal(t, int64(32<<20), store.Current().Avatars.DiskCacheSize)
	assert.Contains(t, rowFor(t, m, "Avatar cache size"), "32 MB")
}

// Resetting writes absence: the key goes out of the file, and what the setting
// is worth is the default again.
func TestSettingsEdit_ResetRemovesTheKeyFromTheFile(t *testing.T) {
	m, store, path := editable(t, "ui:\n  history_limit: 120\n")
	m = moveTo(t, m, "History limit")
	require.True(t, fileContains(t, path, "history_limit"))

	m, changed := press(m, key('r'))

	assert.True(t, changed)
	assert.False(t, fileContains(t, path, "history_limit"), "the key is gone, not set to 50")
	assert.Equal(t, 50, store.Current().UI.HistoryLimit)
	assert.Contains(t, rowFor(t, m, "History limit"), "default")
}

// Emptying the field means the same thing as the reset key: absence is the
// value, so typing nothing is asking for the default.
func TestSettingsEdit_AnEmptiedFieldResetsToTheDefault(t *testing.T) {
	m, store, path := editable(t, "ui:\n  history_limit: 120\n")
	m = moveTo(t, m, "History limit")

	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	for range 3 {
		m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	_, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, changed)
	assert.False(t, fileContains(t, path, "history_limit"))
	assert.Equal(t, 50, store.Current().UI.HistoryLimit)
}

// A read-only setting is not a dead end: pressing it says where it is changed.
func TestSettingsEdit_AReadOnlyRowSaysWhereItIsChanged(t *testing.T) {
	m, _, path := editable(t, "")
	m = moveTo(t, m, "API ID")

	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.False(t, changed)
	assert.Contains(t, m.StatusForTest(), path)
}

func TestSettingsEdit_AReadOnlyRowCannotBeReset(t *testing.T) {
	m, _, path := editable(t, "state_dir: ~/somewhere\n")
	m = moveTo(t, m, "State directory")

	_, changed := press(m, key('r'))

	assert.False(t, changed)
	assert.True(t, fileContains(t, path, "state_dir"))
}

// One half of a theme pair is written on its own, without the overlay having to
// rewrite the pair or flatten it into a single name.
func TestSettingsEdit_AThemeSlotIsEditedOnItsOwn(t *testing.T) {
	m, store, path := editable(t, "ui:\n  theme:\n    dark: nord\n    light: seoul256-light\n")
	m = moveTo(t, m, "Theme (dark)")

	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	for range len("nord") {
		m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	m, _ = press(m, typed("gruvbox-dark")...)
	m, changed := press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	assert.True(t, changed)
	assert.Equal(t, config.ThemeSlots{Dark: "gruvbox-dark", Light: "seoul256-light"}, store.Current().UI.ThemeSlots)
	assert.True(t, fileContains(t, path, "dark: gruvbox-dark"))
	assert.True(t, fileContains(t, path, "light: seoul256-light"), "the other half is left alone")
	assert.Contains(t, rowFor(t, m, "Theme (dark)"), "gruvbox-dark")
}

// The cursor visits settings and steps over everything that is only there to be
// read.
func TestSettingsEdit_TheCursorOnlyStopsOnSettings(t *testing.T) {
	m, _, _ := editable(t, "")

	assert.Equal(t, "API ID", m.CursorLabelForTest(), "it starts on the first setting")

	seen := map[string]bool{}
	for range 40 {
		seen[m.CursorLabelForTest()] = true
		m, _ = press(m, key('j'))
	}

	assert.True(t, seen["Avatar cache size"], "it reaches the last setting")
	for label := range seen {
		assert.NotContains(t, label, "keybindings", "and never stops on a heading or a binding")
		assert.NotEqual(t, "reply", label)
		assert.NotEqual(t, "edited in", label)
	}
}

// Writing goes through the file, so a value set here is a value an editor would
// have set the same way - the criterion this whole design rests on.
func TestSettingsEdit_WhatIsWrittenIsWhatAHandEditWouldWrite(t *testing.T) {
	m, _, path := editable(t, "")
	m = moveTo(t, m, "Max visible")
	m, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = press(m, typed("7")...)
	_, _ = press(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	byHand := writeConfig(t, "ui:\n  toasts:\n    max_visible: 7\n")
	fromOverlay, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	handEdited, err := config.NewStore(byHand, t.TempDir())
	require.NoError(t, err)

	assert.Equal(t, handEdited.Current().UI, fromOverlay.Current().UI)
}

// rowFor is the rendered row with this label, so a test reads what a person
// would see rather than what the store holds.
func rowFor(t *testing.T, m *components.SettingsModal, label string) string {
	t.Helper()
	for _, l := range settingsLines(m) {
		// The row under the cursor carries a marker instead of the left margin,
		// so the label is matched after whatever the margin is.
		if strings.HasPrefix(strings.TrimLeft(l, " ›"), label) {
			return l
		}
	}
	t.Fatalf("no row for %q in:\n%s", label, strings.Join(settingsLines(m), "\n"))
	return ""
}
