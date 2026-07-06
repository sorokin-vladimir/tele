package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
)

func TestOverlayHint_JoinsPairs(t *testing.T) {
	out := OverlayHint([][2]string{{"space", "pause"}, {"q", "close"}}, nil)
	if !strings.Contains(out, "pause") || !strings.Contains(out, "close") {
		t.Fatalf("overlay hint missing entries: %q", out)
	}
}

func TestHintLayout_LetterInWord_HighlightsInPlace(t *testing.T) {
	text, spans := hintLayout("q", "quit")
	assert.Equal(t, "quit", text)
	assert.Equal(t, []span{{0, 1}}, spans)
}

func TestHintLayout_LetterInWord_MidWord(t *testing.T) {
	// "a" is at rune index 1 of "caption".
	text, spans := hintLayout("a", "caption")
	assert.Equal(t, "caption", text)
	assert.Equal(t, []span{{1, 2}}, spans)
}

func TestHintLayout_LetterInWord_CaseInsensitive(t *testing.T) {
	// A lowercase key matches the capital letter but is shown in the key's case,
	// so the highlighted letter reads as the actual keystroke.
	text, spans := hintLayout("n", "Nice")
	assert.Equal(t, "nice", text)
	assert.Equal(t, []span{{0, 1}}, spans)
}

func TestHintLayout_LetterNotInWord_PrefixForm(t *testing.T) {
	// "attach" has no "f".
	text, spans := hintLayout("f", "attach")
	assert.Equal(t, "f attach", text)
	assert.Equal(t, []span{{0, 1}}, spans)
}

func TestHintLayout_UppercaseKey_HighlightsCapitalInPlace(t *testing.T) {
	// A Shift key matches the word's capital letter and is highlighted in place;
	// the capital already conveys that Shift is required.
	text, spans := hintLayout("O", "Open photo externally")
	assert.Equal(t, "Open photo externally", text)
	assert.Equal(t, []span{{0, 1}}, spans)
}

func TestHintLayout_NonLetterKey_PrefixForm(t *testing.T) {
	text, spans := hintLayout("/", "search")
	assert.Equal(t, "/ search", text)
	assert.Equal(t, []span{{0, 1}}, spans)
}

func TestHintLayout_NamedKey_PrefixForm(t *testing.T) {
	text, spans := hintLayout("esc", "cancel")
	assert.Equal(t, "esc cancel", text)
	assert.Equal(t, []span{{0, 3}}, spans)
}

func TestHintLayout_ComboKey_PrefixForm(t *testing.T) {
	text, spans := hintLayout("ctrl+t", "photo/file")
	assert.Equal(t, "ctrl+t photo/file", text)
	assert.Equal(t, []span{{0, 6}}, spans)
}

func TestHintLayout_Enter_SuffixGlyph(t *testing.T) {
	text, spans := hintLayout("enter", "send")
	assert.Equal(t, "send ↵", text)
	// "send " is 5 runes; the glyph is the 6th rune.
	assert.Equal(t, []span{{5, 6}}, spans)
}

func TestHintLayout_EmptyKey_NoAccent(t *testing.T) {
	text, spans := hintLayout("", "quit")
	assert.Equal(t, "quit", text)
	assert.Nil(t, spans)
}

func TestNavLayout_NonArrowPair_PrefixForm(t *testing.T) {
	text, spans := navLayout("j", "k", "move")
	assert.Equal(t, "j/k move", text)
	assert.Equal(t, []span{{0, 3}}, spans)
}

func TestNavLayout_SharedModifierCollapsed(t *testing.T) {
	text, spans := navLayout("ctrl+j", "ctrl+k", "select")
	assert.Equal(t, "ctrl+j/k select", text)
	assert.Equal(t, []span{{0, 8}}, spans)
}

func TestNavLayout_ArrowPair_GlyphForm(t *testing.T) {
	text, spans := navLayout("down", "up", "select")
	assert.Equal(t, "↑ select ↓", text)
	// 10 runes total; accent the first and last.
	assert.Equal(t, []span{{0, 1}, {9, 10}}, spans)
}

func TestNavLayout_EmptyPair_Empty(t *testing.T) {
	text, spans := navLayout("", "", "move")
	assert.Equal(t, "", text)
	assert.Nil(t, spans)
}

func TestApplyAccent_WrapsOnlySpans(t *testing.T) {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	out := applyAccent("quit", []span{{0, 1}}, barStyle, accent)
	// The rune "q" is styled, "uit" is left as-is.
	assert.Contains(t, out, "uit")
	assert.NotEqual(t, "quit", out) // styling was applied
}

func TestApplyAccent_NoSpans_UsesBaseStyle(t *testing.T) {
	accent := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	// With no accent, the whole text is rendered with the base style.
	assert.Equal(t, barStyle.Render("plain"), applyAccent("plain", nil, barStyle, accent))
}

func TestApplyAccent_NonAccentRunUsesBaseStyle(t *testing.T) {
	accent := lipgloss.NewStyle().Background(barBg).Foreground(lipgloss.Color("39"))
	out := applyAccent("quit", []span{{0, 1}}, barStyle, accent)
	// The non-accent remainder must be styled with the base (bar) style, not
	// left plain (which would lose the background after the accent's reset).
	assert.Contains(t, out, barStyle.Render("uit"))
}

func TestJoinHints_SeparatorKeepsBarBackground(t *testing.T) {
	out := joinHints("a", "b")
	assert.Contains(t, out, barStyle.Render(" · "))
}

func TestAccentStyle_FollowsMode(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetMode(keys.ModeNormal)
	normal := sb.accentStyle()
	sb.SetMode(keys.ModeInsert)
	insert := sb.accentStyle()
	// Different foreground per mode; we assert the rendered escapes differ.
	assert.NotEqual(t, normal.Render("x"), insert.Render("x"))
}
