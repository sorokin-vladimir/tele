package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On Windows a file being renamed over cannot be opened, so a read that lands
// in the middle of another process's edit fails. A read therefore waits for
// the edit to finish, on every platform alike.
func TestLoad_WaitsForAnEditInProgress(t *testing.T) {
	path := fixture(t)
	unlock, err := lockEdits(path)
	require.NoError(t, err)

	loaded := make(chan error, 1)
	go func() {
		_, err := Load(path, t.TempDir())
		loaded <- err
	}()

	select {
	case err := <-loaded:
		unlock()
		t.Fatalf("Load returned while an edit held the file (err = %v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	unlock()

	select {
	case err := <-loaded:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Load did not return once the edit was done")
	}
}

// A config in a directory nobody may write to, such as one a system
// configuration manages, still loads: with no lock file to be had there, no
// edit can be replacing the file either.
func TestLoad_ReadsAConfigInADirectoryThatCannotBeWritten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory's mode bits do not stop writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root writes to any directory")
	}
	path := fixture(t)
	dir := filepath.Dir(path)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := Load(path, t.TempDir())

	assert.NoError(t, err)
}
