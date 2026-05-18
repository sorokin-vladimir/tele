package components_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

func roundedBorder() lipgloss.Border { return lipgloss.RoundedBorder() }

func TestRenderBox_NoTitleNoHint(t *testing.T) {
	out := components.RenderBox("hello", "", "", roundedBorder(), 9, 3)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 3, len(lines))
	assert.NotContains(t, lines[0], "hello")
	assert.NotContains(t, lines[2], "hello")
	assert.Contains(t, lines[1], "hello")
}

func TestRenderBox_WithTitle_EmbeddedInTopBorder(t *testing.T) {
	out := components.RenderBox("content", "My Title", "", roundedBorder(), 16, 3)
	lines := strings.Split(out, "\n")
	assert.Contains(t, lines[0], "My Title")
	assert.NotContains(t, lines[2], "My Title")
}

func TestRenderBox_WithHint_EmbeddedInBottomBorder(t *testing.T) {
	out := components.RenderBox("content", "", "esc | enter", roundedBorder(), 18, 3)
	lines := strings.Split(out, "\n")
	assert.NotContains(t, lines[0], "esc")
	assert.Contains(t, lines[2], "esc | enter")
}

func TestRenderBox_ContentPaddedToInnerWidth(t *testing.T) {
	// innerW = 8-2 = 6; "hi" is 2 chars → line padded to innerW in the box
	out := components.RenderBox("hi", "", "", roundedBorder(), 8, 3)
	lines := strings.Split(out, "\n")
	// content line: border-left + 6-char content + border-right = 8 chars visual width
	assert.Equal(t, 8, lipgloss.Width(lines[1]))
}

func TestRenderBox_ContentTrimmedToInnerHeight(t *testing.T) {
	// innerH = 5-2 = 3, but content has 5 lines → trimmed to 3
	content := "a\nb\nc\nd\ne"
	out := components.RenderBox(content, "", "", roundedBorder(), 5, 5)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 5, len(lines))
	assert.NotContains(t, out, "\nd\n")
	assert.NotContains(t, out, "\ne")
}

func TestRenderBox_ContentPaddedToInnerHeight(t *testing.T) {
	// innerH = 5-2 = 3, but content has 1 line → 5 total lines (border+3 content+border)
	out := components.RenderBox("hi", "", "", roundedBorder(), 8, 5)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 5, len(lines))
}

func TestRenderBox_HintTooLong_FallsBackToPlainBorder(t *testing.T) {
	// hint longer than inner width → bottom border has no hint text
	out := components.RenderBox("x", "", "this hint is way too long for the box", roundedBorder(), 5, 3)
	lines := strings.Split(out, "\n")
	assert.NotContains(t, lines[2], "this hint")
}
