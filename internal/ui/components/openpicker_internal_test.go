package components

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncateTail_ShortString_Unchanged(t *testing.T) {
	assert.Equal(t, "short", truncateTail("short", 20))
}

func TestTruncateTail_BoundsWidthAndKeepsTail(t *testing.T) {
	s := "https://example.com/very/long/path/to/resource.html"
	out := truncateTail(s, 20)
	assert.LessOrEqual(t, len([]rune(out)), 20, "result fits the budget")
	assert.Contains(t, out, "…")
	assert.True(t, strings.HasSuffix(out, ".html"), "keeps the tail: %q", out)
	assert.True(t, strings.HasPrefix(out, "https://"), "keeps the head: %q", out)
}
