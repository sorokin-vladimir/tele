package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/textwidth"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// ReactConfirmedMsg is emitted when the user selects an emoji.
type ReactConfirmedMsg struct{ Emoji string }

// CloseReactionPickerMsg is emitted when the user closes the picker without selecting.
type CloseReactionPickerMsg struct{}

var pickerEmoji = [4][7]string{
	{"🤝", "🙏", "👍", "👌", "✍️", "😁", "😈"},
	{"💯", "🖕", "❤️", "🫡", "👏", "👨‍💻", "👀"},
	{"😎", "🏆", "🤔", "🤷", "😢", "🥰", "🦄"},
	{"🤡", "🔥", "💩", "😱", "👎", "😡", "🤬"},
}

const (
	pickerRows = 4
	pickerCols = 7
)

// ReactionPicker is a 5×4 emoji grid overlay. It does not know the target message ID;
// the caller stores the msgID and uses it when handling ReactConfirmedMsg.
type ReactionPicker struct {
	row    int
	col    int
	chosen string // emoji already chosen by the user (highlighted), or ""
}

// NewReactionPicker creates a picker; chosen is the emoji already reacted with (or "").
// If chosen is found in the grid the cursor is pre-positioned on it.
func NewReactionPicker(chosen string) *ReactionPicker {
	p := &ReactionPicker{chosen: chosen}
	if chosen != "" {
		for r := 0; r < pickerRows; r++ {
			for c := 0; c < pickerCols; c++ {
				if pickerEmoji[r][c] == chosen {
					p.row, p.col = r, c
					return p
				}
			}
		}
	}
	return p
}

func (p *ReactionPicker) Update(msg tea.Msg) (*ReactionPicker, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	// Translate non-Latin layout keys to their Latin equivalent so the hjkl
	// bindings fire on the same physical key in any supported layout.
	switch keys.NormalizeKey(kp.String()) {
	case "j", "down":
		if p.row < pickerRows-1 {
			p.row++
		}
	case "k", "up":
		if p.row > 0 {
			p.row--
		}
	case "h", "left":
		if p.col > 0 {
			p.col--
		}
	case "l", "right":
		if p.col < pickerCols-1 {
			p.col++
		}
	case "enter":
		emoji := pickerEmoji[p.row][p.col]
		return nil, func() tea.Msg { return ReactConfirmedMsg{Emoji: emoji} }
	case "esc":
		return nil, func() tea.Msg { return CloseReactionPickerMsg{} }
	}
	return p, nil
}

func (p *ReactionPicker) View() string {
	b := lipgloss.RoundedBorder()

	cellW := 4
	innerW := pickerCols*cellW + 1

	rows := make([]string, pickerRows)
	for r := 0; r < pickerRows; r++ {
		var cells []string
		for c := 0; c < pickerCols; c++ {
			emoji := pickerEmoji[r][c]
			cell := " " + emoji + " "
			switch {
			case r == p.row && c == p.col:
				cell = theme.S().PickerSelected.Render(cell)
			case emoji == p.chosen:
				cell = theme.S().PickerChosen.Render(cell)
			default:
				cell = theme.S().PickerBg.Render(cell)
			}
			cells = append(cells, cell)
		}
		rows[r] = theme.S().PickerBg.Render(" ") + strings.Join(cells, theme.S().PickerBg.Render(""))
	}

	hint := OverlayHint([][2]string{
		{"hjkl", DescribeShort(keys.ContextContextMenu, keys.ActionDown)},
		{"enter", DescribeShort(keys.ContextContextMenu, keys.ActionReact)},
		{"esc", DescribeShort(keys.ContextContextMenu, keys.ActionCancel)},
	}, OverlayMenuBg())
	hintW := textwidth.Width(" " + hint + " ")
	if innerW < hintW {
		innerW = hintW
	}

	outerW := innerW + 2
	outerH := pickerRows + 2
	box := RenderBox(strings.Join(rows, "\n"), "React", "", hint, "", b, nil, outerW, outerH)

	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = theme.S().PickerBg.Render(l)
	}
	return strings.Join(lines, "\n")
}
