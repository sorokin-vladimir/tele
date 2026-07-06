package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// OpenTargetChosenMsg is emitted when the user picks a target to open.
type OpenTargetChosenMsg struct{ Target OpenTarget }

// CloseOpenPickerMsg is emitted when the user dismisses the picker.
type CloseOpenPickerMsg struct{}

// OpenPicker is a keyboard-navigable overlay listing a message's openable targets
// (media and links). It is used both for the chat "open" key and the context-menu
// "Open" item when a message has more than one target.
type OpenPicker struct {
	targets  []OpenTarget
	list     *ListView
	maxLabel int // max label width in cells before truncation
}

// NewOpenPicker builds a picker for targets. maxWidth bounds the overlay width;
// labels wider than the budget are middle-truncated with a preserved tail.
func NewOpenPicker(targets []OpenTarget, maxWidth int) *OpenPicker {
	list := NewListView(true)
	list.SetSelectable(func(int) bool { return true })
	list.SetCount(len(targets))
	list.SetCursor(0)
	// Reserve room for the "N " number prefix.
	maxLabel := maxWidth - 3
	if maxLabel < 8 {
		maxLabel = 8
	}
	return &OpenPicker{targets: targets, list: list, maxLabel: maxLabel}
}

func (p *OpenPicker) Cursor() int { return p.list.Cursor() }

func (p *OpenPicker) Update(msg tea.Msg) (*OpenPicker, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	// Normalize so navigation fires on the same physical key under non-Latin
	// layouts (e.g. Russian ЙЦУКЕН), like the rest of the app.
	key := keys.NormalizeKey(kp.String())
	switch key {
	case "j", "down", "ctrl+j":
		p.list.MoveDown()
		return p, nil
	case "k", "up", "ctrl+k":
		p.list.MoveUp()
		return p, nil
	case "enter":
		return p.choose(p.list.Cursor())
	case "esc":
		return nil, func() tea.Msg { return CloseOpenPickerMsg{} }
	}
	// Digit 1..9 picks the nth target directly.
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		if idx := int(key[0] - '1'); idx < len(p.targets) {
			return p.choose(idx)
		}
	}
	return p, nil
}

func (p *OpenPicker) choose(i int) (*OpenPicker, tea.Cmd) {
	if i < 0 || i >= len(p.targets) {
		return p, nil
	}
	target := p.targets[i]
	return nil, func() tea.Msg { return OpenTargetChosenMsg{Target: target} }
}

func (p *OpenPicker) View() string {
	// The leading number is accented like a status-bar hint key; the selected row
	// stays plain so its highlight background keeps full contrast.
	base := lipgloss.NewStyle().Background(OverlayMenuBg)
	accent := lipgloss.NewStyle().Background(OverlayMenuBg).Foreground(lipgloss.Color("39"))
	rows := make([]string, len(p.targets))
	for i, t := range p.targets {
		label := truncateTail(t.Label, p.maxLabel)
		if i == p.list.Cursor() {
			rows[i] = fmt.Sprintf("%d %s", i+1, label)
		} else {
			rows[i] = accent.Render(fmt.Sprintf("%d", i+1)) + base.Render(" "+label)
		}
	}

	hint := OverlayHint([][2]string{{"j/k", "move"}, {"enter", "open"}, {"esc", "close"}}, OverlayMenuBg)

	innerW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r); w > innerW {
			innerW = w
		}
	}
	innerW++ // right padding
	if hintW := lipgloss.Width(" " + hint + " "); hintW+2 > innerW {
		innerW = hintW + 2
	}

	for i := range rows {
		if i == p.list.Cursor() {
			rows[i] = menuSelectedStyle.Width(innerW).Render(rows[i])
		} else {
			rows[i] = menuBgStyle.Width(innerW).Render(rows[i])
		}
	}

	outerW := innerW + 2
	outerH := len(rows) + 2
	box := RenderBox(strings.Join(rows, "\n"), "Open", "", hint, "", lipgloss.RoundedBorder(), nil, outerW, outerH)
	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = menuBgStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}

// truncateTail shortens s to at most max cells, eliding the middle and keeping a
// short tail so the end of a URL/path stays visible: "https://ex…/x.html".
func truncateTail(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	tail := 6
	if tail > max-2 {
		tail = max - 2
	}
	head := max - 1 - tail
	if head < 0 {
		head = 0
	}
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}
