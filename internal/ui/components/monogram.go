package components

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// A monogram is what a person looks like when there is no picture of them. It
// is a defined answer rather than a gap, and it is the same answer to three
// different questions (#223):
//
//   - the person set no avatar, or their privacy settings withhold it;
//   - the terminal cannot draw images, where an image at avatar size would be a
//     smear of half-blocks and worse than letters;
//   - the avatar is on its way.
//
// Because it stands in for all three, it occupies exactly the cell box an
// avatar would, so a picture arriving replaces it without moving anything.

// monogramLines renders the cols×rows block for a person: their initials,
// framed, in the colour that person's name is drawn in elsewhere.
//
// The frame is what marks it as a stand-in: an avatar is drawn as a circle with
// no border, so the two never read as the same kind of thing even at a glance.
func monogramLines(user domain.User, cols, rows int) []string {
	if cols < 2 || rows < 2 {
		return nil
	}
	b := lipgloss.RoundedBorder()
	bg := theme.NewStyle().Background(theme.T().SurfaceOverlay)
	frame := bg.Foreground(theme.T().TextDim)

	letters := bg.Bold(true).Foreground(theme.T().TextOnSurface)
	if c, ok := senderPaletteColor(user.ID); ok {
		letters = bg.Bold(true).Foreground(c)
	}

	innerW := cols - 2
	label := monogramInitials(user)
	if lipgloss.Width(label) > innerW {
		// A name starting with a wide rune can outgrow a narrow block. One
		// letter says less than two and still says something; none at all is
		// the last resort, and an empty frame is still the right size.
		label = firstRune(label)
		if lipgloss.Width(label) > innerW {
			label = ""
		}
	}

	// The inside of the block is filled by a style rather than by spaces: every
	// cell of an overlay has to carry its surface, or it reads as a hole in the
	// panel (#227). That is also why the letters are centred by the style
	// instead of by padding around them.
	fill := bg.Width(innerW)
	lines := make([]string, 0, rows)
	lines = append(lines, frame.Render(b.TopLeft+strings.Repeat(b.Top, innerW)+b.TopRight))
	for r := 1; r < rows-1; r++ {
		row := fill.Render("")
		// The letters sit on the block's middle row. With an even number of rows
		// there is no true middle, and the upper of the two candidates is the
		// one that reads as centred: a line of text sits optically high in the
		// cell it occupies.
		if r == (rows-1)/2 && label != "" {
			row = fill.Align(lipgloss.Center).Render(letters.Render(label))
		}
		lines = append(lines, frame.Render(b.Left)+row+frame.Render(b.Right))
	}
	lines = append(lines, frame.Render(b.BottomLeft+strings.Repeat(b.Bottom, innerW)+b.BottomRight))
	return lines
}

// monogramInitials picks the letters: the first of the given name and the first
// of the family name when there is one.
//
// A deleted account has neither, and DisplayName falls back to "User 12345", so
// it yields "U" — a degenerate case with a defined answer rather than a hole.
func monogramInitials(user domain.User) string {
	first := firstRune(strings.TrimSpace(user.FirstName))
	last := firstRune(strings.TrimSpace(user.LastName))
	if first == "" {
		first = firstRune(user.DisplayName())
	}
	return strings.ToUpper(first + last)
}

// firstRune returns s's first rune as a string, skipping over leading
// whitespace. Empty for an empty string.
func firstRune(s string) string {
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		return string(r)
	}
	return ""
}
