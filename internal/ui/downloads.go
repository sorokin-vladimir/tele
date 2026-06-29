package ui

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// downloadsDir resolves the OS Downloads directory. A package var so tests can
// stub it.
var downloadsDir = resolveDownloadsDir

// resolveDownloadsDir returns the user's Downloads directory, falling back to
// the home dir and finally the OS temp dir so it always yields a usable path.
func resolveDownloadsDir() string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS != "darwin" {
		if d := os.Getenv("XDG_DOWNLOAD_DIR"); d != "" {
			return d
		}
		if d := xdgDownloadFromUserDirs(home); d != "" {
			return d
		}
	}
	if home != "" {
		dl := filepath.Join(home, "Downloads")
		if fi, err := os.Stat(dl); err == nil && fi.IsDir() {
			return dl
		}
		return home
	}
	return os.TempDir()
}

// xdgDownloadFromUserDirs parses ~/.config/user-dirs.dirs for an
// XDG_DOWNLOAD_DIR="$HOME/..." entry and expands it. Returns "" if absent.
func xdgDownloadFromUserDirs(home string) string {
	if home == "" {
		return ""
	}
	f, err := os.Open(filepath.Join(home, ".config", "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "XDG_DOWNLOAD_DIR=") {
			continue
		}
		val := strings.Trim(strings.TrimPrefix(line, "XDG_DOWNLOAD_DIR="), `"`)
		val = strings.Replace(val, "$HOME", home, 1)
		if val != "" {
			return val
		}
	}
	return ""
}

// createUniqueDownloadFile creates a new file in dir under fileName's base name,
// resolving collisions as "name (1).ext", "name (2).ext", ... It uses O_EXCL so
// the name is claimed atomically (no overwrite, no TOCTOU race). The caller owns
// the returned file and must Close it.
func createUniqueDownloadFile(dir, fileName string) (*os.File, error) {
	base := filepath.Base(filepath.Clean(fileName))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "file"
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 0; ; i++ {
		name := base
		if i > 0 {
			name = stem + " (" + itoa(i) + ")" + ext
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
		if err == nil {
			return f, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
	}
}

// itoa avoids importing strconv for a single positive int.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// itoa64 formats a non-negative int64 (e.g. a media ID) without strconv.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
