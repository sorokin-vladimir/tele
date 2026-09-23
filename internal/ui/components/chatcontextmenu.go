package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// Chat context-menu request messages. The root model turns each into an owner
// command, addressed by chat id like every other (#278).
type ToggleUnreadRequest struct {
	ChatID int64
	Unread bool
}
type ToggleMuteRequest struct {
	ChatID int64
	Muted  bool
}
type AddToFolderRequest struct {
	ChatID   int64
	FilterID int
	Add      bool
}
type ToggleArchiveRequest struct {
	ChatID   int64
	Archived bool
}

type chatMenuState int

const (
	chatStateMain chatMenuState = iota
	chatStateFolderSub
)

// ChatContextMenu is the keyboard-navigable menu shown over a chat-list
// row. It reuses the menu styles and box rendering from the message
// context menu but carries chat-specific actions.
type ChatContextMenu struct {
	chat      project.ChatRow
	folders   []domain.FolderFilter
	items     []menuItem
	savedMain []menuItem
	list      *ListView
	state     chatMenuState
	keyMap    keys.KeyMap
}

// NewChatContextMenu builds the menu over the row under the cursor. The row
// carries everything the main items need, so the menu opens at once; folders
// come from an owner query and arrive through SetFolders (#278).
func NewChatContextMenu(chat project.ChatRow, folders []domain.FolderFilter, km keys.KeyMap) *ChatContextMenu {
	cm := &ChatContextMenu{chat: chat, folders: folders, keyMap: km, list: NewListView(true)}
	cm.setItems(cm.mainItems())
	return cm
}

// ChatID is the chat the menu was opened over, so an answer that arrives after
// the menu moved on can be told from one meant for it.
func (cm *ChatContextMenu) ChatID() int64 { return cm.chat.ID }

// SetFolders installs the account's folders once the owner has answered. The
// main items gain "Add to folder" when there are any; the cursor stays on the
// item it was on rather than on whatever slid under it.
func (cm *ChatContextMenu) SetFolders(folders []domain.FolderFilter) {
	cm.folders = folders
	if cm.state != chatStateMain {
		return
	}
	current := cm.items[cm.list.Cursor()].action
	cm.setItems(cm.mainItems())
	for i, it := range cm.items {
		if it.action == current {
			cm.list.SetCursor(i)
			break
		}
	}
}

// setItems swaps the menu items and re-seeds the list: separator rows are
// skipped (folder rows carry ActionNone but stay navigable) and the cursor
// resets to the first selectable row.
func (cm *ChatContextMenu) setItems(items []menuItem) {
	cm.items = items
	cm.list.SetSelectable(func(i int) bool { return !items[i].separator })
	cm.list.SetCount(len(items))
	cm.list.SetCursor(0)
}

func (cm *ChatContextMenu) Cursor() int { return cm.list.Cursor() }

func (cm *ChatContextMenu) mainItems() []menuItem {
	var items []menuItem
	if cm.chat.Unread > 0 || cm.chat.UnreadMark {
		items = append(items, menuItem{label: "Mark as read", action: keys.ActionMarkRead})
	} else {
		items = append(items, menuItem{label: "Mark as unread", action: keys.ActionMarkUnread})
	}
	if cm.chat.Muted {
		items = append(items, menuItem{label: "Unmute", action: keys.ActionUnmute})
	} else {
		items = append(items, menuItem{label: "Mute", action: keys.ActionMute})
	}
	if len(cm.folders) > 0 {
		items = append(items, menuItem{label: "Add to folder", action: keys.ActionAddToFolder})
	}
	if cm.chat.Archived {
		items = append(items, menuItem{label: "Unarchive", action: keys.ActionUnarchive})
	} else {
		items = append(items, menuItem{label: "Archive", action: keys.ActionArchive})
	}
	// A profile is a fact about a person, so the entry is there for a private
	// chat and absent for a group or a channel (#222).
	if cm.chat.IsUser {
		items = append(items, menuItem{label: "Profile", action: keys.ActionShowProfile})
	}
	return items
}

func (cm *ChatContextMenu) folderSubItems() []menuItem {
	items := make([]menuItem, 0, len(cm.folders)+2)
	for _, f := range cm.folders {
		mark := "  "
		if folderContains(f, cm.chat.ID) {
			mark = "✓ "
		}
		items = append(items, menuItem{label: mark + f.Title, action: keys.ActionNone, isFolder: true, filterID: f.ID})
	}
	items = append(items, menuItem{label: "─────────", action: keys.ActionNone, separator: true})
	items = append(items, menuItem{label: "Cancel", action: keys.ActionCancel})
	return items
}

func folderContains(f domain.FolderFilter, chatID int64) bool {
	for _, id := range f.IncludePeers {
		if id == chatID {
			return true
		}
	}
	return false
}

func (cm *ChatContextMenu) activeContext() keys.Context {
	if cm.state == chatStateFolderSub {
		return keys.ContextFolderSubMenu
	}
	return keys.ContextChatMenu
}

