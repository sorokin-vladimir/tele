package layout

import "github.com/charmbracelet/x/ansi"

// TruncateText bounds unstyled text using the same grapheme widths as layout.
// Walking clusters also handles ASCII-based keycaps, which rune-based and
// byte-fast-path truncators can count as a lone digit.
func TruncateText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = PresentationText(text)
	if ansi.StringWidth(text) <= width {
		return text
	}
	remaining := width - 1 // ellipsis
	end := 0
	for end < len(text) {
		cluster, cells := ansi.FirstGraphemeCluster(text[end:], ansi.GraphemeWidth)
		if len(cluster) == 0 || cells > remaining {
			break
		}
		end += len(cluster)
		remaining -= cells
	}
	return text[:end] + "…"
}
