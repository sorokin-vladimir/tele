package components_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

func TestBuildReplyPreview_ContainsName(t *testing.T) {
	msg := store.Message{SenderName: "Alice", Text: "hello"}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "Alice")
}

func TestBuildReplyPreview_ContainsSnippet(t *testing.T) {
	msg := store.Message{SenderName: "Alice", Text: "hello world"}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "hello world")
}

func TestBuildReplyPreview_TwoLines(t *testing.T) {
	msg := store.Message{SenderName: "Bob", Text: "hello"}
	preview := components.BuildReplyPreview(msg)
	assert.Equal(t, 1, strings.Count(preview, "\n"), "preview must be exactly two lines")
}

func TestBuildReplyPreview_OutgoingEmptyName(t *testing.T) {
	msg := store.Message{SenderName: "", IsOut: true, Text: "hi"}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "You")
}

func TestBuildReplyPreview_IncomingEmptyName(t *testing.T) {
	msg := store.Message{SenderName: "", IsOut: false, Text: "hi"}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "?")
}

func TestBuildReplyPreview_LongTextTruncated(t *testing.T) {
	msg := store.Message{SenderName: "Bob", Text: strings.Repeat("x", 50)}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "…")
}

func TestBuildReplyPreview_MultilineTextUsesFirstLine(t *testing.T) {
	msg := store.Message{SenderName: "Eve", Text: "first line\nsecond line"}
	preview := components.BuildReplyPreview(msg)
	assert.Contains(t, preview, "first line")
	assert.NotContains(t, preview, "second line")
}
