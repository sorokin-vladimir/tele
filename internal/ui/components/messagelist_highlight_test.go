package components_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

func TestMessageList_HighlightMessage_SetsIDAndStep(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.HighlightMessage(7)
	assert.Equal(t, 7, ml.HighlightedMsgID())
	assert.Equal(t, components.HighlightInitialStep, ml.HighlightStep())
}

func TestMessageList_StepHighlight_CountsDownAndClears(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.HighlightMessage(7)
	// Step decrements from HighlightInitialStep; returns true until it hits 0.
	for i := components.HighlightInitialStep; i > 1; i-- {
		assert.True(t, ml.StepHighlight(), "still active at step %d", i)
	}
	// Final decrement reaches 0: returns false and clears the id.
	assert.False(t, ml.StepHighlight())
	assert.Equal(t, 0, ml.HighlightStep())
	assert.Equal(t, 0, ml.HighlightedMsgID())
}

func TestMessageList_StepHighlight_NoOpWhenInactive(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	assert.False(t, ml.StepHighlight())
	assert.Equal(t, 0, ml.HighlightStep())
}
