package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVisual_ViewDrawsTheSelectedBubblesInTheIndicatorColour(t *testing.T) {
	theme.Apply(true)
	ml := NewMessageList(30, 60)
	ml.SetMessages([]domain.Message{{ID: 1, Text: "one"}, {ID: 2, Text: "two"}, {ID: 3, Text: "three"}})
	tinted := lipgloss.NewStyle().Foreground(theme.T().Indicator).Render("╭")
	tintedTop := strings.TrimSuffix(tinted, "\x1b[m")
	require.NotEqual(t, "╭", tinted, "the test renders colour")

	assert.NotContains(t, ml.View(), tintedTop)
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	view := ml.View()
	assert.Equal(t, 2, strings.Count(view, tintedTop))
	assert.Nil(t, ml.selectedWhileDrawing, "the selection is only read while drawing")
}

func TestVisual_BorderColourMarksOnlyTheSelectedMessages(t *testing.T) {
	theme.Apply(true)
	ml := NewMessageList(20, 80)
	ml.SetMessages([]domain.Message{{ID: 1, Text: "one"}, {ID: 2, Text: "two"}})
	require.True(t, ml.StartVisualMark())
	ml.selectedWhileDrawing = ml.selectedPartIDs()
	assert.Equal(t, theme.T().Indicator, ml.bubbleBorderFg(domain.Message{ID: 2}))
	assert.Equal(t, theme.T().BorderBubbleIn, ml.bubbleBorderFg(domain.Message{ID: 1}))
}
