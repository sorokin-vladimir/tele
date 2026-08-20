package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// reloadModel builds a model configured to read themes from dir, with name in
// the dark slot.
func reloadModel(t *testing.T, dir, name string) RootModel {
	t.Helper()
	cfg := &config.Config{ThemesDir: dir}
	cfg.UI.Toasts.MaxVisible = 3
	cfg.UI.ThemeSlots = config.ThemeSlots{Dark: name}
	t.Cleanup(func() { theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight}) })
	return NewRootModel(nil, 50, false).WithConfig(cfg)
}

// The authoring loop: edit the file, press the key, see the change. Without this
// every look at a token costs a restart and a reconnect.
func TestRoot_ReloadThemes_PicksUpAnEdit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mine.yml")
	require.NoError(t, os.WriteFile(file, []byte("accent: \"#111111\"\n"), 0600))

	m := reloadModel(t, dir, "mine")
	m, _ = m.reloadFromDisk()
	theme.Apply(true)
	first, err := theme.ParseColor("#111111")
	require.NoError(t, err)
	require.Equal(t, first, theme.T().Accent)

	require.NoError(t, os.WriteFile(file, []byte("accent: \"#222222\"\n"), 0600))
	m.reloadFromDisk()
	theme.Apply(true)

	second, err := theme.ParseColor("#222222")
	require.NoError(t, err)
	assert.Equal(t, second, theme.T().Accent, "the edit must be on screen without a restart")
}

// Reloading must not snap back to the dark slot: the light slot is where half
// the day is spent.
func TestRoot_ReloadThemes_KeepsTheAppliedBackground(t *testing.T) {
	m := reloadModel(t, t.TempDir(), "")

	theme.Apply(false)
	m.reloadFromDisk()

	assert.Equal(t, "tele-light", theme.T().Name)
}

// A broken theme file reports the problem and leaves the session alone.
func TestRoot_ReloadThemes_WarnsOnABadFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.yml"), []byte("accent: notacolor\n"), 0600))

	m := reloadModel(t, dir, "mine")
	m, cmd := m.reloadFromDisk()
	require.NotNil(t, cmd)
	m.SettleToastsForTest()

	zones := m.toasts.Zones()
	require.NotEmpty(t, zones)
	assert.Contains(t, zones[0].Block, "accent")
}

// Only the first problem is shown, but the toast has to admit there are others.
// Warnings come out in the order the loader finds them, so a stray key can stand
// in front of the legibility audit and hide it completely — in the authoring
// loop this whole reload exists for.
func TestRoot_ReloadThemes_SaysHowManyProblemsItIsNotShowing(t *testing.T) {
	dir := t.TempDir()
	// The stray key is parsed, and so warned about, before the audit runs on the
	// finished resolution — which is what puts it first and the audit second.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.yml"), []byte(
		"sparkle: \"#ffffff\"\nbackground: \"#ffffff\"\ntext: \"#000000\"\n"), 0600))

	m := reloadModel(t, dir, "mine")
	m, _ = m.reloadFromDisk()
	m.SettleToastsForTest()

	zones := m.toasts.Zones()
	require.NotEmpty(t, zones)
	text := toastText(zones[0].Block)
	assert.Contains(t, text, "sparkle", "the first problem is still the one shown")
	assert.Contains(t, text, "+1 more, see the log")
}

// toastText recovers the sentence a toast is showing from the box it is drawn
// in: the panel is narrow, so any line of it may have been wrapped mid-phrase.
func toastText(block string) string {
	var words []string
	for _, line := range strings.Split(xansi.Strip(block), "\n") {
		words = append(words, strings.Fields(strings.Trim(line, "│╭╮╰╯─"))...)
	}
	return strings.Join(words, " ")
}
