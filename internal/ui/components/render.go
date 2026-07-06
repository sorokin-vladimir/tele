package components

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	lipcompat "charm.land/lipgloss/v2/compat"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// Adaptive entity colors, readable on dark and light backgrounds; resolved via
// compat.HasDarkBackground, which the app updates on terminal background change.
var (
	linkColor = lipcompat.AdaptiveColor{Light: lipgloss.Color("25"), Dark: lipgloss.Color("45")}  // url/email/phone/bank_card, text_url
	refColor  = lipcompat.AdaptiveColor{Light: lipgloss.Color("90"), Dark: lipgloss.Color("213")} // mention/hashtag/cashtag/bot_command
)

// utf16ToRuneIndex converts a UTF-16 code unit offset to a rune index in s.
// Telegram entity offsets are in UTF-16 code units; Go strings are UTF-8.
func utf16ToRuneIndex(s string, utf16Offset int) int {
	runeIdx := 0
	u16Pos := 0
	for _, r := range s {
		if u16Pos >= utf16Offset {
			break
		}
		if r >= 0x10000 {
			u16Pos += 2
		} else {
			u16Pos++
		}
		runeIdx++
	}
	return runeIdx
}

// isKnownEntity reports whether typ is a rendered inline entity. Unknown types
// (spoiler, blockquote, custom_emoji, …) pass through as plain text.
func isKnownEntity(typ string) bool {
	switch typ {
	case "bold", "italic", "code", "pre", "underline", "strike",
		"text_url", "url", "email", "phone", "bank_card",
		"mention", "hashtag", "cashtag", "bot_command":
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
		return s.Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	case "underline":
		return s.Underline(true)
	case "strike":
		return s.Strikethrough(true)
	case "text_url":
		return s.Foreground(linkColor).Underline(true)
	case "url", "email", "phone", "bank_card":
		return s.Foreground(linkColor)
	case "mention", "hashtag", "cashtag", "bot_command":
		return s.Foreground(refColor)
	}
	return s
}

// RenderEntities applies lipgloss styles to text according to Telegram
// MessageEntity offsets. Offsets and lengths are in UTF-16 code units. The text
// is swept over rune boundaries: each run accumulates every active entity's
// style into one combined lipgloss.Style, so overlapping/nested entities compose
// correctly. text_url runs are additionally wrapped in an OSC 8 hyperlink.
// Unknown types pass through as plain text.
func RenderEntities(text string, entities []store.MessageEntity) string {
	if len(entities) == 0 {
		return text
	}
	runes := []rune(text)
	n := len(runes)

	type span struct {
		start, end int
		typ        string
		url        string
	}
	spans := make([]span, 0, len(entities))
	boundarySet := map[int]struct{}{0: {}, n: {}}
	for _, e := range entities {
		if !isKnownEntity(e.Type) {
			continue
		}
		start := utf16ToRuneIndex(text, e.Offset)
		end := utf16ToRuneIndex(text, e.Offset+e.Length)
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
		spans = append(spans, span{start, end, e.Type, linkTarget})
		boundarySet[start] = struct{}{}
		boundarySet[end] = struct{}{}
	}
	if len(spans) == 0 {
		return text
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
		style := lipgloss.NewStyle()
		styled := false
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
			}
		}
		segment := string(runes[lo:hi])
		if styled {
			segment = style.Render(segment)
		}
		if linkURL != "" {
			segment = osc8(linkID, linkURL, segment)
		}
		b.WriteString(segment)
	}
	return b.String()
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