func (cm *ChatContextMenu) moveDown() { cm.list.MoveDown() }
func (cm *ChatContextMenu) moveUp()   { cm.list.MoveUp() }

func (cm *ChatContextMenu) Update(msg tea.Msg) (*ChatContextMenu, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return cm, nil
	}
	action := cm.keyMap.Resolve(cm.activeContext(), kp.String())
	switch action {
	case keys.ActionDown:
		cm.moveDown()
		return cm, nil
	case keys.ActionUp:
		cm.moveUp()
		return cm, nil
	case keys.ActionCancel:
		if cm.state == chatStateFolderSub {
			cm.setItems(cm.savedMain)
			cm.state = chatStateMain
			return cm, nil
		}
		return nil, func() tea.Msg { return CloseContextMenuMsg{} }
	case keys.ActionConfirm:
		return cm.execute()
	}
	if action != keys.ActionNone {
		for i, it := range cm.items {
			if it.action == action && !it.separator {
				cm.list.SetCursor(i)
				return cm.execute()
			}
		}
	}
	return cm, nil
}

func (cm *ChatContextMenu) execute() (*ChatContextMenu, tea.Cmd) {
	item := cm.items[cm.list.Cursor()]
	chatID := cm.chat.ID

	if item.isFolder {
		add := true
		for _, f := range cm.folders {
			if f.ID == item.filterID {
				add = !folderContains(f, cm.chat.ID)
				break
			}
		}
		filterID := item.filterID
		return nil, func() tea.Msg {
			return AddToFolderRequest{ChatID: chatID, FilterID: filterID, Add: add}
		}
	}

	switch item.action {
	case keys.ActionMarkRead:
		return nil, func() tea.Msg { return ToggleUnreadRequest{ChatID: chatID, Unread: false} }
	case keys.ActionMarkUnread:
		return nil, func() tea.Msg { return ToggleUnreadRequest{ChatID: chatID, Unread: true} }
	case keys.ActionMute:
		return nil, func() tea.Msg { return ToggleMuteRequest{ChatID: chatID, Muted: true} }
	case keys.ActionUnmute:
		return nil, func() tea.Msg { return ToggleMuteRequest{ChatID: chatID, Muted: false} }
	case keys.ActionAddToFolder:
		cm.savedMain = cm.items
		cm.setItems(cm.folderSubItems())
		cm.state = chatStateFolderSub
		return cm, nil
	case keys.ActionArchive:
		return nil, func() tea.Msg { return ToggleArchiveRequest{ChatID: chatID, Archived: true} }
	case keys.ActionUnarchive:
		return nil, func() tea.Msg { return ToggleArchiveRequest{ChatID: chatID, Archived: false} }
	case keys.ActionShowProfile:
		return nil, func() tea.Msg { return OpenProfileRequest{UserID: chatID} }
	case keys.ActionCancel:
		return nil, func() tea.Msg { return CloseContextMenuMsg{} }
	}
	return cm, nil
}

func (cm *ChatContextMenu) View() string {
	b := lipgloss.RoundedBorder()
	ctx := cm.activeContext()

	rows := make([]string, len(cm.items))
	for i, item := range cm.items {
		if item.separator {
			rows[i] = "  " + item.label
			continue
		}
		k := cm.keyMap.KeyFor(ctx, item.action)
		label := item.label
		if k != "" {
			label = k + " -> " + item.label
		}
		rows[i] = "  " + label
	}

	down := cm.keyMap.KeyFor(ctx, keys.ActionDown)
	up := cm.keyMap.KeyFor(ctx, keys.ActionUp)
	confirm := cm.keyMap.KeyFor(ctx, keys.ActionConfirm)
	cancel := cm.keyMap.KeyFor(ctx, keys.ActionCancel)
	hint := OverlayHint([][2]string{
		{down + "/" + up, DescribeShort(ctx, keys.ActionDown)},
		{confirm, DescribeShort(ctx, keys.ActionConfirm)},
		{cancel, DescribeShort(ctx, keys.ActionCancel)},
	}, OverlayMenuBg())

	innerW := 0
	for _, r := range rows {
		if w := lipgloss.Width(r); w > innerW {
			innerW = w
		}
	}
	innerW++
	if hintW := lipgloss.Width(" " + hint + " "); hintW+2 > innerW {
		innerW = hintW + 2
	}

	for i := range rows {
		if i == cm.list.Cursor() && !cm.items[i].separator {
			rows[i] = theme.S().MenuSelected.Width(innerW).Render(rows[i])
		} else {
			rows[i] = theme.S().MenuBg.Width(innerW).Render(rows[i])
		}
	}

	outerW := innerW + 2
	outerH := len(rows) + 2
	box := RenderBox(strings.Join(rows, "\n"), "", "", hint, "", b, nil, outerW, outerH)

	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = theme.S().MenuBg.Render(l)
	}
	return strings.Join(lines, "\n")
}
