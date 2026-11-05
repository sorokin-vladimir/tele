package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// RenderNoticeBox renders a one-time startup notice: a bordered box with a
// title, a wrapped body, and a footer that counts down until the notice can be
// dismissed. remaining is in whole seconds; 0 means dismissal is unlocked.
//
// The renderer is stateless. The countdown is owned by the caller so this stays
// a pure function of its inputs. maxW bounds the total rendered width including
// the border.
func RenderNoticeBox(title, body string, remaining, maxW int) string {
	footer := "press any key to continue"
	if remaining > 0 {
		footer = fmt.Sprintf("continue in %ds", remaining)
	}
	return RenderDialog(title, body, footer, maxW)
}

// RenderDialog renders a bordered box with a title, a wrapped body, and a faint
// footer that says which keys answer it. maxW bounds the total rendered width
// including the border.
func RenderDialog(title, body, footer string, maxW int) string {
	const padV, padH = 1, 2
	innerW := maxW - 2 - 2*padH
	if innerW < 10 {
		innerW = 10
	}

	wrapped := theme.S().Body.Width(innerW).Render(body)
	dim := theme.S().Body.Faint(true).Render(footer)
	content := strings.Join([]string{wrapped, "", dim}, "\n")

	lines := strings.Split(content, "\n")
	contentW := 0
	for _, l := range lines {
		if w := textwidth.Width(l); w > contentW {
			contentW = w
		}
	}
	if contentW > innerW {
		contentW = innerW
	}
	padded := theme.NewStyle().Padding(padV, padH).Render(content)
	return RenderBox(padded, title, "", "", "",
		lipgloss.RoundedBorder(), nil, contentW+2*padH+2, len(lines)+2*padV+2)
}
