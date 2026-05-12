package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/config"
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

	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, 12345, cfg.Telegram.APIID)
	assert.Equal(t, "abc", cfg.Telegram.APIHash)
	assert.Equal(t, "/tmp/session.json", cfg.Telegram.SessionFile)
	assert.Equal(t, "15:04", cfg.UI.DateFormat)
	assert.Equal(t, 100, cfg.UI.HistoryLimit)
}

func TestLoad_SessionFileDerivedFromConfigDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "session.json"), cfg.Telegram.SessionFile)
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(f, []byte("telegram:\n  api_id: 1\n  api_hash: x\n"), 0600))

	cfg, err := config.Load(f)
	require.NoError(t, err)
	assert.Equal(t, 50, cfg.UI.HistoryLimit)
	assert.Equal(t, "15:04", cfg.UI.DateFormat)
	assert.Equal(t, "default", cfg.UI.Theme)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yml")
	assert.Error(t, err)
}
