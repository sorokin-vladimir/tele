package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

const quoteGlyph = "▌ "

func replyName(orig *domain.Message) string {
	if orig.SenderName != "" {
		return orig.SenderName
	}
	if orig.IsOut {
		return "You"
	}
	return "?"
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// measurePreviewBlock returns the content width needed for a reply preview
// block, accounting for both the original sender's name and the quoted snippet,
// capped at maxContentW. Measuring the snippet keeps a short reply (e.g. "ok")
// from squeezing the original message down to an unreadable width.
func measurePreviewBlock(senderName, snippet string, maxContentW int) int {
	w := textwidth.Width(quoteGlyph + theme.S().NameIncoming.Render(senderName))
	if sw := textwidth.Width(quoteGlyph + theme.S().Quote.Render(snippet)); sw > w {
		w = sw
	}
	if w > maxContentW {
		return maxContentW
	}
	return w
}

// renderPreviewLines returns the bubble content lines for a reply or forward
// preview block. senderName == "" signals the original is unavailable
// (renders a placeholder). actualW is the finalized content width for the bubble.
func (ml *MessageList) renderPreviewLines(senderID int64, senderName, snippet string, actualW int, bs lipgloss.Style) []string {
	b := lipgloss.RoundedBorder()
	glyphW := textwidth.Width(quoteGlyph)

	if senderName == "" {
		placeholder := quoteGlyph + theme.S().Quote.Render("Original not available")
		placeholder += theme.PadTo(textwidth.Width(placeholder), actualW)
		return []string{bs.Render(b.Left) + theme.Pad(1) + placeholder + theme.Pad(1) + bs.Render(b.Right)}
	}

	ns := ml.senderNameStyle(senderID)
	namePart := ns.Render(quoteGlyph) + ns.Render(senderName)
	nw := textwidth.Width(namePart)
	if nw > actualW {
		maxNameW := actualW - glyphW - 1
		if maxNameW < 1 {
			maxNameW = 1
		}
		senderName = runewidth.Truncate(senderName, maxNameW, "…")
		namePart = ns.Render(quoteGlyph) + ns.Render(senderName)
		nw = textwidth.Width(namePart)
	}
	namePart += theme.PadTo(nw, actualW)
	nameRow := bs.Render(b.Left) + theme.Pad(1) + namePart + theme.Pad(1) + bs.Render(b.Right)

	maxSnippetW := actualW - glyphW
	if maxSnippetW < 1 {
		maxSnippetW = 1
	}
	snippet = runewidth.Truncate(snippet, maxSnippetW, "…")
	textPart := ns.Render(quoteGlyph) + theme.S().Quote.Render(snippet)
	textPart += theme.PadTo(textwidth.Width(textPart), actualW)
	snippetRow := bs.Render(b.Left) + theme.Pad(1) + textPart + theme.Pad(1) + bs.Render(b.Right)

	return []string{nameRow, snippetRow}
}

const (
	forwardLabelText  = "Forwarded from"
	forwardHiddenName = "Hidden"
)

// measureForwardBlock returns the content width needed for a forwarded-message
// header block (the "Forwarded from" label and the origin name), capped at
// maxContentW.
func measureForwardBlock(from string, maxContentW int) int {
	name := from
	if name == "" {
		name = forwardHiddenName
	}
	w := textwidth.Width(quoteGlyph + theme.S().NameIncoming.Render(name))
	if lw := textwidth.Width(quoteGlyph + theme.S().Quote.Render(forwardLabelText)); lw > w {
		w = lw
	}
	if w > maxContentW {
		return maxContentW
	}
	return w
}

// renderForwardLines returns the bubble content lines for a forwarded-message
// header block: a dim "Forwarded from" label line followed by the origin name.
// An empty from renders the hidden-sender placeholder. actualW is the finalized
// content width for the bubble.
func renderForwardLines(from string, actualW int, bs lipgloss.Style) []string {
	b := lipgloss.RoundedBorder()
	glyphW := textwidth.Width(quoteGlyph)

	name := from
	if name == "" {
		name = forwardHiddenName
	}

	maxTextW := actualW - glyphW
	if maxTextW < 1 {
		maxTextW = 1
	}

	label := runewidth.Truncate(forwardLabelText, maxTextW, "…")
	labelPart := theme.S().Quote.Render(quoteGlyph) + theme.S().Quote.Render(label)
	labelPart += theme.PadTo(textwidth.Width(labelPart), actualW)
	labelRow := bs.Render(b.Left) + theme.Pad(1) + labelPart + theme.Pad(1) + bs.Render(b.Right)

	name = runewidth.Truncate(name, maxTextW, "…")
	namePart := theme.S().Quote.Render(quoteGlyph) + theme.S().NameIncoming.Render(name)
	namePart += theme.PadTo(textwidth.Width(namePart), actualW)
	nameRow := bs.Render(b.Left) + theme.Pad(1) + namePart + theme.Pad(1) + bs.Render(b.Right)

	return []string{labelRow, nameRow}
}
