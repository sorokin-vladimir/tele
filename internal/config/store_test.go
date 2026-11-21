package config_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// A layout the app cannot use is refused before it reaches the file, exactly
// as Load would repair it afterwards: the overlay and a hand edit are judged
// by the same rule (#238).
func TestStore_SetRefusesAnUnusableDateFormat(t *testing.T) {
	s, _ := storeAt(t, "")

	assert.Error(t, s.Set("ui.date_format", ""))
	assert.Error(t, s.Set("ui.date_format", 1504))
	assert.Error(t, s.Set("ui.date_format", "100%"))
	assert.Equal(t, "15:04", s.Current().UI.DateFormat, "nothing was written, so the default stands")
}

func TestStore_SetAcceptsADateFormatLayout(t *testing.T) {
	s, path := storeAt(t, "")

	require.NoError(t, s.Set("ui.date_format", "02.01.2006 15:04"))

	assert.Equal(t, "02.01.2006 15:04", s.Current().UI.DateFormat, "the running config")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "date_format:", "the key is in the file")
	assert.Contains(t, string(raw), "02.01.2006 15:04", "the layout is in the file")
	v, status := s.Value("ui.date_format")
	assert.Equal(t, settings.Saved, status)
	assert.Equal(t, "02.01.2006 15:04", v)
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

	assert.Error(t, s.Set("ui.nonsense", 1))
}

// A value nobody chose must not read as a choice. The question is whether the
// file names the setting, not whether the value happens to equal the default:
// somebody who wrote the default explicitly did choose it.
func TestStore_IsDefaultAsksWhetherTheFileNamesTheSetting(t *testing.T) {
	s, _ := storeAt(t, "ui:\n  history_limit: 50\n  toasts:\n    max_visible: 4\n")

	assert.False(t, s.IsDefault("ui.history_limit"), "written, even though it equals the default")
	assert.False(t, s.IsDefault("ui.toasts.max_visible"), "written and different")
	assert.True(t, s.IsDefault("ui.notifications.preview"), "not in the file at all")
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
		func() error { return s.Set("ui.notifications.preview", false) },
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
	assert.False(t, s.Current().UI.Notifications.Preview)
	assert.Equal(t, 1200, s.Current().Photos.MaxLongSidePx)
}

// The host and a window of another process often edit one file, each its own
// settings. Neither may write a file read before the other's change landed, or
// that change is gone (#315). Two stores on one file stand in for the two
// processes: the lock is on an open file, so they contend for it as processes
// would.
func TestStore_TwoStoresOnOneFileNeverLoseEachOthersChange(t *testing.T) {
	host, path := storeAt(t, "")
	window, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)

	// Both keys take every value in (first, last].
	const first, last = 100, 200
	var wg sync.WaitGroup
	write := func(s *config.Store, key string, read func(*config.Config) int) {
		defer wg.Done()
		for i := first + 1; i <= last; i++ {
			if !assert.NoError(t, s.Set(key, i)) {
				return
			}
			// Once Set has returned the value is in the file, and only this
			// writer changes this key, so the file may never say less.
			cfg, err := config.Load(path, t.TempDir())
			if !assert.NoError(t, err) || !assert.GreaterOrEqual(t, read(cfg), i, "%s went back", key) {
				return
			}
		}
	}
	wg.Add(2)
	go write(host, "ui.history_limit", func(c *config.Config) int { return c.UI.HistoryLimit })
	go write(window, "photos.max_long_side_px", func(c *config.Config) int { return c.Photos.MaxLongSidePx })
	wg.Wait()

	cfg, err := config.Load(path, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, last, cfg.UI.HistoryLimit)
	assert.Equal(t, last, cfg.Photos.MaxLongSidePx)
}

// A value that crossed a connection as JSON comes back as the Go value the
// setting holds, so it is written and validated exactly as one set here would
// be: a number is an int, never a float64 (#315).
func TestDecodeValue_GivesTheSettingsOwnType(t *testing.T) {
	cases := []struct {
		key, raw string
		want     any
	}{
		{"ui.history_limit", "120", 120},
		{"ui.notifications.desktop", "false", false},
		{"photos.disk_cache_size", "1048576", int64(1 << 20)},
		{"proxy.server", `"example.org"`, "example.org"},
	}
	for _, c := range cases {
		got, err := config.DecodeValue(c.key, json.RawMessage(c.raw))
		require.NoError(t, err, c.key)
		assert.Equal(t, c.want, got, c.key)
	}
}

// Nothing is a reset, which is a value of its own: absence.
func TestDecodeValue_NothingIsAReset(t *testing.T) {
	for _, raw := range []json.RawMessage{nil, json.RawMessage("null")} {
		got, err := config.DecodeValue("ui.history_limit", raw)
		require.NoError(t, err)
		assert.Nil(t, got)
	}
}

func TestDecodeValue_RefusesWhatTheSettingCannotHold(t *testing.T) {
	_, err := config.DecodeValue("ui.history_limit", json.RawMessage(`"many"`))
	assert.Error(t, err)
	_, err = config.DecodeValue("ui.nonexistent", json.RawMessage("1"))
	assert.Error(t, err)
}

// childEdit names the file and the key a child process edits; set only in the
// processes TestStore_TwoProcessesOnOneFileNeverLoseEachOthersChange starts.
const childEdit = "TELE_CONFIG_CHILD_EDIT"

// The same edits from real processes, which is the case in the wild: the lock
// is the operating system's here, not one process's file table.
func TestStore_TwoProcessesOnOneFileNeverLoseEachOthersChange(t *testing.T) {
	if testing.Short() {
		t.Skip("starts processes")
	}
	_, path := storeAt(t, "")
	keys := []string{"ui.history_limit", "photos.max_long_side_px"}
	children := make([]*exec.Cmd, len(keys))
	outs := make([]*strings.Builder, len(keys))
	for i, key := range keys {
		cmd := exec.Command(os.Args[0], "-test.run=^TestStoreChild$")
		cmd.Env = append(os.Environ(), childEdit+"="+path+"|"+key)
		outs[i] = &strings.Builder{}
		cmd.Stdout, cmd.Stderr = outs[i], outs[i]
		children[i] = cmd
	}
	for _, c := range children {
		require.NoError(t, c.Start())
	}
	for i, c := range children {
		assert.NoError(t, c.Wait(), "%s: %s", keys[i], outs[i])
	}

	cfg, err := config.Load(path, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, childLast, cfg.UI.HistoryLimit)
	assert.Equal(t, childLast, cfg.Photos.MaxLongSidePx)
}

// childLast is the last value a child writes; it writes every one from 101 on.
const childLast = 200

// TestStoreChild is one process of the edits above: it writes its key over and
// over, and fails the moment the file says less than it last wrote.
func TestStoreChild(t *testing.T) {
	edit := os.Getenv(childEdit)
	if edit == "" {
		t.Skip("run only as a child of the edits across processes")
	}
	path, key, _ := strings.Cut(edit, "|")
	s, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	for i := 101; i <= childLast; i++ {
		require.NoError(t, s.Set(key, i))
		cfg, err := config.Load(path, t.TempDir())
		require.NoError(t, err)
		got := cfg.UI.HistoryLimit
		if key != "ui.history_limit" {
			got = cfg.Photos.MaxLongSidePx
		}
		require.GreaterOrEqual(t, got, i, "%s went back", key)
	}
}
