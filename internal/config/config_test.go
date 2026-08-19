package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(`
telegram:
  api_id: 12345
  api_hash: "abc"
  session_file: "/tmp/session.json"
ui:
  date_format: "15:04"
  history_limit: 100
`), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 12345, cfg.Telegram.APIID)
	assert.Equal(t, "abc", cfg.Telegram.APIHash)
	assert.Equal(t, "/tmp/session.json", cfg.Telegram.SessionFile)
	assert.Equal(t, "15:04", cfg.UI.DateFormat)
	assert.Equal(t, 100, cfg.UI.HistoryLimit)
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.UI.HistoryLimit)
	// ui.date_format has no default on purpose: nothing reads it, so a default
	// would be a promise the app does not keep. #238 implements it and gives it
	// one back.
	assert.Empty(t, cfg.UI.DateFormat)
	assert.Equal(t, config.ThemeSlots{}, cfg.UI.ThemeSlots, "no ui.theme leaves both slots built-in")
	assert.Empty(t, cfg.Warnings)
}

// A bare name fills both slots. That is how "this theme, whatever the terminal
// is doing" is said, and it needs no separate switch.
func TestTheme_NameFillsBothSlots(t *testing.T) {
	cfg := loadWithUI(t, "  theme: gruvbox-dark\n")

	assert.Equal(t, config.ThemeSlots{Dark: "gruvbox-dark", Light: "gruvbox-dark"}, cfg.UI.ThemeSlots)
	assert.Empty(t, cfg.Warnings)
}

// A map fills the slots it names and leaves the others built-in.
func TestTheme_MapFillsNamedSlots(t *testing.T) {
	cfg := loadWithUI(t, "  theme:\n    dark: gruvbox-dark\n    light: solarized-light\n")
	assert.Equal(t, config.ThemeSlots{Dark: "gruvbox-dark", Light: "solarized-light"}, cfg.UI.ThemeSlots)
	assert.Empty(t, cfg.Warnings)

	half := loadWithUI(t, "  theme:\n    dark: gruvbox-dark\n")
	assert.Equal(t, config.ThemeSlots{Dark: "gruvbox-dark"}, half.UI.ThemeSlots,
		"the unnamed slot keeps the built-in")
	assert.Empty(t, half.Warnings)
}

// Every config tele ever wrote carries ui.theme: default. It named a pair that
// no longer exists, so it must go on meaning what it did — both slots built-in —
// and say so once.
func TestTheme_LegacyDefaultIsIgnoredWithAWarning(t *testing.T) {
	cfg := loadWithUI(t, "  theme: default\n")

	assert.Equal(t, config.ThemeSlots{}, cfg.UI.ThemeSlots)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "tele-dark")
	assert.NotEmpty(t, cfg.Warnings[0].ID,
		"a dead key changes nothing when removed, so it is said once, not every launch")
}

func TestTheme_UnknownSlotWarns(t *testing.T) {
	cfg := loadWithUI(t, "  theme:\n    medium: gruvbox\n")

	assert.Equal(t, config.ThemeSlots{}, cfg.UI.ThemeSlots)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "medium")
	assert.Empty(t, cfg.Warnings[0].ID, "a slot that does not exist is still wrong next launch")
}

func TestTheme_WrongTypeWarns(t *testing.T) {
	cfg := loadWithUI(t, "  theme: 42\n")

	assert.Equal(t, config.ThemeSlots{}, cfg.UI.ThemeSlots)
	require.Len(t, cfg.Warnings, 1)
}

// loadWithUI loads a minimal config with the given ui: section body.
func loadWithUI(t *testing.T, uiBody string) *config.Config {
	t.Helper()
	f := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\nui:\n"+uiBody), 0600))
	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	return cfg
}

func TestDefaults_Toasts(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "bottom-right", cfg.UI.Toasts.ErrorZone)
	assert.Equal(t, "top-right", cfg.UI.Toasts.NotifyZone)
	assert.Equal(t, 3, cfg.UI.Toasts.MaxVisible)
}

