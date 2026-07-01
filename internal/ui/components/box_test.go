package components_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roundedBorder() lipgloss.Border { return lipgloss.RoundedBorder() }

func TestRenderBox_NoTitleNoHint(t *testing.T) {
	out := components.RenderBox("hello", "", "", "", "", roundedBorder(), nil, 9, 3)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 3, len(lines))
	assert.NotContains(t, lines[0], "hello")
	assert.NotContains(t, lines[2], "hello")
	assert.Contains(t, lines[1], "hello")
}

func TestRenderBox_WithTitle_EmbeddedInTopBorder(t *testing.T) {
	out := components.RenderBox("content", "My Title", "", "", "", roundedBorder(), nil, 16, 3)
	lines := strings.Split(out, "\n")
	assert.Contains(t, lines[0], "My Title")
	assert.NotContains(t, lines[2], "My Title")
}

func TestRenderBox_WithHint_EmbeddedInBottomBorder(t *testing.T) {
	out := components.RenderBox("content", "", "", "esc | enter", "", roundedBorder(), nil, 18, 3)
	lines := strings.Split(out, "\n")
	assert.NotContains(t, lines[0], "esc")
	assert.Contains(t, lines[2], "esc | enter")
}

func TestRenderBox_ContentPaddedToInnerWidth(t *testing.T) {
	// innerW = 8-2 = 6; "hi" is 2 chars → line padded to innerW in the box
	out := components.RenderBox("hi", "", "", "", "", roundedBorder(), nil, 8, 3)
	lines := strings.Split(out, "\n")
	// content line: border-left + 6-char content + border-right = 8 chars visual width
	assert.Equal(t, 8, lipgloss.Width(lines[1]))
}

func TestRenderBox_ContentTrimmedToInnerHeight(t *testing.T) {
	// innerH = 5-2 = 3, but content has 5 lines → trimmed to 3
	content := "a\nb\nc\nd\ne"
	out := components.RenderBox(content, "", "", "", "", roundedBorder(), nil, 5, 5)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 5, len(lines))
	assert.NotContains(t, out, "\nd\n")
	assert.NotContains(t, out, "\ne")
}

func TestRenderBox_ContentPaddedToInnerHeight(t *testing.T) {
	// innerH = 5-2 = 3, but content has 1 line → 5 total lines (border+3 content+border)
	out := components.RenderBox("hi", "", "", "", "", roundedBorder(), nil, 8, 5)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 5, len(lines))
}

func TestRenderBox_HintTooLong_FallsBackToPlainBorder(t *testing.T) {
	// hint longer than inner width → bottom border has no hint text
	out := components.RenderBox("x", "", "", "this hint is way too long for the box", "", roundedBorder(), nil, 5, 3)
	lines := strings.Split(out, "\n")
	assert.NotContains(t, lines[2], "this hint")
}

func TestRenderBox_WithSuffix_EmbeddedInTopBorder(t *testing.T) {
	out := components.RenderBox("content", "Name", "●", "", "", roundedBorder(), nil, 24, 3)
	lines := strings.Split(out, "\n")
	assert.Contains(t, lines[0], "Name")
	assert.Contains(t, lines[0], "●")
	assert.Equal(t, 24, lipgloss.Width(lines[0]))
}

func TestRenderBox_WithSuffix_TooNarrow_SuffixSkipped(t *testing.T) {
	out := components.RenderBox("x", "Name", "●", "", "", roundedBorder(), nil, 10, 3)
	lines := strings.Split(out, "\n")
	assert.Equal(t, 10, lipgloss.Width(lines[0]))
}

func TestRenderBox_WithSuffix_SpacesAroundDot(t *testing.T) {
	out := components.RenderBox("content", "Name", "●", "", "", roundedBorder(), nil, 24, 3)
	lines := strings.Split(out, "\n")
	dotIdx := strings.Index(lines[0], "●")
	require.Greater(t, dotIdx, 0)
	assert.Equal(t, " ", string(lines[0][dotIdx-1]))
}

