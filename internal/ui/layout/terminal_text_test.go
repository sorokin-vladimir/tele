package layout

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestTerminalSafeText(t *testing.T) {
	const joined = "\U0001f441\u200d\U0001f5e8"
	for _, tc := range []struct{ name, input, want string }{
		{"plain", "plain text", "plain text"},
		{"joined", "Team " + joined + "!", "Team ?!"},
		{"repeated", joined + " | " + joined, "? | ?"},
		{"qualified", "\U0001f441\ufe0f\u200d\U0001f5e8\ufe0f", "\U0001f441\ufe0f\u200d\U0001f5e8\ufe0f"},
		{"wide", "\U0001f469\u200d\U0001f4bb", "\U0001f469\u200d\U0001f4bb"},
		{"keycap", "2\ufe0f\u20e3", "2\ufe0f\u20e3"},
		{"combining", "Cafe\u0301", "Cafe\u0301"},
		{"conjunct", "\u0915\u094d\u200d\u0937", "\u0915\u094d\u200d\u0937"},
		{"text symbols", "\u2665 + \u263a", "\u2665 + \u263a"},
		{"styles", "\x1b[31m" + joined + "\x1b[0m", "\x1b[31m?\x1b[0m"},
		{"link", "\x1b]8;;https://example.org/" + joined + "\x1b\\" + joined + "\x1b]8;;\x1b\\", "\x1b]8;;https://example.org/" + joined + "\x1b\\?\x1b]8;;\x1b\\"},
		{"opaque marker", "\x1bPq" + joined + "\x1b\\", "\x1bPq" + joined + "\x1b\\"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := TerminalSafeText(tc.input)
			require.Equal(t, tc.want, got)
			require.Equal(t, ansi.StringWidth(tc.input), ansi.StringWidth(got), "the fallback must preserve layout width")
			require.Equal(t, got, TerminalSafeText(got), "idle frames must remain unchanged")
		})
	}
}
