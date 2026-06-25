package components

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestBubbleBorderFg_NormalWhenNoHighlight(t *testing.T) {
	ml := NewMessageList(20, 80)
	in := ml.bubbleBorderFg(store.Message{ID: 1, IsOut: false})
	assert.Equal(t, lipgloss.Color("238"), in)
	out := ml.bubbleBorderFg(store.Message{ID: 2, IsOut: true})
	assert.Equal(t, lipgloss.Color("25"), out)
}

func TestBubbleBorderFg_AccentWhenHighlighted_Dark(t *testing.T) {
	ml := NewMessageList(20, 80)
	ml.SetDarkBackground(true)
	ml.HighlightMessage(1)
	got := ml.bubbleBorderFg(store.Message{ID: 1, IsOut: false})
	want := FadeAccentColor(HighlightAccent, lipgloss.Color("238"), HighlightFadeSteps, HighlightFadeSteps)
	assert.Equal(t, want, got)
}

func TestBubbleBorderFg_AccentWhenHighlighted_Light(t *testing.T) {
	ml := NewMessageList(20, 80)
	ml.SetDarkBackground(false)
	ml.HighlightMessage(1)
	got := ml.bubbleBorderFg(store.Message{ID: 1, IsOut: false})
	want := FadeAccentColor(HighlightAccentLight, lipgloss.Color("238"), HighlightFadeSteps, HighlightFadeSteps)
	assert.Equal(t, want, got)
	// The light accent must differ from the dark one.
	assert.NotEqual(t, HighlightAccent, HighlightAccentLight)
}

func TestBubbleBorderFg_OtherMessagesUnaffected(t *testing.T) {
	ml := NewMessageList(20, 80)
	ml.HighlightMessage(1)
	got := ml.bubbleBorderFg(store.Message{ID: 99, IsOut: false})
	assert.Equal(t, lipgloss.Color("238"), got)
}
