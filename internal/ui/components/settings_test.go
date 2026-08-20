package components_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// settingsOverlay builds the overlay over a real config file, because what is
// being tested is what a person sees for a config they actually have.
// writeConfig puts a config file with the given body in a fresh directory.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(
		"telegram:\n  api_id: 12345\n  api_hash: \"deadbeef\"\n"+body), 0600))
	return path
}

func settingsOverlay(t *testing.T, body string, km keys.KeyMap) (*components.SettingsModal, string) {
	t.Helper()
	path := writeConfig(t, body)
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	if km == nil {
		km = keys.DefaultKeyMap()
	}
	return components.NewSettingsModal(store, km, keys.DefaultKeyMap(), 100, 40), path
}

// plain returns the overlay's rows with their styling stripped, which is what a
// person reads.
func settingsLines(m *components.SettingsModal) []string {
	out := make([]string, 0, len(m.LinesForTest()))
	for _, l := range m.LinesForTest() {
		out = append(out, strings.TrimRight(xansi.Strip(l), " "))
	}
	return out
}

func find(t *testing.T, m *components.SettingsModal, needle string) string {
	t.Helper()
	for _, l := range settingsLines(m) {
		if strings.Contains(l, needle) {
			return l
		}
	}
	t.Fatalf("no row containing %q in:\n%s", needle, strings.Join(settingsLines(m), "\n"))
	return ""
}

func indexOf(t *testing.T, m *components.SettingsModal, needle string) int {
	t.Helper()
	for i, l := range settingsLines(m) {
		if strings.Contains(l, needle) {
			return i
		}
	}
	t.Fatalf("no row containing %q", needle)
	return -1
}

// The overlay reads like the file, top to bottom, so a row seen here is found
// in the file by looking in the same place.
func TestSettings_SectionsFollowTheFile(t *testing.T) {
	m, path := settingsOverlay(t, "", nil)

	sections := []string{"telegram", filepath.Base(path), "ui", "ui.toasts", "photos", "avatars",
		"keybindings.global", "keybindings.chat"}
	last := -1
	for _, s := range sections {
		at := indexOf(t, m, s)
		assert.Greater(t, at, last, "%s is out of order", s)
		last = at
	}
}

// The credential is shown as present and not shown.
func TestSettings_MasksTheApiHash(t *testing.T) {
	m, _ := settingsOverlay(t, "", nil)

	row := find(t, m, "API hash")
	assert.NotContains(t, row, "deadbeef")
	assert.Contains(t, row, "••••")
	assert.Contains(t, row, "read-only")

	assert.Contains(t, find(t, m, "API ID"), "12345", "the id is not a secret")
}

// Read-only rows say so, and the overlay says where they are changed instead -
// otherwise being unable to change one here is a dead end.
func TestSettings_ReadOnlyRowsNameTheFile(t *testing.T) {
	m, path := settingsOverlay(t, "", nil)

	assert.Contains(t, find(t, m, "State directory"), "read-only")
	// Shortened to fit the box, so the end of it - the file's own name - is what
	// is checked. A path too long to show is still shown as much as there is
	// room for.
	assert.Contains(t, find(t, m, "edited in"), filepath.Base(path))
}

// A value nobody chose reads as tele's answer rather than as a decision.
func TestSettings_MarksDefaultsAndLeavesChoicesUnmarked(t *testing.T) {
	m, _ := settingsOverlay(t, "ui:\n  history_limit: 120\n", nil)

	chosen := find(t, m, "History limit")
	assert.Contains(t, chosen, "120 messages")
	assert.NotContains(t, chosen, "default")

	assert.Contains(t, find(t, m, "Notification preview"), "default")
}

// Immediate settings carry no mark: taking effect at once is what a person
// expects, and marking most of the overlay to say nothing is unusual is noise.
func TestSettings_MarksOnlyTheSettingsThatDoNotApplyAtOnce(t *testing.T) {
	m, _ := settingsOverlay(t, "", nil)

	assert.NotContains(t, find(t, m, "Notification preview"), "next")
	assert.NotContains(t, find(t, m, "Notification preview"), "restart")

	assert.Contains(t, find(t, m, "History limit"), "next")
	assert.Contains(t, find(t, m, "Max long side"), "next")
	assert.Contains(t, find(t, m, "Image mode"), "restart")
	assert.Contains(t, find(t, m, "Media cache size"), "restart")
}

// The marks are explained where they are used, so the overlay does not need
// documentation open beside it.
func TestSettings_ExplainsItsMarks(t *testing.T) {
	m, _ := settingsOverlay(t, "", nil)

	for _, mark := range []string{"default", "next", "restart", "read-only"} {
		assert.Contains(t, find(t, m, "["+mark+"]"), mark)
	}
	assert.Contains(t, find(t, m, "what the marks mean"), "what the marks mean")
}

