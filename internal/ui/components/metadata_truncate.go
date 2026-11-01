package components

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// truncateMetadata bounds plain or already styled labels without splitting
// clusters. The pinned ansi.Truncate counts ASCII-based keycaps as a lone
// digit; segment those too so its result agrees with lipgloss.Width.
func truncateMetadata(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	var out strings.Builder
	var state byte
	used := 0
	clipped := false
	for len(text) > 0 {
		seq, cells, n, next := ansi.DecodeSequence(text, state, nil)
		if n == 0 {
			break
		}
		if n == 1 && seq[0] > 0x1f && seq[0] < 0x7f {
			seq, cells = ansi.FirstGraphemeCluster(text, ansi.GraphemeWidth)
			n = len(seq)
		}
		if !clipped && cells > 0 && used+cells > width-1 {
			out.WriteRune('…')
			clipped = true
		}
		// Keep trailing resets/link closures even after omitting the text.
		if !clipped || cells == 0 {
			out.WriteString(seq)
		}
		used += cells
		text = text[n:]
		state = next
	}
	return out.String()
}
