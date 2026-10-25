package media

import "strings"

// Mode is the selected image rendering strategy.
type Mode int

// String names the mode the way photos.mode spells it, so a log line and the
// config file speak the same language.
func (m Mode) String() string {
	switch m {
	case ModeKitty:
		return "kitty"
	case ModeSixel:
		return "sixel"
	default:
		return "blocks"
	}
}

const (
	// ModeBlocks renders photos as ANSI half-block art (universal fallback).
	ModeBlocks Mode = iota
	// ModeKitty renders photos via the Kitty graphics protocol.
	ModeKitty
	// ModeSixel renders inline photo previews and the photo modal via DEC
	// sixel, repainting graphics after text updates and retaining block art
	// underneath as a fallback.
	ModeSixel
)

// DetectMode picks a rendering mode. override is the config value
// (photos.mode): "kitty", "sixel" or "blocks" force a mode, "auto" (or empty)
// runs heuristics. getenv is usually os.Getenv; it is injected for testing.
//
// Kitty is enabled only for terminals with confirmed Unicode-placeholder
// support (kitty, Ghostty, iTerm2 from 3.7.0) and never inside a tmux/screen
// session (no passthrough this pass). Sixel is enabled for foot, which draws
// sixel graphics and not Kitty ones, and for the other terminals that name
// themselves in TERM.
func DetectMode(override string, getenv func(string) string) Mode {
	switch strings.ToLower(override) {
	case "kitty":
		return ModeKitty
	case "sixel":
		return ModeSixel
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
	if isSixelTerminal(term, getenv) {
		return ModeSixel
	}
	return ModeBlocks
}

// isSixelTerminal reports whether the environment names a terminal that draws
// sixel graphics. foot is the reason this exists: it implements sixel and no
// Kitty protocol at all. mlterm and WezTerm are the others that both speak
// sixel and are recognisable from the environment; WezTerm has the Kitty
// protocol but not the Unicode placeholders, so sixel is the better answer
// there and it is deliberately not listed with the Kitty terminals above.
func isSixelTerminal(term string, getenv func(string) string) bool {
	term = strings.ToLower(term)
	if strings.HasPrefix(term, "foot") || strings.HasPrefix(term, "mlterm") {
		return true
	}
	return strings.EqualFold(getenv("TERM_PROGRAM"), "WezTerm")
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
// as its release, and missing fields read as zero. An unparseable or empty
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
