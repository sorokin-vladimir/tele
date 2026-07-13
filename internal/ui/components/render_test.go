package components_test

import (
	"regexp"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

// stripANSI removes ANSI escape sequences (SGR and OSC 8 hyperlinks) so we can
// assert plain text content.
func stripANSI(s string) string {
	sgr := regexp.MustCompile(`\x1b\[[0-9;]*[mKJH]`)
	osc8 := regexp.MustCompile(`\x1b\]8;[^\x1b]*\x1b\\`)
	return sgr.ReplaceAllString(osc8.ReplaceAllString(s, ""), "")
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

func TestRenderEntities_Strike(t *testing.T) {
	entities := []store.MessageEntity{{Type: "strike", Offset: 0, Length: 3}}
	result := components.RenderEntities("abc def", entities)
	assert.Contains(t, stripANSI(result), "abc def")
	assert.NotEqual(t, "abc def", result) // a style escape was emitted
}

func TestRenderEntities_Underline(t *testing.T) {
	entities := []store.MessageEntity{{Type: "underline", Offset: 0, Length: 3}}
	result := components.RenderEntities("abc def", entities)
	assert.Contains(t, stripANSI(result), "abc def")
	assert.NotEqual(t, "abc def", result)
}

func TestRenderEntities_LinkGroupColored(t *testing.T) {
	for _, typ := range []string{"url", "email", "phone", "bank_card"} {
		entities := []store.MessageEntity{{Type: typ, Offset: 0, Length: 3}}
		result := components.RenderEntities("abc def", entities)
		assert.Contains(t, stripANSI(result), "abc def", typ)
		assert.NotEqual(t, "abc def", result, typ)
	}
}

func TestRenderEntities_RefGroupColored(t *testing.T) {
	for _, typ := range []string{"mention", "hashtag", "cashtag", "bot_command"} {
		entities := []store.MessageEntity{{Type: typ, Offset: 0, Length: 3}}
		result := components.RenderEntities("abc def", entities)
		assert.Contains(t, stripANSI(result), "abc def", typ)
		assert.NotEqual(t, "abc def", result, typ)
	}
}

// Overlapping/nested entities must both survive: the visible text is intact and
// the render differs from applying either entity alone.
func TestRenderEntities_OverlapNested(t *testing.T) {
	// "hello world": bold over [0,11), italic over [6,11) (nested)
	entities := []store.MessageEntity{
		{Type: "bold", Offset: 0, Length: 11},
		{Type: "italic", Offset: 6, Length: 5},
	}
	result := components.RenderEntities("hello world", entities)
	assert.Contains(t, stripANSI(result), "hello world")

	boldOnly := components.RenderEntities("hello world", entities[:1])
	assert.NotEqual(t, boldOnly, result) // italic also took effect inside the bold span
}

func TestRenderEntities_TextURL_StyledNoCrash(t *testing.T) {
	entities := []store.MessageEntity{{Type: "text_url", Offset: 0, Length: 5, URL: "https://example.com"}}
	result := components.RenderEntities("click here", entities)
	assert.Contains(t, stripANSI(result), "click here")
}

func TestRenderEntities_TextURL_EmitsOSC8WithURL(t *testing.T) {
	entities := []store.MessageEntity{{Type: "text_url", Offset: 0, Length: 5, URL: "https://example.com/x"}}
	result := components.RenderEntities("click here", entities)
	// OSC 8 open sequence with the target URL, and a closing sequence.
	assert.Contains(t, result, "\x1b]8;id=1;https://example.com/x\x1b\\")
	assert.Contains(t, result, "\x1b]8;;\x1b\\")
}

func TestRenderEntities_PlainURLWithScheme_EmitsOSC8(t *testing.T) {
	text := "see https://example.com now"
	entities := []store.MessageEntity{{Type: "url", Offset: 4, Length: 19}}
	result := components.RenderEntities(text, entities)
	assert.Contains(t, result, "8;id=1;https://example.com\x1b\\", "plain url must be wrapped in OSC 8")
	assert.Contains(t, stripANSI(result), "https://example.com", "visible text unchanged")
}

func TestRenderEntities_SchemelessURL_LinkedAsHTTPS(t *testing.T) {
	text := "visit example.com today"
	entities := []store.MessageEntity{{Type: "url", Offset: 6, Length: 11}}
	result := components.RenderEntities(text, entities)
	assert.Contains(t, result, "8;id=1;https://example.com\x1b\\",
		"a scheme-less url must be linked as https://")
	assert.Contains(t, stripANSI(result), "example.com", "visible text stays as typed (no scheme added)")
}

func TestRenderEntities_Email_LinkedViaMailto(t *testing.T) {
	text := "mail me a@b.com ok"
	entities := []store.MessageEntity{{Type: "email", Offset: 8, Length: 7}}
	result := components.RenderEntities(text, entities)
	assert.Contains(t, result, "8;id=1;mailto:a@b.com\x1b\\",
		"an email entity must be linked via mailto:")
}

// Validation gate: OSC 8 escapes must be zero-width so word-wrap and the
// height cache (wrappedLineCount, which calls RenderEntities) stay correct.
func TestRenderEntities_OSC8_ZeroWidth(t *testing.T) {
	plain := "click here"
	entities := []store.MessageEntity{{Type: "text_url", Offset: 0, Length: 5, URL: "https://example.com/very/long/path"}}
	result := components.RenderEntities(plain, entities)
	assert.Equal(t, lipgloss.Width(plain), lipgloss.Width(result),
		"OSC 8 + SGR escapes must not add display width")
}

func TestRenderMentionNameStyled(t *testing.T) {
	components.SetSelfIdentity(0, "") // not me
	entities := []store.MessageEntity{{Type: "mention_name", Offset: 0, Length: 5, UserID: 123}}
	out := components.RenderEntities("Alice hello", entities)
	if out == "Alice hello" {
		t.Fatal("mention_name should be styled, got plain text")
	}
}

func TestRenderSelfMentionByUserID(t *testing.T) {
	components.SetSelfIdentity(123, "me")
	defer components.SetSelfIdentity(0, "")
	entities := []store.MessageEntity{{Type: "mention_name", Offset: 0, Length: 5, UserID: 123}}
	me := components.RenderEntities("Alice hello", entities)
	components.SetSelfIdentity(999, "other")
	notMe := components.RenderEntities("Alice hello", entities)
	if me == notMe {
		t.Fatal("self mention (by user id) must render differently from a non-self mention")
	}
}

func TestRenderSelfMentionByUsername(t *testing.T) {
	components.SetSelfIdentity(123, "me")
	defer components.SetSelfIdentity(0, "")
	entities := []store.MessageEntity{{Type: "mention", Offset: 0, Length: 3}}
	me := components.RenderEntities("@me hi", entities)
	components.SetSelfIdentity(123, "someoneelse")
	notMe := components.RenderEntities("@me hi", entities)
	if me == notMe {
		t.Fatal("self mention (by username) must render differently")
	}
}
