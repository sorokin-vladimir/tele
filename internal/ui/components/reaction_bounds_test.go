package components

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestReactionsKeepWholeCountsAndTelegramTies(t *testing.T) {
	largest := int(^uint(0) >> 1)
	reactions := []domain.Reaction{
		{Emoji: "👍", Count: 12}, {Emoji: "2\ufe0f\u20e3", Count: 20},
		{Emoji: "👩‍💻", Count: 20, IsChosen: true}, {Emoji: "中", Count: largest},
	}
	original := slices.Clone(reactions)
	want := []string{"中 " + strconv.Itoa(largest), "2\ufe0f\u20e3 20", "👩‍💻 20", "👍 12"}
	require.Equal(t, strings.Join(want, " · "), strings.TrimSpace(ansi.Strip(buildReactStr(reactions, 200))))
	for width := 0; width <= 100; width++ {
		rendered := buildReactStr(reactions, width)
		require.LessOrEqual(t, lipgloss.Width(rendered), width)
		plain := strings.TrimSpace(ansi.Strip(rendered))
		require.NotContains(t, plain, "\n")
		if plain == "" {
			require.Zero(t, width)
			continue
		}
		parts := strings.Split(plain, " · ")
		if width < lipgloss.Width(buildReactStr(reactions, 200)) {
			require.Equal(t, "…", parts[len(parts)-1])
			parts = parts[:len(parts)-1]
		}
		require.Equal(t, want[:len(parts)], parts, "width=%d: no partial emoji/count, highest first", width)
	}
	require.Equal(t, original, reactions, "presentation must not reorder Telegram data")
}

func TestReactionFirstEntryCannotFit(t *testing.T) {
	reactions := []domain.Reaction{{Emoji: "👍", Count: 12}}
	for width := 1; width < lipgloss.Width(" 👍 12 "); width++ {
		require.Equal(t, "…", strings.TrimSpace(ansi.Strip(buildReactStr(reactions, width))))
	}
	require.Empty(t, buildReactStr(nil, 30))
	require.Empty(t, buildReactStr(reactions, 0))
}
