package components_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/require"
)

func TestRenderEntitiesConjunctPreservesOffsetsAndTargets(t *testing.T) {
	const word = "\u0d2e\u0d4d\u0d2a"
	const shown = "\u0d2e\u0d4d\u200c\u0d2a"
	const target = "https://example.com/" + word
	msg := domain.Message{Text: "\U0001f642 " + word + " link", Entities: []domain.MessageEntity{
		{Type: "bold", Offset: 3, Length: 3},
		{Type: "text_url", Offset: 7, Length: 4, URL: target},
	}}
	before := append([]domain.MessageEntity(nil), msg.Entities...)
	out := components.RenderEntities(msg.Text, msg.Entities)
	require.Equal(t, "\U0001f642 "+shown+" link", ansi.Strip(out))
	require.Contains(t, out, "\x1b]8;id=2;"+target+"\x1b\\")
	_, linked, found := strings.Cut(out, "\x1b]8;id=2;"+target+"\x1b\\")
	require.True(t, found)
	label, _, _ := strings.Cut(linked, "\x1b]8;;")
	require.Equal(t, "link", ansi.Strip(label))
	require.Equal(t, "\U0001f642 "+word+" link", msg.Text)
	require.Equal(t, before, msg.Entities)
	for _, entities := range [][]domain.MessageEntity{nil, {{Type: "bold", Offset: 100, Length: 1}}} {
		require.Equal(t, shown, ansi.Strip(components.RenderEntities(word, entities)))
	}
}

func TestRenderBoxMeasuresConjunctTitlesBeforePadding(t *testing.T) {
	const word = "\u0d2e\u0d4d\u0d2a"
	for width := 10; width < 30; width++ {
		out := components.RenderBox("", word, word, "", "", lipgloss.RoundedBorder(), nil, width, 3)
		for _, line := range strings.Split(out, "\n") {
			require.Equal(t, width, ansi.StringWidth(line))
		}
		require.Contains(t, out, "\u0d2e\u0d4d\u200c\u0d2a")
	}
}
