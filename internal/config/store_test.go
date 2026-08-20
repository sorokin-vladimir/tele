package config_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/settings"
)

func storeAt(t *testing.T, body string) (*config.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"+body), 0600))
	s, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	return s, path
}

func TestStore_SetWritesTheFileAndSwapsTheConfig(t *testing.T) {
	s, path := storeAt(t, "ui:\n  history_limit: 50\n")

	require.NoError(t, s.Set("ui.history_limit", 120))

	assert.Equal(t, 120, s.Current().UI.HistoryLimit, "the running config")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "history_limit: 120", "and the file")
}

// The criterion the whole design rests on. It holds structurally rather than by
// agreement: both paths end in the same read of the same file, so there is no
// second way for them to disagree.
func TestStore_AHandEditAndAnOverlayEditProduceTheSameConfig(t *testing.T) {
	byHand, handPath := storeAt(t, "ui:\n  history_limit: 50\n")
	byOverlay, _ := storeAt(t, "ui:\n  history_limit: 50\n")

	require.NoError(t, os.WriteFile(handPath, []byte(
		"telegram:\n  api_id: 1\n  api_hash: x\nui:\n  history_limit: 120\n  toasts:\n    error_zone: top-right\n"), 0600))
	require.NoError(t, byHand.Reload())

	require.NoError(t, byOverlay.Set("ui.history_limit", 120))
	require.NoError(t, byOverlay.Set("ui.toasts.error_zone", "top-right"))

	assert.Equal(t, byHand.Current().UI, byOverlay.Current().UI)
	assert.Equal(t, byHand.Current().Photos, byOverlay.Current().Photos)
}

// Resetting is writing absence. What comes back is the default, and the file no
// longer carries the key at all.
func TestStore_SetNilResetsToTheDefault(t *testing.T) {
	s, path := storeAt(t, "ui:\n  history_limit: 120\n")

	require.NoError(t, s.Set("ui.history_limit", nil))

	assert.Equal(t, 50, s.Current().UI.HistoryLimit)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "history_limit")
}

// The refusal happens before the file is opened. An illegal value never reaches
// the disk, so nothing has to be repaired on the way back in.
func TestStore_SetRefusesAnIllegalValueWithoutTouchingTheFile(t *testing.T) {
	s, path := storeAt(t, "ui:\n  toasts:\n    error_zone: top-right\n")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	err = s.Set("ui.toasts.error_zone", "bottom-middle")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "bottom-right", "the refusal says what is allowed")
	after, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, before, after)
	assert.Equal(t, "top-right", s.Current().UI.Toasts.ErrorZone)
}

// A read-only setting is not a dead end: the refusal names the file that holds
// it, which is the whole reason it is shown at all.
func TestStore_SetRefusesAReadOnlySettingAndSaysWhereItLives(t *testing.T) {
	s, path := storeAt(t, "")

	err := s.Set("telegram.api_id", 999)

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
}

func TestStore_SetRefusesAKeyThatIsNotASetting(t *testing.T) {
	s, _ := storeAt(t, "")

	assert.Error(t, s.Set("ui.date_format", "15:04"), "excluded, not declared: #238")
	assert.Error(t, s.Set("ui.nonsense", 1))
}

// A value nobody chose must not read as a choice. The question is whether the
// file names the setting, not whether the value happens to equal the default:
// somebody who wrote the default explicitly did choose it.
func TestStore_IsDefaultAsksWhetherTheFileNamesTheSetting(t *testing.T) {
	s, _ := storeAt(t, "ui:\n  history_limit: 50\n  toasts:\n    max_visible: 4\n")

	assert.False(t, s.IsDefault("ui.history_limit"), "written, even though it equals the default")
	assert.False(t, s.IsDefault("ui.toasts.max_visible"), "written and different")
	assert.True(t, s.IsDefault("ui.notification_preview"), "not in the file at all")
	assert.True(t, s.IsDefault("photos.disk_cache_size"), "not even its section is in the file")
}

// Writing a setting stops it being a default; resetting it makes it one again.
func TestStore_IsDefaultFollowsWritesAndResets(t *testing.T) {
	s, _ := storeAt(t, "")
	require.True(t, s.IsDefault("ui.history_limit"))

	require.NoError(t, s.Set("ui.history_limit", 120))
	assert.False(t, s.IsDefault("ui.history_limit"))

	require.NoError(t, s.Set("ui.history_limit", nil))
	assert.True(t, s.IsDefault("ui.history_limit"))
}

func TestStore_ValueAnswersTheResolvedValue(t *testing.T) {
	s, _ := storeAt(t, "ui:\n  history_limit: 77\n")

	v, status := s.Value("ui.history_limit")
	assert.Equal(t, 77, v)
	assert.Equal(t, settings.Saved, status)

	// Not what the file said - the file said nothing - but where the session
	// actually is, which is what a read-only row is for.
	v, status = s.Value("telegram.session_file")
	assert.Equal(t, settings.Saved, status)
	assert.NotEmpty(t, v)

	_, status = s.Value("ui.nonsense")
	assert.Equal(t, settings.Unknown, status)
}

func TestStore_ValueReadsEveryDeclaredSetting(t *testing.T) {
	s, _ := storeAt(t, "")

	for _, e := range s.Entries() {
		_, status := s.Value(e.Key)
		assert.Equal(t, settings.Saved, status, "%s is declared and cannot be read", e.Key)
	}
}

// A failed reload leaves the app running on the config it had. A file that
// stopped parsing while tele was open is a reason to say so, not a reason to
// lose the settings that were working.
func TestStore_AFailedReloadKeepsTheRunningConfig(t *testing.T) {
	s, path := storeAt(t, "ui:\n  history_limit: 120\n")
	require.NoError(t, os.WriteFile(path, []byte("ui:\n  history_limit: 50\n   theme: nord\n"), 0600))

	require.Error(t, s.Reload())

	assert.Equal(t, 120, s.Current().UI.HistoryLimit)
}

// Two settings changed at once must not each write a file missing the other's
// change: the read-modify-write is serialized.
func TestStore_ConcurrentSetsBothLand(t *testing.T) {
	s, _ := storeAt(t, "")

	var wg sync.WaitGroup
	for _, set := range []func() error{
		func() error { return s.Set("ui.history_limit", 120) },
		func() error { return s.Set("ui.notification_preview", false) },
		func() error { return s.Set("photos.max_long_side_px", 1200) },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, set())
		}()
	}
	wg.Wait()

	require.NoError(t, s.Reload())
	assert.Equal(t, 120, s.Current().UI.HistoryLimit)
	assert.False(t, s.Current().UI.NotificationPreview)
	assert.Equal(t, 1200, s.Current().Photos.MaxLongSidePx)
}
