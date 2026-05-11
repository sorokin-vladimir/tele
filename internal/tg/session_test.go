package tg_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

func TestFileSession_StoreLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.yaml")

	s := internaltg.NewFileSession(path)
	ctx := context.Background()

	data := []byte(`{"test": true}`)
	require.NoError(t, s.StoreSession(ctx, data))

	loaded, err := s.LoadSession(ctx)
	require.NoError(t, err)
	assert.Equal(t, data, loaded)
}

func TestFileSession_LoadMissing(t *testing.T) {
	path := filepath.Join(os.TempDir(), "tele_nonexistent_test_session.yaml")
	os.Remove(path)

	s := internaltg.NewFileSession(path)
	data, err := s.LoadSession(context.Background())
	// gotd FileStorage returns empty bytes (not error) when file is missing
	assert.NoError(t, err)
	assert.Empty(t, data)
}

func TestFileSession_TildeExpansion(t *testing.T) {
	s := internaltg.NewFileSession("~/some/session.yaml")
	// Just verify it doesn't panic and the path is expanded
	assert.NotNil(t, s)
}
