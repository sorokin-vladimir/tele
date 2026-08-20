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
	cfg := loadWith(t, "ui:\n  notification_preview: sometimes\n")

	assert.True(t, cfg.UI.NotificationPreview)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "ui.notification_preview")
}

// Repair touches the key that is wrong and nothing else. A file with one bad
// value is not a file to be reset.
func TestLoad_RepairLeavesTheRestOfTheFileAlone(t *testing.T) {
	cfg := loadWith(t, "ui:\n  history_limit: 0\n  notification_preview: false\n  toasts:\n    notify_zone: bottom-right\n")

	assert.Equal(t, 50, cfg.UI.HistoryLimit, "repaired")
	assert.False(t, cfg.UI.NotificationPreview, "kept")
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

	_, ok = config.Setting("ui.date_format")
	assert.False(t, ok, "excluded, not declared: #238")
}

func TestSettings_AreReturnedAsACopy(t *testing.T) {
	first := config.Settings()
	require.NotEmpty(t, first)
	first[0] = settings.Entry{Key: "vandalised"}

	assert.NotEqual(t, "vandalised", config.Settings()[0].Key)
}
