// Package statedir owns the directory holding one account's state: the
// Telegram session, the SQLite database, and the lock that keeps a second
// process out of both.
package statedir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrLocked reports that another process holds the state directory.
var ErrLocked = errors.New("state directory is locked by another process")

// lockFileName is the lock's fixed name inside the state directory.
const lockFileName = "tele.lock"

// Lock is an exclusive advisory lock on a state directory.
//
// The OS drops it when the process dies, so unlike a pid file there is no
// stale-lock recovery path and no kill -9 edge case. Release exists so tests
// and the shutdown path can be explicit, not because correctness needs it.
type Lock struct {
	f *os.File
}

// Acquire takes the lock on dir, creating dir when missing. When another
// process holds it, the returned error wraps ErrLocked and names the holding
// pid if the lock file records a usable one.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, lockFileName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockFile(f); err != nil {
		pid := readPID(f)
		_ = f.Close()
		if pid > 0 {
			return nil, fmt.Errorf("%w (pid %d)", ErrLocked, pid)
		}
		return nil, ErrLocked
	}
	writePID(f)
	return &Lock{f: f}, nil
}

// Release drops the lock and closes the file. Safe to call more than once.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := unlockFile(l.f)
	if cerr := l.f.Close(); err == nil {
		err = cerr
	}
	l.f = nil
	return err
}

// writePID records the owning pid so a losing process can name the holder.
// Best effort only: the lock itself arbitrates, the pid is for the message.
func writePID(f *os.File) {
	if err := f.Truncate(0); err != nil {
		return
	}
	_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0)
}

// readPID reads a pid previously written by writePID. Returns 0 when the file
// is empty or holds anything unparsable, so a corrupted lock file degrades to
// a message without a pid rather than to a failure.
func readPID(f *os.File) int {
	buf := make([]byte, 32)
	n, _ := f.ReadAt(buf, 0)
	if n <= 0 {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		return 0
	}
	return pid
}
