package screens

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/stretchr/testify/require"
)

func TestChatTitleTruncationStaysInsideRow(t *testing.T) {
	for _, title := range []string{"\U0001f441\u200d\U0001f5e8 " + strings.Repeat("Channel ", 10), "2\ufe0f\u20e3 " + strings.Repeat("Channel ", 10)} {
		for _, badge := range []string{"", "[99+]"} {
			for width := 8; width <= 40; width++ {
				row := rowContent(title, badge, width, theme.S().Body, func(s string) string { return s })
				require.Equal(t, width, lipgloss.Width(row), "title=%q width=%d", title, width)
			}
		}
	}
}
