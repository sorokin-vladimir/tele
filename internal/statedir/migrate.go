package statedir

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/zap"
)

// migratedFiles lists what moves, in move order. session.json is deliberately
// last: its presence in the destination is the "already migrated" marker, so
// moving it first would let a crash strand the database in the old location
// while the app came up looking healthy.
var migratedFiles = []string{"state.db", "state.db-wal", "state.db-shm", "session.json"}

// Migrate moves account state from legacyDir into dir, once, and reports
// whether anything was actually relocated. It is idempotent and resumable:
// re-running after a crash finishes the job.
//
// The caller must already hold the directory lock. Nothing may have state.db
// open while its file moves.
func Migrate(dir, legacyDir string, log *zap.Logger) (bool, error) {
	if dir == legacyDir {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(dir, "session.json")); err == nil {
		return false, nil
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "session.json")); err != nil {
		return false, nil //nolint:nilerr // no legacy session means nothing to migrate
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("create state dir: %w", err)
	}
	moved := false
	for _, name := range migratedFiles {
		src := filepath.Join(legacyDir, name)
		if _, err := os.Stat(src); err != nil {
			continue // -wal and -shm exist only after an unclean shutdown
		}
		if err := moveFile(src, filepath.Join(dir, name)); err != nil {
			return moved, fmt.Errorf("migrate %s: %w", name, err)
		}
		moved = true
		log.Info("migrated state file",
			zap.String("name", name),
			zap.String("from", legacyDir),
			zap.String("to", dir))
	}
	return moved, nil
}

// moveFile renames src to dst, falling back to copy-then-remove when the two
// paths are on different filesystems (a config directory on one volume and a
// state directory on another is unusual but not impossible).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile writes src to dst with 0600 permissions and fsyncs before returning,
// so a power loss cannot leave a truncated destination that the caller has
// already removed the source for.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // opened for reading, closing cannot lose data

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
