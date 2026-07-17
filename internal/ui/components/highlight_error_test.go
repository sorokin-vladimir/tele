package components

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestErrorAccentFor_DarkLightDiffer(t *testing.T) {
	assert.Equal(t, ErrorAccent, ErrorAccentFor(true))
	assert.Equal(t, ErrorAccentLight, ErrorAccentFor(false))
	assert.NotEqual(t, ErrorAccentFor(true), ErrorAccentFor(false))
}

func TestErrorAccent_IsTruecolorRed(t *testing.T) {
	// Anchored to the toast error red (xterm 203).
	assert.Equal(t, lipgloss.Color("#ff5f5f"), ErrorAccent)
}

func TestHighlightKind_InfoIsZero(t *testing.T) {
	assert.Equal(t, HighlightKind(0), HighlightInfo)
	assert.NotEqual(t, HighlightInfo, HighlightError)
}
