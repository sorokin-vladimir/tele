package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/settings"
)

// loadWith writes a config file with the given body and loads it.
func loadWith(t *testing.T, body string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"+body), 0600))
	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	return cfg
}

// warningTexts is what a person would actually be shown.
func warningTexts(cfg *config.Config) []string {
	out := make([]string, 0, len(cfg.Warnings))
	for _, w := range cfg.Warnings {
		out = append(out, w.Text)
	}
	return out
}

// The behaviour this replaces: a zone the app does not know was silently
// swapped for bottom-right deep in the UI and nobody was told. The value is
// still repaired - one wrong key must not keep anyone out of their messages -
// but it is now said out loud, in the same terms the overlay would have used to
// refuse it.
func TestLoad_IllegalChoiceIsRepairedAndReported(t *testing.T) {
	cfg := loadWith(t, "ui:\n  toasts:\n    error_zone: bottom-middle\n")

	assert.Equal(t, "bottom-right", cfg.UI.Toasts.ErrorZone)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "ui.toasts.error_zone")
	assert.Contains(t, cfg.Warnings[0].Text, "bottom-middle")
	assert.Empty(t, cfg.Warnings[0].ID, "a value still wrong at every launch is said at every launch")
}

func TestLoad_OutOfBoundsNumberIsRepairedAndReported(t *testing.T) {
	cfg := loadWith(t, "ui:\n  toasts:\n    max_visible: 0\n")

	assert.Equal(t, 3, cfg.UI.Toasts.MaxVisible)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "ui.toasts.max_visible")
}

func TestLoad_WrongTypeIsRepairedAndReported(t *testing.T) {
	cfg := loadWith(t, "ui:\n  notifications:\n    preview: sometimes\n")

	assert.True(t, cfg.UI.Notifications.Preview)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "ui.notifications.preview")
}

// A layout the app cannot use is repaired to the default layout and reported,
// the way any other illegal value is: one wrong key must not keep anyone out of
// their messages, and the person who wrote it must be told (#238).
func TestLoad_UnusableDateFormatIsRepairedAndReported(t *testing.T) {
	cases := []struct {
		name, value string
	}{
		{"not a string", "1504"},
		{"empty", `""`},
		{"a layout that does not round-trip", `"100%"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := loadWith(t, "ui:\n  date_format: "+c.value+"\n")

			assert.Equal(t, "15:04", cfg.UI.DateFormat, "falls back to the default layout")
			require.Len(t, cfg.Warnings, 1, warningTexts(cfg))
			assert.Contains(t, cfg.Warnings[0].Text, "ui.date_format")
			assert.Contains(t, cfg.Warnings[0].Text, "15:04")
			assert.Equal(t, settings.Client, cfg.Warnings[0].Process, "the window reads this key")
		})
	}
}

// A layout the app can use is kept exactly as written, and says nothing.
func TestLoad_DateFormatLayoutIsKeptAsWritten(t *testing.T) {
	cfg := loadWith(t, "ui:\n  date_format: \"02.01.2006 15:04\"\n")

	assert.Equal(t, "02.01.2006 15:04", cfg.UI.DateFormat)
	assert.Empty(t, cfg.Warnings, warningTexts(cfg))
}

// Repair touches the key that is wrong and nothing else. A file with one bad
// value is not a file to be reset.
func TestLoad_RepairLeavesTheRestOfTheFileAlone(t *testing.T) {
	cfg := loadWith(t, "ui:\n  history_limit: 0\n  notifications:\n    preview: false\n  toasts:\n    notify_zone: bottom-right\n")

	assert.Equal(t, 50, cfg.UI.HistoryLimit, "repaired")
	assert.False(t, cfg.UI.Notifications.Preview, "kept")
	assert.Equal(t, "bottom-right", cfg.UI.Toasts.NotifyZone, "kept")
	require.Len(t, cfg.Warnings, 1)
}

// A legal file is a quiet file. This is the test that fails if a default is
// ever declared illegal by its own entry, because then every launch warns.
func TestLoad_LegalFileWarnsAboutNothing(t *testing.T) {
	cfg := loadWith(t, "ui:\n  history_limit: 100\n  toasts:\n    error_zone: top-right\n    max_visible: 5\nphotos:\n  mode: blocks\n  disk_cache_size: 0\n")

	assert.Empty(t, warningTexts(cfg))
}

// Both spellings of ui.theme are correct and mean different things, so neither
// may be repaired into the other. What an unknown theme name does is the theme
// resolver's business, and it has its own warning for it.
func TestLoad_ThemeIsNotJudgedByTheRegistry(t *testing.T) {
	scalar := loadWith(t, "ui:\n  theme: nord\n")
	assert.Empty(t, warningTexts(scalar))

	pair := loadWith(t, "ui:\n  theme:\n    dark: nord\n    light: seoul256-light\n")
	assert.Empty(t, warningTexts(pair))
}

// Read-only settings are not repaired: they are shown, and what they are worth
// is decided by resolveState, which has rules of its own about precedence and
// pinning.
func TestLoad_ReadOnlySettingsAreNotRepaired(t *testing.T) {
	cfg := loadWith(t, "state_dir: ~/somewhere\n")

	assert.NotEmpty(t, cfg.StateDir)
	assert.Empty(t, warningTexts(cfg))
}

func TestSetting_LooksUpByKey(t *testing.T) {
	e, ok := config.Setting("photos.disk_cache_size")
	require.True(t, ok)
	assert.Equal(t, settings.Bytes, e.Widget)
	assert.Equal(t, settings.Startup, e.Applies, "the cache takes its bound when it is opened")

	e, ok = config.Setting("ui.date_format")
	require.True(t, ok, "declared since #238: the message list reads it")
	assert.Equal(t, settings.Text, e.Widget)
	assert.Equal(t, settings.NextUse, e.Applies, "what is drawn keeps the layout it was drawn with")
	assert.Equal(t, settings.Client, e.Process)
}

func TestSettings_AreReturnedAsACopy(t *testing.T) {
	first := config.Settings()
	require.NotEmpty(t, first)
	first[0] = settings.Entry{Key: "vandalised"}

	assert.NotEqual(t, "vandalised", config.Settings()[0].Key)
}

// Every warning says whose setting it is about, because each process reports
// only its own: the host and a client read the same file, and a warning both
// reported would be shown twice in the window of a tele that is both (#315).
func TestLoad_AWarningIsThatOfTheProcessWhoseSettingItIsAbout(t *testing.T) {
	cases := []struct {
		name, body string
		want       settings.Process
	}{
		{"a repaired client setting", "ui:\n  toasts:\n    error_zone: bottom-middle\n", settings.Client},
		{"a repaired host setting", "ui:\n  history_limit: 0\n", settings.Host},
		{"a theme that names no slot", "ui:\n  theme:\n    middle: nord\n", settings.Client},
		{"a proxy key its type does not use", "proxy:\n  type: direct\n  server: example.org\n", settings.Host},
		{"the moved notification preview", "ui:\n  notification_preview: false\n", settings.Host},
		{"the deprecated session file", "", settings.Host},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if body == "" {
				body = "  session_file: " + filepath.Join(t.TempDir(), "session.json") + "\n"
			}
			cfg := loadWith(t, body)

			require.Len(t, cfg.Warnings, 1, warningTexts(cfg))
			assert.Equal(t, c.want, cfg.Warnings[0].Process, cfg.Warnings[0].Text)
		})
	}
}
