package config

import (
	"os"
	"path/filepath"
)

// lockEdits holds the edit lock of the config file at path until the returned
// function is called. Every edit of the file happens under it, in whichever
// process: the host and a window of another tele often edit one file, each its
// own settings, and a read-modify-write that read the file before the other's
// write landed would put the file back without it (#315).
//
// The lock is on a file of its own beside the config, not on the config: an
// edit replaces the config file by renaming a new one over it, and a lock on
// the old file would not be seen by whoever opens the new one. The name is
// hidden, because the directory is the person's and the file means nothing to
// them.
//
// Reading the file takes the lock too. The file is replaced whole, so a reader
// never sees half of one version, but on Windows a file being renamed over
// cannot be opened at all: a read that lands in another process's edit fails
// instead of waiting for it. The lock is not reentrant, so nothing reads the
// file while holding it.
func lockEdits(path string) (unlock func(), err error) {
	f, err := os.OpenFile(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = unlockFile(f)
		_ = f.Close()
	}, nil
}
