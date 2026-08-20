package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// applyModel builds a model over a real config file, wired to reload from it the
// way the app wires it. Everything here goes through the file, because that is
// the only way a change reaches a running tele and the point is to test that.
func applyModel(t *testing.T, body string) (RootModel, *config.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"+body), 0600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	t.Cleanup(func() { theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight}) })

	m := NewRootModel(nil, 50, false).
		WithConfig(store.Current()).
		WithConfigReload(func() (*config.Config, error) {
			if err := store.Reload(); err != nil {
				return nil, err
			}
			return store.Current(), nil
		})
	m.width, m.height = 100, 40
	m.toasts.SetSize(100, 40)
	return m, store
}

// liveObservation is how a setting is seen to have taken effect, for the
// settings whose value is copied out of the config into something else. These
// are the ones that can silently keep the old value after a reload: a setting
// read where it is used follows a reload for free, a copy does not.
type liveObservation struct {
	// value is what to write into the file.
	value any
	// observe reads back what the model or the app is actually running on -
	// never the config, which would only prove the file was re-read.
	observe func(RootModel) any
	want    any
}

var liveObservations = map[string]liveObservation{
	"ui.toasts.error_zone": {
		value:   "top-right",
		observe: func(m RootModel) any { return m.toasts.ZoneOf(components.ToastError) },
		want:    components.ZoneTopRight,
	},
	"ui.toasts.notify_zone": {
		value:   "bottom-right",
		observe: func(m RootModel) any { return m.toasts.ZoneOf(components.ToastNotify) },
		want:    components.ZoneBottomRight,
	},
	"ui.toasts.max_visible": {
		value:   7,
		observe: func(m RootModel) any { return m.toasts.MaxVisible() },
		want:    7,
	},
	"ui.history_limit": {
		value:   111,
		observe: func(m RootModel) any { return m.historyLimit },
		want:    111,
	},
	"ui.theme": {
		value:   "nord",
		observe: func(RootModel) any { return theme.T().Name },
		want:    "nord",
	},
	"photos.kitty_placement_cap": {
		value:   33,
		observe: func(m RootModel) any { return m.kittyCap },
		want:    33,
	},
	"photos.max_long_side_px": {
		value:   1234,
		observe: func(m RootModel) any { return m.chat.MaxMediaPx() },
		want:    1234,
	},
}

// readAtPointOfUse names the settings that need no observation because nothing
// copies them: the code that acts on them reads the config where it acts, so
// installing the new config is the whole of applying them.
var readAtPointOfUse = map[string]bool{
	"ui.notification_preview":   true, // internal/core reads o.Config() as it decides
	"photos.eager_full_quality": true, // root_download.go reads m.cfg as it downloads
}

// The guard on the whole idea. A setting declared to take effect without a
// restart, and then quietly not taking effect, is worse than one that says it
// needs a restart: the person changes it, sees nothing, and concludes the
// setting is broken.
//
// Adding a live setting fails this test until it is either observed here or
// declared to be read where it is used.
func TestLiveSettings_AreEachAccountedFor(t *testing.T) {
	for _, e := range config.Settings() {
		if e.ReadOnly || e.Applies == settings.Startup {
			continue
		}
		_, observed := liveObservations[e.Key]
		assert.True(t, observed || readAtPointOfUse[e.Key],
			"%s takes effect without a restart and nothing here checks that it does", e.Key)
	}
	for key := range liveObservations {
		e, ok := config.Setting(key)
		require.True(t, ok, "%s is observed here and is not a setting", key)
		assert.NotEqual(t, settings.Startup, e.Applies, "%s is a startup setting; observing it after a reload proves nothing", key)
	}
}

// Each live setting, changed in the file and reloaded, reaches the thing that
// acts on it - not merely the config.
func TestLiveSettings_TakeEffectOnReload(t *testing.T) {
	for key, obs := range liveObservations {
		t.Run(key, func(t *testing.T) {
			m, store := applyModel(t, "")
			require.NotEqual(t, obs.want, obs.observe(m), "%s already looks applied; the test proves nothing", key)

			require.NoError(t, store.Set(key, obs.value))
			m, _ = m.reloadFromDisk()

			assert.Equal(t, obs.want, obs.observe(m))
		})
	}
}

