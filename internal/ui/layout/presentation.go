package layout

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/unicode/norm"
)

// PresentationText keeps narrow virama conjuncts in an explicit non-joining
// form. The width library groups them into one cell, while terminals including
// Foot advance one cell per consonant. Insert a zero-width non-joiner before
// measuring or wrapping, so both sides agree without removing any letters.
// Apply only to display strings; entity offsets, link targets and stored text
// remain based on the original input.
func PresentationText(text string) string {
	var out strings.Builder
	var state byte
	start := 0
	for pos := 0; pos < len(text); {
		seq, width, n, next := ansi.DecodeSequence(text[pos:], state, nil)
		if n == 0 {
			break
		}
		if width == 1 && hasNarrowConjunct(seq) {
			out.WriteString(text[start:pos])
			letters := 0
			for _, r := range seq {
				if unicode.IsLetter(r) {
					if letters > 0 {
						out.WriteRune('\u200c')
					}
					letters++
				}
				out.WriteRune(r)
			}
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

func hasNarrowConjunct(cluster string) bool {
	letters, virama := 0, false
	for _, r := range cluster {
		if unicode.IsLetter(r) {
			letters++
		}
		// Canonical combining class 9 identifies virama without a script table.
		if unicode.IsMark(r) && norm.NFD.PropertiesString(string(r)).CCC() == 9 {
			virama = true
		}
	}
	return virama && letters > 1
}
