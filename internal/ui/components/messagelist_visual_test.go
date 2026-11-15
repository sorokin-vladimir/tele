package components_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVisual_TextCoversAnchorToCursorOldestFirst(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisual())
	ml.CursorUp()
	ml.CursorUp()
	assert.True(t, ml.InVisual())
	assert.Equal(t, "msg 3\n\nmsg 4\n\nmsg 5", ml.VisualText())
}

func TestVisual_TextIsTheSameWhenTheCursorPassesBelowTheAnchor(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	ml.CursorUp()
	ml.CursorUp()
	require.True(t, ml.StartVisual())
	ml.CursorDown()
	ml.CursorDown()
	assert.Equal(t, "msg 3\n\nmsg 4\n\nmsg 5", ml.VisualText())
}

func TestVisual_SingleMessageRangeIsTheFocusedMessage(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisual())
	assert.Equal(t, "msg 3", ml.VisualText())
}

func TestVisual_TextSkipsMessagesWithoutText(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages([]domain.Message{
		{ID: 1, ChatID: 1, Text: "first"},
		{ID: 2, ChatID: 1, Photo: &domain.PhotoRef{ID: 9}},
		{ID: 3, ChatID: 1, Text: "third"},
	})
	require.True(t, ml.StartVisual())
	ml.CursorUp()
	ml.CursorUp()
	assert.Equal(t, "first\n\nthird", ml.VisualText())
}

func TestVisual_ClearEndsTheRange(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisual())
	ml.ClearVisual()
	assert.False(t, ml.InVisual())
	assert.Equal(t, "", ml.VisualText())
}

func TestVisual_SetMessagesEndsTheRange(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisual())
	ml.SetMessages(makeMessages(3))
	assert.False(t, ml.InVisual())
}

func TestVisual_CannotStartOnAnEmptyList(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	assert.False(t, ml.StartVisual())
	assert.False(t, ml.InVisual())
}

func TestVisual_ViewDrawsTheIndicatorBesideEverySelectedBubble(t *testing.T) {
	ml := components.NewMessageList(30, 60)
	ml.SetShowIndicator(true)
	ml.SetMessages(makeMessages(3))
	single := countIndicators(ml.View())
	require.True(t, ml.StartVisual())
	ml.CursorUp()
	ml.CursorUp()
	assert.Greater(t, countIndicators(ml.View()), single)
}

func countIndicators(view string) int { return strings.Count(view, "┃") }
