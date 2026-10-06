package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// ConfirmResult is what a ConfirmModal made of a key press: nothing yet, a
// confirmation, or a cancellation.
type ConfirmResult int

const (
	ConfirmNone ConfirmResult = iota
	ConfirmYes
	ConfirmNo
)

// ConfirmModal is a small centred yes/no dialog. It owns every key while it is
// open and answers with ConfirmYes on y or enter and ConfirmNo on n, esc or q.
//
// It is drawn on the same opaque surface as the help modal, for the same
// reason: an overlay that lets the background through is unreadable on one of
// the two terminal backgrounds.
type ConfirmModal struct {
	title    string
	question string
	width    int
	height   int
}

// NewConfirmModal builds a dialog asking question under title. width and height
// are the terminal size, used only to bound the box.
func NewConfirmModal(title, question string, width, height int) *ConfirmModal {
	return &ConfirmModal{title: title, question: question, width: width, height: height}
}

func (c *ConfirmModal) SetSize(w, h int) {
	c.width, c.height = w, h
}

// Update handles a key press. It returns the (unchanged) modal and what the key
// meant: y/enter confirm, n/esc/q cancel, anything else is ConfirmNone and the
// modal stays open.
func (c *ConfirmModal) Update(msg tea.KeyPressMsg) (*ConfirmModal, ConfirmResult) {
	switch keys.NormalizeKey(msg.String()) {
	case "y", "enter":
		return c, ConfirmYes
	case "n", "esc", "q":
		return c, ConfirmNo
	}
	return c, ConfirmNone
}

func (c *ConfirmModal) View() string {
	innerW := lipgloss.Width(c.question) + 4
	if innerW < 20 {
		innerW = 20
	}
	// Bound by the terminal so a narrow window does not tear the box.
	if c.width > 0 {
		if maxW := c.width - 2*helpMargin - 2; innerW > maxW {
			innerW = maxW
		}
	}
	if innerW < 8 {
		innerW = 8
	}

	question := c.question
	if lipgloss.Width(question) > innerW {
		question = runewidth.Truncate(question, innerW, "…")
	}
	// Centre the question in the box and paint the whole row with the surface.
	left := (innerW - lipgloss.Width(question)) / 2
	content := theme.S().HelpBg.Render(theme.Pad(left)) +
		theme.S().HelpDesc.Render(question) +
		theme.S().HelpBg.Width(innerW-left-lipgloss.Width(question)).Render("")

	hint := OverlayHint([][2]string{{"y", "quit"}, {"n", "cancel"}}, theme.T().SurfaceHelp)
	box := RenderBox(content, theme.S().HelpTitle.Render(c.title), "", hint, "",
		lipgloss.RoundedBorder(), theme.T().BorderOverlay, innerW+2, 3)

	// Bake the modal background onto every line so the border and any gaps share
	// the fill (each inner run already sets its own bg, so it survives).
	boxLines := strings.Split(box, "\n")
	for i, l := range boxLines {
		boxLines[i] = theme.S().HelpBg.Render(l)
	}
	return strings.Join(boxLines, "\n")
}