// A person editing the file in another window gets the same result as a person
// using the overlay, because both arrive the same way.
func TestReload_PicksUpAnEditMadeOutsideTheApp(t *testing.T) {
	m, store := applyModel(t, "ui:\n  history_limit: 50\n")

	require.NoError(t, os.WriteFile(store.Path(), []byte(
		"telegram:\n  api_id: 1\n  api_hash: x\nui:\n  history_limit: 175\n"), 0600))
	m, _ = m.reloadFromDisk()

	assert.Equal(t, 175, m.historyLimit)
	assert.Equal(t, 175, m.cfg.UI.HistoryLimit)
}

// A startup setting is written and kept, and the running app goes on with what
// it started with. Swapping the renderer under images already transmitted to the
// terminal is not a setting taking effect, it is a mess.
func TestReload_StartupSettingDoesNotChangeTheRunningApp(t *testing.T) {
	m, store := applyModel(t, "photos:\n  mode: blocks\n")
	require.Equal(t, media.ModeBlocks, m.imageMode)

	require.NoError(t, store.Set("photos.mode", "kitty"))
	m, _ = m.reloadFromDisk()

	assert.Equal(t, media.ModeBlocks, m.imageMode, "still what it started with")
	assert.Equal(t, "kitty", m.cfg.Photos.Mode, "and the change is kept for next time")
}

// Toasts already on screen move with the setting. Leaving them where they were
// would draw the stack in two corners at once until they expired, which reads as
// a bug rather than as a setting taking effect.
func TestReload_MovingTheZoneTakesTheToastsAlreadyShowing(t *testing.T) {
	m, store := applyModel(t, "")
	m.toasts.Add(components.ToastError, "something went wrong")
	require.Equal(t, components.ZoneBottomRight, m.toasts.ZoneOf(components.ToastError))

	require.NoError(t, store.Set("ui.toasts.error_zone", "top-right"))
	m, _ = m.reloadFromDisk()

	m.SettleToastsForTest()
	zones := m.toasts.Zones()
	require.NotEmpty(t, zones, "the toast is still on screen")
	// The top-right zone is stamped one row down from the top edge; the
	// bottom-right one would be near row 40 in a 40-row viewport.
	assert.Equal(t, 1, zones[0].Top, "and it has moved up with the setting")
}

// A reload that fails leaves the app running on what it had. A config that
// stopped parsing is a reason to say so, not a reason to lose the settings that
// were working.
func TestReload_AFailureKeepsTheRunningConfigAndSaysSo(t *testing.T) {
	m, store := applyModel(t, "ui:\n  history_limit: 120\n")
	require.NoError(t, os.WriteFile(store.Path(), []byte("ui:\n  history_limit: 50\n   theme: nord\n"), 0600))

	m, cmd := m.reloadFromDisk()

	assert.Equal(t, 120, m.historyLimit, "still running on what it had")
	require.NotNil(t, cmd)
	m.SettleToastsForTest()
	zones := m.toasts.Zones()
	require.NotEmpty(t, zones)
	assert.Contains(t, toastText(zones[0].Block), "not reloaded")
}

// The overlay writes and the reload applies, so a value set through the store
// reaches the running model without anybody editing anything by hand.
func TestReload_AppliesAValueWrittenThroughTheStore(t *testing.T) {
	m, store := applyModel(t, "")

	require.NoError(t, store.Set("ui.toasts.max_visible", 9))
	m, _ = m.reloadFromDisk()

	assert.Equal(t, 9, m.toasts.MaxVisible())
}

// Resetting a setting is writing absence, and absence applies like any other
// value: the model goes back to the default.
func TestReload_ResettingASettingAppliesTheDefault(t *testing.T) {
	m, store := applyModel(t, "ui:\n  history_limit: 175\n")
	require.Equal(t, 175, m.historyLimit)

	require.NoError(t, store.Set("ui.history_limit", nil))
	m, _ = m.reloadFromDisk()

	assert.Equal(t, 50, m.historyLimit)
}
