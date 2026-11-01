package screens

import "github.com/charmbracelet/x/ansi"

// truncateTitle uses the same grapheme widths as the row's measurement. Titles
// are unstyled here; preserve complete clusters, including ASCII-based keycaps.
func truncateTitle(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	remaining, end := width-1, 0
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
