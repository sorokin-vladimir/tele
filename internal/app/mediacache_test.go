package app

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/accountstate"
)

// Two accounts must not share a cache directory: with one process per state
// directory (#188), a shared directory means two independent indexes evicting
// each other's files.
func TestAccountSegment_DiffersPerStateDir(t *testing.T) {
	a := accountstate.Segment("/home/u/.local/state/tele")
	b := accountstate.Segment("/home/u/.local/state/tele-work")

	assert.NotEqual(t, a, b)
	assert.Len(t, a, 12)
}

func TestAccountSegment_IsStableForOneStateDir(t *testing.T) {
	assert.Equal(t, accountstate.Segment("/home/u/state"), accountstate.Segment("/home/u/state"))
}

func TestMediaCacheDir_SitsUnderTheAccountSegment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("os.UserCacheDir ignores XDG_CACHE_HOME outside Linux")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	dir, err := accountstate.MediaCacheDir("/home/u/state")

	require.NoError(t, err)
	assert.Equal(t, "media", filepath.Base(dir))
	assert.Equal(t, accountstate.Segment("/home/u/state"), filepath.Base(filepath.Dir(dir)))
	assert.Equal(t, "tele", filepath.Base(filepath.Dir(filepath.Dir(dir))))
}
