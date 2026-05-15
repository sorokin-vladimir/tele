package components

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
)

var (
	boldStyle   = lipgloss.NewStyle().Bold(true)
	italicStyle = lipgloss.NewStyle().Italic(true)
	codeStyle   = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
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

// RenderEntities applies lipgloss styles to text according to Telegram MessageEntity offsets.
// Offsets and lengths are in UTF-16 code units. Entities are applied left-to-right;
// unknown types are passed through as plain text.
func RenderEntities(text string, entities []store.MessageEntity) string {
	if len(entities) == 0 {
		return text
	}
	runes := []rune(text)

	type segment struct {
		start, end int
		style      *lipgloss.Style
	}
	segs := make([]segment, 0, len(entities))
	for _, e := range entities {
		var s lipgloss.Style
		switch e.Type {
		case "bold":
			s = boldStyle
		case "italic":
			s = italicStyle
		case "code", "pre":
			s = codeStyle
		default:
			continue
		}
		start := utf16ToRuneIndex(text, e.Offset)
		end := utf16ToRuneIndex(text, e.Offset+e.Length)
		if start >= len(runes) || start >= end {
			continue
		}
		if end > len(runes) {
			end = len(runes)
		}
		sCopy := s
		segs = append(segs, segment{start, end, &sCopy})
	}
	if len(segs) == 0 {
		return text
	}

	var b strings.Builder
	pos := 0
	for _, seg := range segs {
		if pos < seg.start {
			b.WriteString(string(runes[pos:seg.start]))
		}
		b.WriteString(seg.style.Render(string(runes[seg.start:seg.end])))
		pos = seg.end
	}
	if pos < len(runes) {
		b.WriteString(string(runes[pos:]))
	}
	return b.String()
}
