package components

import (
	"image/color"
	"slices"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// senderPaletteColor is the colour this person is drawn in, picked from
// sender_palette by id. It lives on its own so that everything answering "which
// colour is this person" answers the same: a name in a chat and a monogram in
// the profile overlay must not disagree about someone (#223). Reports false
// when the theme sets no palette, which is a legal theme.
func senderPaletteColor(userID int64) (color.Color, bool) {
	pal := theme.T().SenderPalette
	if len(pal) == 0 {
		return nil, false
	}
	idx := userID % int64(len(pal))
	if idx < 0 {
		idx = -idx
	}
	return pal[idx], true
}

func (ml *MessageList) senderNameStyle(senderID int64) lipgloss.Style {
	c, ok := senderPaletteColor(senderID)
	if !ok {
		return theme.S().BodyBold
	}
	return theme.NewStyle().Foreground(c).Bold(true)
}

// buildReactStr presents whole reactions in descending count order without
// changing the Telegram slice. Width includes the framing spaces.
func buildReactStr(reactions []domain.Reaction, width int) string {
	if len(reactions) == 0 || width <= 0 {
		return ""
	}
	ordered := slices.Clone(reactions)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Count > ordered[j].Count })
	parts := make([]string, 0, len(ordered))
	for _, r := range ordered {
		entry := r.Emoji + " " + strconv.Itoa(r.Count)
		if r.IsChosen {
			entry = theme.S().TickRead.Render(entry)
		} else {
			entry = theme.S().Timestamp.Render(entry)
		}
		parts = append(parts, entry)
	}
	sep := theme.S().Timestamp.Render(" · ")
	frame := func(body string) string { return theme.Pad(1) + body + theme.Pad(1) }
	if all := frame(strings.Join(parts, sep)); lipgloss.Width(all) <= width {
		return all
	}
	ellipsis := theme.S().Timestamp.Render("…")
	if width < 3 {
		return ellipsis
	}
	// Reserve the ellipsis and its separator before accepting each whole entry.
	kept := 0
	for kept < len(parts) {
		body := strings.Join(parts[:kept+1], sep) + sep + ellipsis
		if lipgloss.Width(frame(body)) > width {
			break
		}
		kept++
	}
	if kept == 0 {
		return frame(ellipsis)
	}
	return frame(strings.Join(parts[:kept], sep) + sep + ellipsis)
}
