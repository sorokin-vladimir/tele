package components

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/markup"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

var (
	selfMentionID   int64
	selfMentionUser string // lowercase, without leading '@'
)

// SetSelfIdentity records the signed-in user so @me mentions can be highlighted
// distinctly. Updated by the root model after auth. 256-color-safe styling.
func SetSelfIdentity(userID int64, username string) {
	selfMentionID = userID
	selfMentionUser = strings.ToLower(username)
}

// isSelfMention reports whether a span refers to the signed-in user. For
// mention_name it matches by user id; for a plain @username mention it compares
// the visible text against the stored username.
func isSelfMention(typ string, userID int64, visible string) bool {
	switch typ {
	case "mention_name":
		return selfMentionID != 0 && userID == selfMentionID
	case "mention":
		return selfMentionUser != "" && strings.EqualFold(strings.TrimPrefix(visible, "@"), selfMentionUser)
	}
	return false
}

// isKnownEntity reports whether typ is a rendered inline entity. Unknown types
// (spoiler, blockquote, custom_emoji, …) pass through as plain text.
func isKnownEntity(typ string) bool {
	switch typ {
	case "bold", "italic", "code", "pre", "underline", "strike",
		"text_url", "url", "email", "phone", "bank_card",
		"mention", "mention_name", "hashtag", "cashtag", "bot_command":
		return true
	}
	return false
}

// applyEntityStyle layers the attributes for typ onto s, so overlapping entities
// accumulate into one combined style.
func applyEntityStyle(s lipgloss.Style, typ string) lipgloss.Style {
	switch typ {
	case "bold":
		return s.Bold(true)
	case "italic":
		return s.Italic(true)
	case "code", "pre":
		return s.Background(theme.T().SurfaceCode).Foreground(theme.T().TextCode)
	case "underline":
		return s.Underline(true)
	case "strike":
		return s.Strikethrough(true)
	case "text_url":
		return s.Foreground(theme.T().MarkupLink).Underline(true)
	case "url", "email", "phone", "bank_card":
		return s.Foreground(theme.T().MarkupLink)
	case "mention", "mention_name", "hashtag", "cashtag", "bot_command":
		return s.Foreground(theme.T().MarkupRef)
	}
	return s
}

// RenderEntities applies lipgloss styles to text according to Telegram
// MessageEntity offsets. Offsets and lengths are in UTF-16 code units. The text
// is swept over rune boundaries: each run accumulates every active entity's
// style into one combined lipgloss.Style, so overlapping/nested entities compose
// correctly. text_url runs are additionally wrapped in an OSC 8 hyperlink.
// Unknown types pass through as plain text.
func RenderEntities(text string, entities []domain.MessageEntity) string {
	// The two short circuits go through the body style rather than returning the
	// text raw. Most messages carry no entity at all, so this is the ordinary
	// path, not a corner: returning raw text here left the body of nearly every
	// message outside the theme's reach — unpainted by the canvas, and not even
	// taking the text token that shipped before it.
	if len(entities) == 0 {
		return theme.S().Body.Render(text)
	}
	runes := []rune(text)
	n := len(runes)

	type span struct {
		start, end int
		typ        string
		url        string
		userID     int64
	}
	spans := make([]span, 0, len(entities))
	boundarySet := map[int]struct{}{0: {}, n: {}}
	for _, e := range entities {
		if !isKnownEntity(e.Type) {
			continue
		}
		start := markup.UTF16ToRuneIndex(text, e.Offset)
		end := markup.UTF16ToRuneIndex(text, e.Offset+e.Length)
		if start >= n || start >= end {
			continue
		}
		if end > n {
			end = n
		}
		// Resolve the hyperlink target: text_url carries a hidden URL; plain
		// url/email link to their own visible text (scheme-normalized).
		linkTarget := e.URL
		if e.Type == "url" || e.Type == "email" {
			linkTarget = normalizeLinkTarget(e.Type, string(runes[start:end]))
		}
		spans = append(spans, span{start, end, e.Type, linkTarget, e.UserID})
		boundarySet[start] = struct{}{}
		boundarySet[end] = struct{}{}
	}
	if len(spans) == 0 {
		return theme.S().Body.Render(text)
	}

	bounds := make([]int, 0, len(boundarySet))
	for b := range boundarySet {
		bounds = append(bounds, b)
	}
	sort.Ints(bounds)

	var b strings.Builder
	for i := 0; i+1 < len(bounds); i++ {
		lo, hi := bounds[i], bounds[i+1]
		if lo >= hi {
			continue
		}
		style := theme.NewStyle()
		styled := false
		self := false
		linkURL := ""
		linkID := 0
		for idx, s := range spans {
			if s.start <= lo && hi <= s.end {
				style = applyEntityStyle(style, s.typ)
				styled = true
				if s.url != "" && (s.typ == "text_url" || s.typ == "url" || s.typ == "email") {
					linkURL = s.url
					linkID = idx + 1
				}
				if isSelfMention(s.typ, s.userID, string(runes[s.start:s.end])) {
					self = true
				}
			}
		}
		segment := string(runes[lo:hi])
		switch {
		case self:
			segment = renderPerLine(theme.S().SelfMention, segment)
		case styled:
			segment = renderPerLine(style, segment)
		default:
			// Plain message text. Rendered through the body style rather than
			// emitted raw, so a theme that sets Text owns it; with Text unset
			// this is byte-for-byte the raw segment.
			segment = renderPerLine(theme.S().Body, segment)
		}
		if linkURL != "" {
			segment = osc8(linkID, linkURL, segment)
		}
		b.WriteString(segment)
	}
	return b.String()
}

// renderPerLine styles s one line at a time, because lipgloss.Style.Render
// block-aligns a multi-line string: it pads every line out to the width of the
// widest line in the block.
//
// A run here spans whatever lies between two entity boundaries, so every plain
// run between two entities carries the paragraph breaks that sit between them.
// Handing that to Render put real columns inside the message: the styled span
// landed far to the right of the word before it, the wrapper broke lines where
// the source never would, and the trailing spaces sat inside an SGR run where
// the render path's TrimRight could not reach them. Styling line by line keeps
// the run-by-run styling and removes the alignment entirely (#232).
//
// An empty line is left empty rather than styled: there is no cell to paint,
// and the row it becomes is filled by the bubble renderer, which carries the
// canvas itself.
func renderPerLine(style lipgloss.Style, s string) string {
	if !strings.Contains(s, "\n") {
		return style.Render(s)
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line == "" {
			continue
		}
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

// normalizeLinkTarget turns the visible text of a plain url/email entity into an
// openable target: emails become mailto: links, and scheme-less URLs get an
// https:// prefix. Text already carrying a scheme is left untouched.
func normalizeLinkTarget(typ, text string) string {
	switch typ {
	case "email":
		return "mailto:" + text
	case "url":
		if strings.Contains(text, "://") {
			return text
		}
		return "https://" + text
	}
	return text
}

// osc8 wraps s in an OSC 8 hyperlink to url. Terminals without OSC 8 support
// ignore the escapes and show s unchanged. The id keeps a link unified when it
// spans multiple styled runs.
func osc8(id int, url, s string) string {
	return fmt.Sprintf("\x1b]8;id=%d;%s\x1b\\%s\x1b]8;;\x1b\\", id, url, s)
}