// Values are shown in the words the setting is thought about in.
func TestSettings_ShowsValuesInHumanTerms(t *testing.T) {
	m, _ := settingsOverlay(t, "ui:\n  notification_preview: false\nphotos:\n  disk_cache_size: 268435456\navatars:\n  disk_cache_size: 0\n", nil)

	assert.Contains(t, find(t, m, "Notification preview"), "off")
	assert.Contains(t, find(t, m, "Media cache size"), "256 MB")
	assert.Contains(t, find(t, m, "Avatar cache size"), "nothing kept")
}

// A theme written as a dark/light pair is two rows, because that spelling means
// something different from a single name and flattening it would lose the
// difference.
func TestSettings_ShowsAThemePairAsTwoRows(t *testing.T) {
	m, _ := settingsOverlay(t, "ui:\n  theme:\n    dark: nord\n    light: seoul256-light\n", nil)

	assert.Contains(t, find(t, m, "Theme (dark)"), "nord")
	assert.Contains(t, find(t, m, "Theme (light)"), "seoul256-light")
}

func TestSettings_ShowsASingleThemeNameAsOneRow(t *testing.T) {
	m, _ := settingsOverlay(t, "ui:\n  theme: nord\n", nil)

	// Anchored on the label column: a temp path can contain anything, including
	// the word this row is called.
	assert.Contains(t, find(t, m, "  Theme"), "nord")
	for _, l := range settingsLines(m) {
		assert.NotContains(t, l, "Theme (dark)")
	}
}

// The whole resolved set, not only what the config changed: "what is bound to
// what" is a question about all of it.
func TestSettings_ListsEveryBindingNotOnlyTheOverridden(t *testing.T) {
	m, _ := settingsOverlay(t, "", nil)

	assert.Contains(t, find(t, m, "keybindings.global"), "keybindings.global")
	assert.Contains(t, find(t, m, "  quit"), "q")
	assert.Contains(t, find(t, m, "keybindings.chatlist"), "keybindings.chatlist")
	assert.Contains(t, find(t, m, "  search"), "/")
	assert.Contains(t, find(t, m, "  reply"), "r")
}

// An overridden binding says what it used to be, so a person can tell what their
// config changed from what tele ships.
func TestSettings_ShowsWhatAnOverriddenBindingUsedToBe(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"reply": {"R"}},
	})
	require.Empty(t, warns)
	m, _ := settingsOverlay(t, "", km)

	row := find(t, m, "  reply")
	assert.Contains(t, row, "R")
	assert.Contains(t, row, "was r")
}

// An action tele binds to nothing, bound by a config, is an addition rather than
// an override - and there is no default to report.
func TestSettings_MarksABindingThatHadNoDefault(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"reload_config": {"ctrl+t"}},
	})
	require.Empty(t, warns)
	m, _ := settingsOverlay(t, "", km)

	row := find(t, m, "  reload_config")
	assert.Contains(t, row, "ctrl+t")
	assert.Contains(t, row, "added")
}

// The help screen and this overlay are two views of one fact, so they must not
// disagree about it. Both read the keymap in force; neither reads the defaults
// for what is bound now.
func TestSettings_AgreesWithTheHelpScreenAboutABinding(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"chat": {"reply": {"R"}},
	})
	require.Empty(t, warns)

	m, _ := settingsOverlay(t, "", km)
	// Tall enough that nothing is below the fold: the question is what the help
	// screen says, not what happens to be scrolled into view.
	help := xansi.Strip(components.NewHelpModal(km, 100, 200).View())

	assert.Contains(t, find(t, m, "  reply"), "R")
	assert.Contains(t, help, "R", "the help screen shows the same key")
	assert.NotContains(t, find(t, m, "  reply"), " r ", "and neither shows the default as if it were current")
}

// The overlay is a reference: it scrolls and it closes, and it does not edit.
func TestSettings_ScrollsAndCloses(t *testing.T) {
	m, _ := settingsOverlay(t, "", nil)
	require.Greater(t, len(m.LinesForTest()), 20, "there is more here than fits")

	before := m.View()
	m, res := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	require.True(t, res.Open)
	assert.NotEqual(t, before, m.View(), "the cursor moved")

	_, res = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.False(t, res.Open)

	m2, _ := settingsOverlay(t, "", nil)
	_, res = m2.Update(tea.KeyPressMsg{Code: ',', Text: ","})
	assert.False(t, res.Open, "the key that opens it closes it")
}
