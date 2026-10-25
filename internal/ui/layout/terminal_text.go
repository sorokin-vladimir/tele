package layout

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// TerminalSafeText gives narrow joined symbols a stable display fallback.
// Some terminals draw these clusters in two cells even though the width library
// reserves one. Replacing them with one ASCII cell keeps the already composed
// geometry intact. Qualified two-cell clusters and ordinary text stay unchanged.
// This belongs at the final view boundary, never in stored or copied text.
func TerminalSafeText(text string) string {
	if !strings.ContainsRune(text, '\u200d') {
		return text
	}
	var out strings.Builder
	var state byte
	start := 0
	for pos := 0; pos < len(text); {
		seq, width, n, next := ansi.DecodeSequence(text[pos:], state, nil)
		if n == 0 {
			break
		}
		if width == 1 && narrowJoinedSymbols(seq) {
			out.WriteString(text[start:pos])
			out.WriteByte('?')
			start = pos + n
		}
		pos += n
		state = next
	}
	if start == 0 {
		return text
	}
	out.WriteString(text[start:])
	return out.String()
}

func narrowJoinedSymbols(cluster string) bool {
	if !strings.ContainsRune(cluster, '\u200d') {
		return false
	}
	// Count symbols, not letters: Indic conjuncts also contain joiners but must
	// retain their original text. Grapheme segmentation supplies the join boundary.
	symbols := 0
	for _, r := range cluster {
		if unicode.IsSymbol(r) {
			symbols++
		}
	}
	return symbols >= 2
}