func TestRenderBox_Scrollbar_DrawsThumb(t *testing.T) {
	// 6-tall box => innerH = 4. Content overflows: 100 rows, 4 visible, at top.
	sb := &components.Scrollbar{
		Info:     components.ScrollInfo{Total: 100, Visible: 4, Offset: 0},
		TrackTop: 0,
		TrackLen: 4,
	}
	out := components.RenderBox("a\nb\nc\nd", "", "", "", "", lipgloss.RoundedBorder(), nil, 6, 6, sb)
	lines := strings.Split(out, "\n")
	// lines[0] top border, lines[1..4] content, lines[5] bottom border.
	// At top, thumb is row 0 of the track (content line 1), size 1.
	assert.True(t, strings.HasSuffix(lines[1], "█"), "thumb on first content row")
	assert.True(t, strings.HasSuffix(lines[2], "│"), "track elsewhere")
	assert.True(t, strings.HasSuffix(lines[4], "│"), "track elsewhere")
}

func TestRenderBox_Scrollbar_NoOverflowPlainBorder(t *testing.T) {
	sb := &components.Scrollbar{
		Info:     components.ScrollInfo{Total: 2, Visible: 4, Offset: 0},
		TrackTop: 0,
		TrackLen: 4,
	}
	out := components.RenderBox("a\nb\nc\nd", "", "", "", "", lipgloss.RoundedBorder(), nil, 6, 6, sb)
	assert.NotContains(t, out, "█", "content fits, no thumb")
}

func TestRenderBox_Scrollbar_SubRangeTrack(t *testing.T) {
	// Track covers only the first 2 inner rows (e.g. message-list region);
	// the remaining rows must keep a plain border even on overflow.
	sb := &components.Scrollbar{
		Info:     components.ScrollInfo{Total: 100, Visible: 2, Offset: 100},
		TrackTop: 0,
		TrackLen: 2,
	}
	out := components.RenderBox("a\nb\nc\nd", "", "", "", "", lipgloss.RoundedBorder(), nil, 6, 6, sb)
	lines := strings.Split(out, "\n")
	assert.True(t, strings.HasSuffix(lines[3], "│"), "row outside track is plain")
	assert.True(t, strings.HasSuffix(lines[4], "│"), "row outside track is plain")
}

func TestRenderBox_NilScrollbar_Unchanged(t *testing.T) {
	out := components.RenderBox("a\nb", "", "", "", "", lipgloss.RoundedBorder(), nil, 6, 6, nil)
	assert.NotContains(t, out, "█")
}

func TestRenderBoxBottomSuffix(t *testing.T) {
	out := components.RenderBox("x", "", "", "", "GO", roundedBorder(), nil, 12, 3)
	lines := strings.Split(out, "\n")
	last := lines[len(lines)-1]
	if !strings.Contains(last, "GO") {
		t.Fatalf("bottom suffix missing from bottom border:\n%s", out)
	}
	// Suffix hugs the right: it must appear after the midpoint of the line.
	if idx := strings.Index(last, "GO"); idx < lipgloss.Width(last)/2 {
		t.Fatalf("bottom suffix not right-anchored (idx=%d):\n%s", idx, out)
	}
	for _, l := range lines {
		if w := lipgloss.Width(l); w != 12 {
			t.Fatalf("line width = %d, want 12:\n%q in\n%s", w, l, out)
		}
	}
}

func TestRenderBoxBottomSuffixTooNarrowFallsBack(t *testing.T) {
	// Suffix cannot fit: bottom border must fall back to plain, no panic, width preserved.
	out := components.RenderBox("x", "", "", "", "WAYTOOLONG", roundedBorder(), nil, 6, 3)
	lines := strings.Split(out, "\n")
	last := lines[len(lines)-1]
	if strings.Contains(last, "WAYTOOLONG") {
		t.Fatalf("oversized suffix should have been dropped:\n%s", out)
	}
	if w := lipgloss.Width(last); w != 6 {
		t.Fatalf("fallback bottom width = %d, want 6:\n%s", w, out)
	}
}

func TestRenderBoxBottomHintAndSuffixCoexist(t *testing.T) {
	out := components.RenderBox("x", "", "", "esc", "GO", roundedBorder(), nil, 20, 3)
	last := strings.Split(out, "\n")[2]
	if !strings.Contains(last, "esc") || !strings.Contains(last, "GO") {
		t.Fatalf("bottom border must contain both hint and suffix:\n%s", out)
	}
	if strings.Index(last, "esc") > strings.Index(last, "GO") {
		t.Fatalf("hint must be left of suffix:\n%s", out)
	}
}
