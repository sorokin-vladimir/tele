package screens

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/stretchr/testify/require"
)

func TestTitlesStayWithinRow(t *testing.T) {
	for _, title := range []string{"2\ufe0f\u20e3 " + strings.Repeat("Channel ", 10), "#\ufe0f\u20e3 " + strings.Repeat("Folder ", 10), "Cafe\u0301 " + strings.Repeat("title ", 10), strings.Repeat("中文", 20)} {
		for _, badge := range []string{"", "[99+]"} {
			for width := 12; width <= 40; width++ {
				t.Run(fmt.Sprintf("%q/%s/%d", title, badge, width), func(t *testing.T) {
					row := rowContent(title, badge, width, theme.S().Body, func(s string) string { return s })
					require.Equal(t, width, lipgloss.Width(row))
				})
			}
		}
	}
}
func TestFolderTitlesStayWithinRow(t *testing.T) {
	for width := 12; width <= 30; width++ {
		m := NewFoldersModel()
		m.SetSize(width, 10)
		m.SetUnreadCounts(map[int]int{1: 100})
		row := m.formatEntry(domain.FolderFilter{ID: 1, Title: "2\ufe0f\u20e3 " + strings.Repeat("Folder ", 10)}, false)
		require.Equal(t, width, lipgloss.Width(row), "width %d", width)
	}
}

func TestTruncateTitlePreservesWholeClusters(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
		want  string
	}{
		{"2\ufe0f\u20e3abc", 2, "…"}, {"2\ufe0f\u20e3abc", 3, "2\ufe0f\u20e3…"},
		{"e\u0301abc", 2, "e\u0301…"}, {"中文abc", 3, "中…"},
		{"👩‍💻abc", 3, "👩‍💻…"}, {"abc", 1, "…"}, {"abc", 0, ""}, {"abc", -1, ""},
		{"2\ufe0f\u20e3", 2, "2\ufe0f\u20e3"}, {"क्ष", 10, "क्ष"},
	} {
		require.Equal(t, tc.want, truncateTitle(tc.text, tc.width))
	}
}
