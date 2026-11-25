package media

import "strings"

// Mode is the selected image rendering strategy.
type Mode int

const (
	// ModeBlocks renders photos as ANSI half-block art (universal fallback).
	ModeBlocks Mode = iota
	// ModeKitty renders photos via the Kitty graphics protocol.
	ModeKitty
)

// DetectMode picks a rendering mode. override is the config value
// (photos.mode): "kitty" or "blocks" force a mode, "auto" (or empty) runs
// heuristics. getenv is usually os.Getenv; it is injected for testing.
//
// Kitty is enabled only for terminals with confirmed Unicode-placeholder
// support (kitty, Ghostty, iTerm2 from 3.7.0) and never inside a tmux/screen
// session (no passthrough this pass).
func DetectMode(override string, getenv func(string) string) Mode {
	switch strings.ToLower(override) {
	case "kitty":
		return ModeKitty
	case "blocks":
		return ModeBlocks
	}
	if getenv("TMUX") != "" || getenv("STY") != "" {
		return ModeBlocks
	}
	term := getenv("TERM")
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") {
		return ModeKitty
	}
	if getenv("KITTY_WINDOW_ID") != "" {
		return ModeKitty
	}
	if strings.EqualFold(getenv("TERM_PROGRAM"), "ghostty") {
		return ModeKitty
	}
	if isITerm2(getenv) && atLeastVersion(iterm2Version(getenv), 3, 7, 0) {
		return ModeKitty
	}
	return ModeBlocks
}

// isITerm2 reports whether we are talking to iTerm2. LC_TERMINAL is the one
// that survives an ssh hop, since iTerm2 forwards it on purpose.
func isITerm2(getenv func(string) string) bool {
	return strings.EqualFold(getenv("TERM_PROGRAM"), "iTerm.app") ||
		strings.EqualFold(getenv("LC_TERMINAL"), "iTerm2")
}

// iterm2Version returns the reported iTerm2 version, empty when it says
// nothing.
func iterm2Version(getenv func(string) string) string {
	if v := getenv("TERM_PROGRAM_VERSION"); v != "" {
		return v
	}
	return getenv("LC_TERMINAL_VERSION")
}

// atLeastVersion reports whether the dotted version names a release at or
// after want. Trailing junk ends a field, so a "3.7.0beta10" prerelease counts
// as its release, and missing fields read as zero. An unparsable or empty
// version is not enough.
//
// iTerm2 is version-gated (3.7.0) rather than named outright: it has drawn
// Kitty images since 2024 but only recent builds place them through Unicode
// placeholders correctly, and a terminal that ignores the placeholder cells
// shows blank space where block art would have shown the photo.
func atLeastVersion(version string, want ...int) bool {
	if version == "" {
		return false
	}
	fields := strings.Split(version, ".")
	for i, w := range want {
		var got int
		if i < len(fields) {
			got = leadingInt(fields[i])
		}
		if got != w {
			return got > w
		}
	}
	return true
}

// leadingInt reads the digits that start s, so "0beta10" is 0.
func leadingInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