func TestDefaults_PhotosDiskCacheSize(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(256*1024*1024), cfg.Photos.DiskCacheSize)
}

// Avatars have a budget of their own, so a busy chat cannot evict the faces of
// the people in your chat list (#223, ADR 0007).
func TestDefaults_AvatarsDiskCacheSize(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(16*1024*1024), cfg.Avatars.DiskCacheSize)
}

func TestLoad_AvatarsDiskCacheSizeIsSetApartFromPhotos(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(
		"telegram:\n  api_id: 1\n  api_hash: x\nphotos:\n  disk_cache_size: 1024\navatars:\n  disk_cache_size: 2048\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, int64(1024), cfg.Photos.DiskCacheSize)
	assert.Equal(t, int64(2048), cfg.Avatars.DiskCacheSize)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yml", t.TempDir())
	assert.Error(t, err)
}

func TestLoad_KeybindingsScalarAndSequence(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(`
telegram:
  api_id: 1
  api_hash: x
keybindings:
  chat:
    reply: "R"
    go_top: ["g g", "gg"]
`), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)

	ov := cfg.KeybindingOverrides()
	assert.Equal(t, []string{"R"}, ov["chat"]["reply"])
	assert.Equal(t, []string{"g g", "gg"}, ov["chat"]["go_top"])
}

func TestLoad_StateDirIsCanonical(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(
		"telegram:\n  api_id: 1\n  api_hash: h\nstate_dir: /custom/state\n"), 0600))

	cfg, err := config.Load(f, "/default/state")
	require.NoError(t, err)

	assert.Equal(t, "/custom/state", cfg.StateDir)
	assert.Equal(t, filepath.Join("/custom/state", "session.json"), cfg.Telegram.SessionFile)
	assert.False(t, cfg.SessionPinned)
	assert.Empty(t, cfg.Warnings)
}

func TestLoad_StateDirDefaultsToPlatformDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(
		"telegram:\n  api_id: 1\n  api_hash: h\n"), 0600))

	cfg, err := config.Load(f, "/default/state")
	require.NoError(t, err)

	assert.Equal(t, "/default/state", cfg.StateDir)
	assert.Equal(t, filepath.Join("/default/state", "session.json"), cfg.Telegram.SessionFile)
	assert.False(t, cfg.SessionPinned)
}

func TestLoad_SessionFilePinsItsOwnDirectoryAndWarns(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(
		"telegram:\n  api_id: 1\n  api_hash: h\n  session_file: /vault/tg.json\n"), 0600))

	cfg, err := config.Load(f, "/default/state")
	require.NoError(t, err)

	// StateDir comes from filepath.Dir, so it carries the native separator.
	assert.Equal(t, filepath.FromSlash("/vault"), cfg.StateDir)
	assert.Equal(t, "/vault/tg.json", cfg.Telegram.SessionFile)
	assert.True(t, cfg.SessionPinned, "a deliberate session path must not be migrated away")
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "session_file is deprecated")
}

func TestLoad_StateDirWinsOverSessionFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte(
		"telegram:\n  api_id: 1\n  api_hash: h\n  session_file: /vault/tg.json\nstate_dir: /custom/state\n"), 0600))

	cfg, err := config.Load(f, "/default/state")
	require.NoError(t, err)

	assert.Equal(t, "/custom/state", cfg.StateDir)
	assert.Equal(t, filepath.Join("/custom/state", "session.json"), cfg.Telegram.SessionFile)
	assert.False(t, cfg.SessionPinned)
	require.Len(t, cfg.Warnings, 1)
	assert.Contains(t, cfg.Warnings[0].Text, "ignored")
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(home, "x", "y"), config.ExpandTilde("~/x/y"))
	assert.Equal(t, "/absolute/path", config.ExpandTilde("/absolute/path"))
	assert.Equal(t, "relative/path", config.ExpandTilde("relative/path"))
}

func TestKeybindingOverrides_AbsentSectionIsNil(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f, t.TempDir())
	require.NoError(t, err)
	assert.Nil(t, cfg.KeybindingOverrides())
}
