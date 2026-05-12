package components_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// stripANSI removes ANSI escape sequences so we can assert plain text content.
func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*[mKJH]`)
	return re.ReplaceAllString(s, "")
}

func TestRenderEntities_NoEntities_ReturnsInput(t *testing.T) {
	result := components.RenderEntities("hello world", nil)
	assert.Equal(t, "hello world", result)
}

func TestRenderEntities_EmptyEntities_ReturnsInput(t *testing.T) {
	result := components.RenderEntities("hello", []store.MessageEntity{})
	assert.Equal(t, "hello", result)
}

func TestRenderEntities_Bold_TextPreserved(t *testing.T) {
	entities := []store.MessageEntity{{Type: "bold", Offset: 0, Length: 5}}
	result := components.RenderEntities("hello world", entities)
	assert.Contains(t, stripANSI(result), "hello world")
}

func TestRenderEntities_MiddleSegment(t *testing.T) {
	// "foo BAR baz": BAR is italic, offset=4, length=3
	entities := []store.MessageEntity{{Type: "italic", Offset: 4, Length: 3}}
	result := components.RenderEntities("foo BAR baz", entities)
	plain := stripANSI(result)
	assert.Contains(t, plain, "foo ")
	assert.Contains(t, plain, "BAR")
	assert.Contains(t, plain, " baz")
}

func TestRenderEntities_Emoji_UTF16Offset(t *testing.T) {
	// "😀 hi": 😀 is U+1F600 = 2 UTF-16 units; " hi" starts at UTF-16 offset 2
	// Entity: bold on "hi", offset=2, length=2 (UTF-16 units — the space is offset 2, "hi" is 3-4)
	// Actually: 😀=2 units, space=1 unit → "hi" starts at offset 3
	// Let's use: bold on just the space+" hi" starting at offset 2, length 3
	// Simpler: mark " hi" (space+hi) as bold: offset=2, length=3
	entities := []store.MessageEntity{{Type: "bold", Offset: 2, Length: 3}}
	result := components.RenderEntities("😀 hi", entities)
	plain := stripANSI(result)
	assert.Contains(t, plain, "😀")
	assert.Contains(t, plain, "hi")
}

func TestRenderEntities_Code(t *testing.T) {
	entities := []store.MessageEntity{{Type: "code", Offset: 0, Length: 4}}
	result := components.RenderEntities("code stuff", entities)
	plain := stripANSI(result)
	assert.Contains(t, plain, "code stuff")
}

func TestRenderEntities_Pre(t *testing.T) {
	entities := []store.MessageEntity{{Type: "pre", Offset: 0, Length: 5}}
	result := components.RenderEntities("block text", entities)
	plain := stripANSI(result)
	assert.Contains(t, plain, "block text")
}

func TestRenderEntities_UnknownType_PassesThrough(t *testing.T) {
	entities := []store.MessageEntity{{Type: "spoiler", Offset: 0, Length: 5}}
	result := components.RenderEntities("hello world", entities)
	assert.Equal(t, "hello world", result)
}

func TestRenderEntities_OutOfBounds_NoCrashTextPreserved(t *testing.T) {
	entities := []store.MessageEntity{{Type: "bold", Offset: 100, Length: 5}}
	result := components.RenderEntities("short", entities)
	assert.Contains(t, stripANSI(result), "short")
}
