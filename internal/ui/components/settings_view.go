package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// innerWidth is the room inside the box, which both the rows and the box itself
// are built to.
func (s *SettingsModal) innerWidth() int {
	w := s.width - 2*settingsMargin - 2
	if w > settingsMaxWidth {
		w = settingsMaxWidth
	}
	if w < 20 {
		w = 20
	}
	return w
}

// viewportH is the number of body rows visible inside the box.
func (s *SettingsModal) viewportH() int {
	vh := s.height - 2*settingsMargin - 2 /*borders*/ - 1 /*bottom hint*/
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (s *SettingsModal) clampOffset() {
	max := len(s.items) - s.viewportH()
	if max < 0 {
		max = 0
	}
	if s.offset > max {
		s.offset = max
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// line renders one item. The cursor is drawn as a marker in the left margin
// rather than as a filled row: the rows carry colour of their own - a value, its
// notes - and a selection bar would have to fight all of it.
func (s *SettingsModal) line(i int) string {
	it := s.items[i]
	switch {
	case it.blank:
		return ""
	case it.heading != "":
		return theme.S().HelpSection.Render(it.heading)
	}

	cursor := "  "
	if i == s.cursor && it.selectable() {
		cursor = "› "
	}

	label := it.row.label
	if len(label) > s.labelCol {
		label = label[:s.labelCol]
	}
	// canvas:ok rendered through HelpBg below, so the padding carries the modal
	// surface rather than the canvas behind it.
	pad := strings.Repeat(" ", max(0, s.labelCol-len(label)))

	value, notes := it.row.value, it.row.notes
	if s.editing != nil && s.editing.item == i {
		value, notes = s.editing.buffer+"▌", editingNotes(it)
	}

	// A row is one line. A value too long for what is left of it - a state
	// directory buried under a temp path - is shortened rather than wrapped:
	// wrapping puts the label on a line of its own and the column disappears.
	room := s.innerWidth() - 2 - s.labelCol - 2
	for _, note := range notes {
		room -= len(note) + 3 // the note, its brackets and the space before it
	}
	value = fitValue(value, room)

	valueStyle := theme.S().HelpKey
	if it.row.muted && s.editing == nil {
		valueStyle = theme.S().HelpDesc
	}

	line := theme.S().HelpFaint.Render(cursor) + theme.S().HelpDesc.Render(label) +
		theme.S().HelpBg.Render(pad+"  ") + valueStyle.Render(value)
	for _, note := range notes {
		// Bracketed as well as coloured. A note is about the value beside it,
		// not more of it, and colour alone does not carry that on a low-contrast
		// theme or a terminal rendering the panel flat.
		line += theme.S().HelpBg.Render(" ") + theme.S().HelpFaint.Render("["+note+"]")
	}
	return line
}

// editingNotes says what unit is being typed in, on the row being typed into,
// where a person is looking when the question arises.
func editingNotes(it settingsItem) []string {
	switch {
	case it.entry == nil:
		return nil
	case it.entry.Widget == settings.Bytes:
		return []string{"MB"}
	case it.entry.Unit != "":
		return []string{it.entry.Unit}
	}
	return nil
}

// hint is the bottom line: what happened, if anything did, and otherwise what
// the keys do here. It changes with what is under the cursor, because "enter
// change" is a lie on a row that cannot be changed.
func (s *SettingsModal) hint() string {
	if s.status != "" {
		return theme.S().HelpKey.Render(" " + s.status + " ")
	}
	if s.editing != nil {
		return OverlayHint([][2]string{{"enter", "save"}, {"esc", "cancel"}}, theme.T().SurfaceHelp)
	}
	pairs := [][2]string{{"j/k", "move"}}
	if it, ok := s.current(); ok && !it.entry.ReadOnly {
		switch it.entry.Widget {
		case settings.Toggle:
			pairs = append(pairs, [2]string{"enter", "toggle"})
		case settings.Choice:
			pairs = append(pairs, [2]string{"h/l", "choose"})
		default:
			pairs = append(pairs, [2]string{"enter", "edit"})
		}
		pairs = append(pairs, [2]string{"r", "default"})
	}
	pairs = append(pairs, [2]string{"esc", "close"})
	return OverlayHint(pairs, theme.T().SurfaceHelp)
}

func (s *SettingsModal) View() string {
	vh := s.viewportH()
	innerW := s.innerWidth()

	end := s.offset + vh
	if end > len(s.items) {
		end = len(s.items)
	}
	visible := make([]string, 0, vh)
	for i := s.offset; i < end; i++ {
		visible = append(visible, theme.S().HelpBg.Width(innerW).MaxWidth(innerW).Render(s.line(i)))
	}
	for len(visible) < vh {
		visible = append(visible, theme.S().HelpBg.Width(innerW).Render(""))
	}

	scrollNote := ""
	if len(s.items) > vh {
		scrollNote = theme.S().HelpDesc.Render(fmt.Sprintf(" %d-%d/%d", s.offset+1, end, len(s.items)))
	}

	box := RenderBox(strings.Join(visible, "\n"), theme.S().HelpTitle.Render("Settings"), "",
		s.hint()+scrollNote, "", lipgloss.RoundedBorder(), theme.T().BorderOverlay, innerW+2, vh+2)
	boxLines := strings.Split(box, "\n")
	for i, l := range boxLines {
		boxLines[i] = theme.S().HelpBg.Render(l)
	}
	return strings.Join(boxLines, "\n")
}

// LinesForTest exposes the rendered rows so a test can read what is on screen
// without parsing a box.
func (s *SettingsModal) LinesForTest() []string {
	out := make([]string, 0, len(s.items))
	for i := range s.items {
		out = append(out, s.line(i))
	}
	return out
}

// StatusForTest is what the overlay is currently saying about the last thing
// that happened.
func (s *SettingsModal) StatusForTest() string { return s.status }

// CursorLabelForTest names the row the cursor is on.
func (s *SettingsModal) CursorLabelForTest() string {
	if it, ok := s.current(); ok {
		return it.row.label
	}
	return ""
}
