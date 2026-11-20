package textwidth_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/stretchr/testify/assert"
)

func TestWidth_FollowsGoRunewidth(t *testing.T) {
	assert.Equal(t, 1, textwidth.Width("✍️"), "a base glyph with VS16 is as wide as the base")
	assert.Equal(t, 2, textwidth.Width("📖"), "an emoji-presentation glyph is two cells")
	assert.Equal(t, 2, textwidth.Width("💻"))
	assert.Equal(t, 2, textwidth.Width("👨‍👩‍👧‍👦"), "a ZWJ sequence is as wide as its first rune")
	assert.Equal(t, 1, textwidth.Width("🇿🇦"), "a flag is as wide as its first rune")
	assert.Equal(t, 5, textwidth.Width("hello"))
}

func TestWidth_IgnoresEscapes(t *testing.T) {
	assert.Equal(t, 2, textwidth.Width("\x1b[1mhi\x1b[0m"))
}

func TestWrap_ReappliesStyleOnContinuationRows(t *testing.T) {
	out := textwidth.Wrap("\x1b[1mone two three\x1b[0m", 7)
	rows := strings.Split(out, "\n")
	assert.Len(t, rows, 2)
	for _, r := range rows {
		assert.Contains(t, r, "\x1b[1m", "a wrapped row must carry the style it was painted with")
	}
}

func TestWrap_LimitBelowOneReturnsInput(t *testing.T) {
	assert.Equal(t, "abc", textwidth.Wrap("abc", 0))
	assert.Equal(t, "\x1b[1mabc", textwidth.Wrap("\x1b[1mabc", 0))
}

func TestWordwrap_ReappliesStyleOnContinuationRows(t *testing.T) {
	out := textwidth.Wordwrap("\x1b[1mone two three\x1b[0m", 7, "")
	rows := strings.Split(out, "\n")
	assert.Len(t, rows, 2)
	for _, r := range rows {
		assert.Contains(t, r, "\x1b[1m", "a wrapped row must carry the style it was painted with")
	}
}

func TestTruncate_FollowsGoRunewidth(t *testing.T) {
	assert.Equal(t, "✍️ab", textwidth.Truncate("✍️abc", 3, ""))
	assert.Equal(t, "✍️…", textwidth.Truncate("✍️abc", 2, "…"))
}

func TestTruncateLeft_FollowsGoRunewidth(t *testing.T) {
	assert.Equal(t, "bc", textwidth.TruncateLeft("✍️abc", 2, ""))
}

func TestCut_FollowsGoRunewidth(t *testing.T) {
	assert.Equal(t, "ab", textwidth.Cut("✍️abc", 1, 3))
	assert.Equal(t, "", textwidth.Cut("✍️abc", 3, 3))
}
