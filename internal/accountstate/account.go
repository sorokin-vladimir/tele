// Package accountstate owns the files that must not survive an account change.
package accountstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotd/td/session"
)

const accountIDFile = "account.id"

var errNoUserCache = errors.New("user cache directory unavailable")

// CleanupReason describes why account-owned files were removed before startup.
type CleanupReason string

const (
	CleanupIdentityChanged    CleanupReason = "identity changed"
	CleanupNoSession          CleanupReason = "no session"
	CleanupNoRecordedIdentity CleanupReason = "no recorded identity"
)

// Segment is the stable, filename-safe directory name for one state directory's
// persistent caches.
func Segment(stateDir string) string {
	sum := sha256.Sum256([]byte(stateDir))
	return hex.EncodeToString(sum[:])[:12]
}

// MediaCacheDir returns the persistent media cache for stateDir.
func MediaCacheDir(stateDir string) (string, error) {
	return cacheDir(stateDir, "media")
}

// AvatarCacheDir returns the persistent avatar cache for stateDir: a sibling of
// the media directory, never inside it, so the two bounds are enforced over
// disjoint sets of files (#223).
func AvatarCacheDir(stateDir string) (string, error) {
	return cacheDir(stateDir, "avatars")
}

func cacheDir(stateDir, name string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("%w: %v", errNoUserCache, err)
	}
	return filepath.Join(base, "tele", Segment(stateDir), name), nil
}

// Reconcile compares the session's auth key with the identity recorded at the
// last successful sign-in. A missing, unreadable, or different identity clears
// every account-owned file before any of them can be opened.
func Reconcile(stateDir, sessionFile string) (bool, CleanupReason, error) {
	sessionID, sessionErr := sessionIdentity(sessionFile)
	recordedID, recordedErr := recordedIdentity(stateDir)
	if sessionErr == nil && recordedErr == nil && sessionID == recordedID {
		return false, "", nil
	}

	reason := CleanupIdentityChanged
	switch {
	case sessionErr != nil:
		reason = CleanupNoSession
	case recordedErr != nil:
		reason = CleanupNoRecordedIdentity
	}
	if err := removeAccountFiles(stateDir); err != nil {
		return false, reason, err
	}
	return true, reason, nil
}

// Record writes the current session identity after a successful sign-in.
func Record(stateDir, sessionFile string) error {
	id, err := sessionIdentity(sessionFile)
	if err != nil {
		return err
	}
	path := filepath.Join(stateDir, accountIDFile)
	if err := os.WriteFile(path, []byte(id+"\n"), 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func sessionIdentity(path string) (string, error) {
	loader := session.Loader{Storage: &session.FileStorage{Path: path}}
	data, err := loader.Load(context.Background())
	if err != nil {
		return "", fmt.Errorf("read session identity from %s: %w", path, err)
	}
	if len(data.AuthKeyID) == 0 {
		return "", fmt.Errorf("read session identity from %s: auth key ID is empty", path)
	}
	return hex.EncodeToString(data.AuthKeyID), nil
}

func recordedIdentity(stateDir string) (string, error) {
	path := filepath.Join(stateDir, accountIDFile)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(decoded) == 0 {
		return "", fmt.Errorf("read %s: invalid account identity", path)
	}
	return hex.EncodeToString(decoded), nil
}

func removeAccountFiles(stateDir string) error {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return fmt.Errorf("list %s: %w", stateDir, err)
	}

	paths := make([]string, 0, len(entries)+2)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "state.db") {
			paths = append(paths, filepath.Join(stateDir, entry.Name()))
		}
	}
	for _, cache := range []struct {
		name    string
		resolve func(string) (string, error)
	}{
		{name: "media cache", resolve: MediaCacheDir},
		{name: "avatar cache", resolve: AvatarCacheDir},
	} {
		path, cacheErr := cache.resolve(stateDir)
		if cacheErr == nil {
			paths = append(paths, path)
			continue
		}
		if errors.Is(cacheErr, errNoUserCache) {
			continue
		}
		return fmt.Errorf("locate %s: %w", cache.name, cacheErr)
	}

	var removeErrors []error
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			removeErrors = append(removeErrors, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	if err := errors.Join(removeErrors...); err != nil {
		return fmt.Errorf("clear previous account state: %w", err)
	}
	return nil
}
