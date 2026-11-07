package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/ipc/wire"
	"github.com/sorokin-vladimir/tele/internal/settings"
)

// hostEndAt is the host's end over a config file with body, and the configs it
// has handed on to be applied.
func hostEndAt(t *testing.T, body string) (*HostEnd, string, *[]*config.Config) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"+body), 0o600))
	store, err := config.NewStore(path, t.TempDir())
	require.NoError(t, err)
	e := NewHostEnd(store, testHello)
	var applied []*config.Config
	e.ApplyTo(func(cfg *config.Config) { applied = append(applied, cfg) })
	return e, path, &applied
}

func settingOf(t *testing.T, s wire.HostSettings, key string) wire.Setting {
	t.Helper()
	for _, it := range s.Settings {
		if it.Key == key {
			return it
		}
	}
	require.FailNow(t, "no such setting in the answer", key)
	return wire.Setting{}
}

// The host answers with every one of its own settings and none of a client's,
// each with its value and whether the file names it, and with the file it
// reads them from (#315).
func TestHostEnd_SettingsAreTheHostsOwn(t *testing.T) {
	e, path, _ := hostEndAt(t, "ui:\n  history_limit: 120\n")

	got := e.Settings()

	assert.Equal(t, path, got.Config)
	var keys []string
	for _, s := range got.Settings {
		keys = append(keys, s.Key)
	}
	for _, entry := range config.Settings() {
		if entry.Process == settings.Host {
			assert.Contains(t, keys, entry.Key)
		} else {
			assert.NotContains(t, keys, entry.Key)
		}
	}
	limit := settingOf(t, got, "ui.history_limit")
	assert.JSONEq(t, "120", string(limit.Value))
	assert.False(t, limit.Default)
	assert.True(t, settingOf(t, got, "ui.notifications.desktop").Default)
}

// A change is written into the host's own file and handed on to be applied.
func TestHostEnd_SetWritesTheHostsFileAndAppliesIt(t *testing.T) {
	e, path, applied := hostEndAt(t, "")

	got := e.SetSetting(wire.SetParams{Key: "ui.history_limit", Value: json.RawMessage("120")})

	assert.Empty(t, got.Refused)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "history_limit: 120")
	require.Len(t, *applied, 1)
	assert.Equal(t, 120, (*applied)[0].UI.HistoryLimit)
}

// A client's setting is never the host's to write: it is the window's, in the
// window's own file.
func TestHostEnd_SetRefusesAClientsSetting(t *testing.T) {
	e, path, applied := hostEndAt(t, "")
	before, err := os.ReadFile(path)
	require.NoError(t, err)

	got := e.SetSetting(wire.SetParams{Key: "ui.theme", Value: json.RawMessage(`"nord"`)})

	assert.NotEmpty(t, got.Refused)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
	assert.Empty(t, *applied)
}

// A value the setting will not take is refused in the words the row shows,
// and nothing is written.
func TestHostEnd_SetRefusesAnIllegalValue(t *testing.T) {
	e, _, applied := hostEndAt(t, "")

	got := e.SetSetting(wire.SetParams{Key: "ui.history_limit", Value: json.RawMessage("0")})

	assert.Contains(t, got.Refused, "at least 1")
	assert.Empty(t, *applied)
}

// Nothing resets the setting to its default.
func TestHostEnd_SetNothingResets(t *testing.T) {
	e, _, applied := hostEndAt(t, "ui:\n  history_limit: 120\n")

	got := e.SetSetting(wire.SetParams{Key: "ui.history_limit"})

	assert.Empty(t, got.Refused)
	require.Len(t, *applied, 1)
	assert.Equal(t, 50, (*applied)[0].UI.HistoryLimit)
	assert.True(t, settingOf(t, e.Settings(), "ui.history_limit").Default)
}

// A reload takes what the file says now, hands it on, and answers with what is
// wrong with the host's settings in it, not with a client's.
func TestHostEnd_ReloadAppliesTheFileAndSaysWhatIsWrongWithTheHostsPart(t *testing.T) {
	e, path, applied := hostEndAt(t, "")
	require.NoError(t, os.WriteFile(path, []byte(
		"telegram:\n  api_id: 1\n  api_hash: x\nui:\n  history_limit: 0\n  toasts:\n    error_zone: bottom-middle\n"), 0o600))

	got := e.Reload()

	assert.Empty(t, got.Failed)
	require.Len(t, got.Warnings, 1)
	assert.Contains(t, got.Warnings[0].Text, "ui.history_limit")
	require.Len(t, *applied, 1)
}

// A file that no longer loads leaves the host on the settings it had, and
// says why.
func TestHostEnd_AReloadOfABrokenFileKeepsWhatWasRunning(t *testing.T) {
	e, path, applied := hostEndAt(t, "")
	require.NoError(t, os.WriteFile(path, []byte("telegram: [\n"), 0o600))

	got := e.Reload()

	assert.NotEmpty(t, got.Failed)
	assert.Empty(t, *applied)
}

// cli.account_commands is the host's, and takes hold at once: commands may do
// everything by default, and once the host reads read, the next one is held
// to reading (#181).
func TestTakesHold_cli_account_commands(t *testing.T) {
	e, path, _ := hostEndAt(t, "")
	require.Equal(t, wire.CommandsAll, e.AccountCommands(), "commands may do everything by default")

	read := "telegram:\n  api_id: 1\n  api_hash: x\ncli:\n  account_commands: read\n"
	require.NoError(t, os.WriteFile(path, []byte(read), 0o600))
	require.Empty(t, e.Reload().Failed)

	assert.Equal(t, wire.CommandsRead, e.AccountCommands())
}

// A value the setting does not take is put back to all, as any setting's
// illegal value is to its default, and said in a warning.
func TestHostEnd_AnUnknownAccountCommandsIsAll(t *testing.T) {
	e, _, _ := hostEndAt(t, "cli:\n  account_commands: sometimes\n")

	assert.Equal(t, wire.CommandsAll, e.AccountCommands())
}

// Hello says what was wrong with the host's settings when it started, and
// nothing of a client's.
func TestHostEnd_HelloCarriesTheHostsWarningsOnly(t *testing.T) {
	e, _, _ := hostEndAt(t, "ui:\n  history_limit: 0\n  toasts:\n    error_zone: bottom-middle\n")

	got := e.Hello().Warnings

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Text, "ui.history_limit")
}
