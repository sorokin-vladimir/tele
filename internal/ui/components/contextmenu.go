package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	lipcompat "charm.land/lipgloss/v2/compat"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// CloseContextMenuMsg is emitted when the context menu closes without an action.
type CloseContextMenuMsg struct{}

// DeleteMsgRequest is emitted when the user confirms deletion.
type DeleteMsgRequest struct {
	MsgID  int
	Revoke bool
}

type menuState int

const (
	stateMain menuState = iota
	stateDeleteSub
)

type menuItem struct {
	label  string
	action keys.Action
}

var (
	menuBgStyle = lipgloss.NewStyle().
			Background(lipcompat.AdaptiveColor{
			Light: lipgloss.Color("252"),
			Dark:  lipgloss.Color("235"),
		})

	menuSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("63")).
				Foreground(lipgloss.Color("0"))
)

// ContextMenu is a keyboard-navigable context menu overlaid on the chat view.
type ContextMenu struct {
	items  []menuItem
	cursor int
	state  menuState
	msgID  int
	isOut  bool
	keyMap keys.KeyMap
}

func NewContextMenu(msgID int, isOut bool, km keys.KeyMap) *ContextMenu {
	return &ContextMenu{
		items:  mainItems(isOut),
		msgID:  msgID,
		isOut:  isOut,
		keyMap: km,
	}
}

func (cm *ContextMenu) Cursor() int { return cm.cursor }

func mainItems(isOut bool) []menuItem {
	items := []menuItem{
		{label: "Reply", action: keys.ActionReply},
		{label: "React", action: keys.ActionReact},
	}
	if isOut {
		items = append(items, menuItem{label: "Edit", action: keys.ActionEdit})
	}
	items = append(items, menuItem{label: "Delete", action: keys.ActionDelete})
	return items
}

func deleteSubItems() []menuItem {
	return []menuItem{
		{label: "For everyone", action: keys.ActionDeleteRevoke},
		{label: "For me", action: keys.ActionDeleteMe},
		{label: "─────────", action: keys.ActionNone}, // separator
		{label: "Cancel", action: keys.ActionCancel},
	}
}

func (cm *ContextMenu) activeContext() keys.Context {
	if cm.state == stateDeleteSub {
		return keys.ContextDeleteSubMenu
	}
	return keys.ContextContextMenu
}

func (cm *ContextMenu) moveDown() {
	n := len(cm.items)
	for i := 1; i < n; i++ {
		next := (cm.cursor + i) % n
		if cm.items[next].action != keys.ActionNone {
			cm.cursor = next
			return
		}
	}
}

func (cm *ContextMenu) moveUp() {
	n := len(cm.items)
	for i := 1; i < n; i++ {
		prev := (cm.cursor - i + n) % n
		if cm.items[prev].action != keys.ActionNone {
			cm.cursor = prev
			return
		}
	}
}

func (cm *ContextMenu) Update(msg tea.Msg) (*ContextMenu, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return cm, nil
	}

	ctx := cm.activeContext()
	action := cm.keyMap.Resolve(ctx, kp.String())

	switch action {
	case keys.ActionDown:
		cm.moveDown()
		return cm, nil
	case keys.ActionUp:
		cm.moveUp()
		return cm, nil
	case keys.ActionCancel:
		if cm.state == stateDeleteSub {
			cm.state = stateMain
			cm.items = mainItems(cm.isOut)
			cm.cursor = 0
			return cm, nil
		}
		return nil, func() tea.Msg { return CloseContextMenuMsg{} }
	case keys.ActionConfirm:
		return cm.execute()
	}

	// direct item key: find the item whose action matches and execute
	if action != keys.ActionNone {
		for i, item := range cm.items {
			if item.action == action {
				cm.cursor = i
				return cm.execute()
			}
		}
	}

	return cm, nil
}

func (cm *ContextMenu) execute() (*ContextMenu, tea.Cmd) {
	action := cm.items[cm.cursor].action
	switch action {
	case keys.ActionReply, keys.ActionReact, keys.ActionEdit, keys.ActionCancel:
		return nil, func() tea.Msg { return CloseContextMenuMsg{} }
	case keys.ActionDelete:
		if !cm.isOut {
			msgID := cm.msgID
			return nil, func() tea.Msg { return DeleteMsgRequest{MsgID: msgID, Revoke: false} }
		}
		cm.state = stateDeleteSub
		cm.items = deleteSubItems()
		cm.cursor = 0
		return cm, nil
	case keys.ActionDeleteMe:
		msgID := cm.msgID
		return nil, func() tea.Msg { return DeleteMsgRequest{MsgID: msgID, Revoke: false} }
	case keys.ActionDeleteRevoke:
		msgID := cm.msgID
		return nil, func() tea.Msg { return DeleteMsgRequest{MsgID: msgID, Revoke: true} }
	}
	return cm, nil
}

func (cm *ContextMenu) View() string {
	b := lipgloss.RoundedBorder()
	ctx := cm.activeContext()

	rows := make([]string, len(cm.items))
	for i, item := range cm.items {
		if item.action == keys.ActionNone {
			rows[i] = "  " + item.label
			continue
		}
		k := cm.keyMap.KeyFor(ctx, item.action)
		var label string
		if k != "" {
			label = k + " -> " + item.label
		} else {
			label = item.label
		}
		rows[i] = "  " + label
	}

	// build bottom nav hint
	down := cm.keyMap.KeyFor(ctx, keys.ActionDown)
	up := cm.keyMap.KeyFor(ctx, keys.ActionUp)
	confirm := cm.keyMap.KeyFor(ctx, keys.ActionConfirm)
	cancel := cm.keyMap.KeyFor(ctx, keys.ActionCancel)
	hint := strings.Join([]string{down + "/" + up, confirm, cancel}, " | ")

	// compute inner width: max of content width+padding and hint minimum
	innerW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r); w > innerW {
			innerW = w
		}
	}
	innerW++ // right padding
	// RenderBox needs fillW>=2, so innerW must be >= hintW+2 (one border char each side)
	if hintW := lipgloss.Width(" " + hint + " "); hintW+2 > innerW {
		innerW = hintW + 2
	}

	// apply per-row backgrounds (selected vs normal)
	for i := range rows {
		if i == cm.cursor && cm.items[i].action != keys.ActionNone {
			rows[i] = menuSelectedStyle.Width(innerW).Render(rows[i])
		} else {
			rows[i] = menuBgStyle.Width(innerW).Render(rows[i])
		}
	}

	outerW := innerW + 2
	outerH := len(rows) + 2
	box := RenderBox(strings.Join(rows, "\n"), "", hint, b, outerW, outerH)

	// apply background to border rows (top, bottom) so the entire box shares the bg
	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = menuBgStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}
