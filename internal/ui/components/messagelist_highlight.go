package components

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// HighlightMessage starts a fade highlight on the bubble with the given id.
func (ml *MessageList) HighlightMessage(id int) {
	ml.highlightedMsgID = id
	ml.highlightStep = HighlightInitialStep
}

// StepHighlight advances the fade by one step. It returns true while the
// highlight is still visible; on reaching 0 it clears the highlight and
// returns false. It is a no-op (returns false) when no highlight is active.
func (ml *MessageList) StepHighlight() bool {
	if ml.highlightStep <= 0 {
		return false
	}
	ml.highlightStep--
	if ml.highlightStep <= 0 {
		ml.highlightedMsgID = 0
		return false
	}
	return true
}

// HighlightedMsgID returns the currently highlighted message id (0 when none).
func (ml *MessageList) HighlightedMsgID() int { return ml.highlightedMsgID }

// HighlightStep returns the current fade step (0 when no highlight is active).
func (ml *MessageList) HighlightStep() int { return ml.highlightStep }

// bubbleBorderFg returns the border color for a message bubble: the normal
// outgoing/incoming tone, replaced by a faded accent while this message is the
// active highlight target.
func (ml *MessageList) bubbleBorderFg(msg store.Message) color.Color {
	var borderFg color.Color
	if msg.IsOut {
		borderFg = lipgloss.Color("25")
	} else {
		borderFg = lipgloss.Color("238")
	}
	if ml.highlightStep > 0 && msg.ID == ml.highlightedMsgID {
		borderFg = FadeAccentColor(HighlightAccentFor(ml.hasDarkBackground), borderFg, ml.highlightStep, HighlightFadeSteps)
	}
	return borderFg
}
