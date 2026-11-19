package components

import (
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// BuildEditPreview returns a two-line preview string for the edit bar above the composer:
//
//	line 1: "▌ Edit Message"
//	line 2: "▌ first-line-of-text (truncated to 40 runes)"
func BuildEditPreview(msg domain.Message) string {
	snippet := msg.Text
	if idx := strings.IndexByte(snippet, '\n'); idx >= 0 {
		snippet = snippet[:idx]
	}
	snippet = xansi.Truncate(snippet, 39, "…")
	nameLine := quoteGlyph + theme.S().NameEditing.Render("Edit Message")
	snippetLine := quoteGlyph + theme.S().Quote.Render(snippet)
	return nameLine + "\n" + snippetLine
}

// BuildReplyPreview returns a two-line preview string for the reply bar above the composer:
//
//	line 1: "▌ SenderName"
//	line 2: "▌ first-line-of-text (truncated to 40 runes)"
func BuildReplyPreview(msg domain.Message) string {
	name := msg.SenderName
	if name == "" {
		if msg.IsOut {
			name = "You"
		} else {
			name = "?"
		}
	}

	snippet := msg.Text
	if idx := strings.IndexByte(snippet, '\n'); idx >= 0 {
		snippet = snippet[:idx]
	}
	snippet = xansi.Truncate(snippet, 39, "…")

	nameLine := quoteGlyph + theme.S().NameIncoming.Render(name)
	snippetLine := quoteGlyph + theme.S().Quote.Render(snippet)
	return nameLine + "\n" + snippetLine
}
