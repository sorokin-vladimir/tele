package components

import (
	"strings"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// BuildReplyPreview returns a two-line preview string for the reply bar above the composer:
//
//	line 1: "▌ SenderName"
//	line 2: "▌ first-line-of-text (truncated to 40 runes)"
func BuildReplyPreview(msg store.Message) string {
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
	runes := []rune(snippet)
	if len(runes) > 40 {
		snippet = string(runes[:39]) + "…"
	}

	nameLine := quoteGlyph + inNameStyle.Render(name)
	snippetLine := quoteGlyph + quoteStyle.Render(snippet)
	return nameLine + "\n" + snippetLine
}
