package media_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

func TestDetectMode(t *testing.T) {
	cases := []struct {
		name     string
		override string
		env      map[string]string
		want     media.Mode
	}{
		{"kitty term", "auto", map[string]string{"TERM": "xterm-kitty"}, media.ModeKitty},
		{"kitty window id", "auto", map[string]string{"KITTY_WINDOW_ID": "1"}, media.ModeKitty},
		{"ghostty term", "auto", map[string]string{"TERM": "xterm-ghostty"}, media.ModeKitty},
		{"ghostty program", "auto", map[string]string{"TERM_PROGRAM": "ghostty"}, media.ModeKitty},
		{"plain xterm", "auto", map[string]string{"TERM": "xterm-256color"}, media.ModeBlocks},
		{"foot term uses sixel", "auto", map[string]string{"TERM": "foot"}, media.ModeSixel},
		{"foot-extra uses sixel", "auto", map[string]string{"TERM": "foot-extra"}, media.ModeSixel},
		{"mlterm uses sixel", "auto", map[string]string{"TERM": "mlterm"}, media.ModeSixel},
		{
			"wezterm has no placeholders, so sixel",
			"auto",
			map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "WezTerm"},
			media.ModeSixel,
		},
		{"foot under tmux stays on blocks", "auto", map[string]string{"TERM": "foot", "TMUX": "/tmp/x"}, media.ModeBlocks},
		{"override sixel on plain", "sixel", map[string]string{"TERM": "xterm-256color"}, media.ModeSixel},
		{"override blocks on foot", "blocks", map[string]string{"TERM": "foot"}, media.ModeBlocks},
		{"kitty under tmux", "auto", map[string]string{"TERM": "xterm-kitty", "TMUX": "/tmp/x"}, media.ModeBlocks},
		{"kitty under screen", "auto", map[string]string{"TERM": "xterm-kitty", "STY": "1.x"}, media.ModeBlocks},
		{"empty override falls through", "", map[string]string{"TERM": "xterm-kitty"}, media.ModeKitty},
		{"override kitty on plain", "kitty", map[string]string{"TERM": "xterm-256color"}, media.ModeKitty},
		{"override blocks on kitty", "blocks", map[string]string{"TERM": "xterm-kitty"}, media.ModeBlocks},
		{"override auto falls through", "auto", map[string]string{"TERM": "xterm-kitty"}, media.ModeKitty},
		{
			"iterm2 draws placeholders from 3.7",
			"auto",
			map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.7.0"},
			media.ModeKitty,
		},
		{
			"iterm2 beta counts as its release",
			"auto",
			map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.7.0beta10"},
			media.ModeKitty,
		},
		{
			"iterm2 before 3.7 stays on blocks",
			"auto",
			map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.6.9"},
			media.ModeBlocks,
		},
		{
			"iterm2 without a version stays on blocks",
			"auto",
			map[string]string{"TERM": "xterm-256color", "TERM_PROGRAM": "iTerm.app"},
			media.ModeBlocks,
		},
		{
			"iterm2 over ssh is named by LC_TERMINAL",
			"auto",
			map[string]string{"TERM": "xterm-256color", "LC_TERMINAL": "iTerm2", "LC_TERMINAL_VERSION": "3.7.1"},
			media.ModeKitty,
		},
		{
			"iterm2 under tmux stays on blocks",
			"auto",
			map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.7.0", "TMUX": "/tmp/x"},
			media.ModeBlocks,
		},
		{
			"a later iterm2 major keeps drawing",
			"auto",
			map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "4.0.0"},
			media.ModeKitty,
		},
		{
			"a short iterm2 version is padded",
			"auto",
			map[string]string{"TERM_PROGRAM": "iTerm.app", "TERM_PROGRAM_VERSION": "3.8"},
			media.ModeKitty,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := media.DetectMode(tc.override, func(k string) string { return tc.env[k] })
			require.Equal(t, tc.want, got)
		})
	}
}
