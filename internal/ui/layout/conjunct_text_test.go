package layout

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestConjunctTitleUsesTerminalCellWidth(t *testing.T) {
	const title = "\u0d15\u0d2e\u0d4d\u0d2a\u0d28\u0d3f"
	got := TruncateText(title, 20)
	require.Equal(t, 4, ansi.StringWidth(got), "Foot advances four cells for this Malayalam word")
	for width := 8; width <= 40; width++ {
		got := TruncateText(strings.Repeat(title+" ", 8), width-6) + " [99+]"
		require.LessOrEqual(t, ansi.StringWidth(got), width)
		require.Equal(t, got, PresentationText(got), "truncation must measure the presentation form")
	}
}

func TestPresentationTextNarrowConjuncts(t *testing.T) {
	cases := []struct{ name, input, want string }{
		{"Malayalam", "\u0d2e\u0d4d\u0d2a", "\u0d2e\u0d4d\u200c\u0d2a"},
		{"Devanagari", "\u0915\u094d\u0937", "\u0915\u094d\u200c\u0937"},
		{"Devanagari joiner", "\u0915\u094d\u200d\u0937", "\u0915\u094d\u200d\u200c\u0937"},
		{"Bengali", "\u0995\u09cd\u09b7", "\u0995\u09cd\u200c\u09b7"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := PresentationText(tt.input)
			require.Equal(t, tt.want, got)
			require.Equal(t, 2, ansi.StringWidth(got))
			require.Equal(t, got, PresentationText(got))
			require.Equal(t, tt.input, strings.ReplaceAll(got, "\u200c", ""))
		})
	}
}

func TestPresentationTextPreservesOtherText(t *testing.T) {
	for _, input := range []string{
		"", "ordinary ASCII", "e\u0301", "\u4e2d\u6587", "\u0e01\u0e33",
		"\u0d15\u0d3f", "\u0d2e\u0d4d\u200c\u0d2a", "2\ufe0f\u20e3",
		"\U0001f441\ufe0f\u200d\U0001f5e8\ufe0f",
	} {
		require.Equal(t, input, PresentationText(input))
	}
}

func TestPresentationTextPreservesTerminalPayloads(t *testing.T) {
	const conjunct = "\u0d2e\u0d4d\u0d2a"
	const shown = "\u0d2e\u0d4d\u200c\u0d2a"
	const open = "\x1b]8;id=1;https://example.com/" + conjunct + "\x1b\\"
	const close = "\x1b]8;;\x1b\\"
	const marker = "\x1bPtele;" + conjunct + "\x1b\\"
	input := marker + open + "\x1b[1m" + conjunct + "\x1b[0m" + close
	require.Equal(t, marker+open+"\x1b[1m"+shown+"\x1b[0m"+close, PresentationText(input))
}
