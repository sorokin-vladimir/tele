package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// Chat context-menu request messages. The root model turns each into an
// optimistic store mutation plus an async Telegram RPC.
type ToggleUnreadRequest struct {
	Peer   store.Peer
	Unread bool
}
type ToggleMuteRequest struct {
	Peer  store.Peer
	Muted bool
}
type AddToFolderRequest struct {
	Peer     store.Peer
	FilterID int
	Add      bool
}
type ToggleArchiveRequest struct {
	Peer     store.Peer
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
	chat      store.Chat
	folders   []store.FolderFilter
	items     []menuItem
	savedMain []menuItem
	cursor    int
	state     chatMenuState
	keyMap    keys.KeyMap
}

func NewChatContextMenu(chat store.Chat, folders []store.FolderFilter, km keys.KeyMap) *ChatContextMenu {
	cm := &ChatContextMenu{chat: chat, folders: folders, keyMap: km}
	cm.items = cm.mainItems()
	return cm
}

func (cm *ChatContextMenu) Cursor() int { return cm.cursor }

func (cm *ChatContextMenu) mainItems() []menuItem {
	var items []menuItem
	if cm.chat.UnreadCount > 0 || cm.chat.UnreadMark {
		items = append(items, menuItem{label: "Mark as read", action: keys.ActionMarkRead})
	} else {
		items = append(items, menuItem{label: "Mark as unread", action: keys.ActionMarkUnread})
	}
	if cm.chat.IsMuted {
		items = append(items, menuItem{label: "Unmute", action: keys.ActionUnmute})
	} else {
		items = append(items, menuItem{label: "Mute", action: keys.ActionMute})
	}
	if len(cm.folders) > 0 {
		items = append(items, menuItem{label: "Add to folder", action: keys.ActionAddToFolder})
	}
	if cm.chat.IsArchived {
		items = append(items, menuItem{label: "Unarchive", action: keys.ActionUnarchive})
	} else {
		items = append(items, menuItem{label: "Archive", action: keys.ActionArchive})
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

func folderContains(f store.FolderFilter, chatID int64) bool {
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

func (cm *ChatContextMenu) moveDown() { cm.move(1) }
func (cm *ChatContextMenu) moveUp()   { cm.move(-1) }

func (cm *ChatContextMenu) move(dir int) {
	n := len(cm.items)
	for i := 1; i < n; i++ {
		next := ((cm.cursor+dir*i)%n + n) % n
		if !cm.items[next].separator {
			cm.cursor = next
			return
		}
	}
}

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
			cm.items = cm.savedMain
			cm.cursor = 0
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
				cm.cursor = i
				return cm.execute()
			}
		}
	}
	return cm, nil
}

func (cm *ChatContextMenu) execute() (*ChatContextMenu, tea.Cmd) {
	item := cm.items[cm.cursor]
	peer := cm.chat.Peer

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
			return AddToFolderRequest{Peer: peer, FilterID: filterID, Add: add}
		}
	}

	switch item.action {
	case keys.ActionMarkRead:
		return nil, func() tea.Msg { return ToggleUnreadRequest{Peer: peer, Unread: false} }
	case keys.ActionMarkUnread:
		return nil, func() tea.Msg { return ToggleUnreadRequest{Peer: peer, Unread: true} }
	case keys.ActionMute:
		return nil, func() tea.Msg { return ToggleMuteRequest{Peer: peer, Muted: true} }
	case keys.ActionUnmute:
		return nil, func() tea.Msg { return ToggleMuteRequest{Peer: peer, Muted: false} }
	case keys.ActionAddToFolder:
		cm.savedMain = cm.items
		cm.items = cm.folderSubItems()
		cm.cursor = 0
		cm.state = chatStateFolderSub
		return cm, nil
	case keys.ActionArchive:
		return nil, func() tea.Msg { return ToggleArchiveRequest{Peer: peer, Archived: true} }
	case keys.ActionUnarchive:
		return nil, func() tea.Msg { return ToggleArchiveRequest{Peer: peer, Archived: false} }
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
	hint := strings.Join([]string{down + "/" + up, confirm, cancel}, " | ")

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
		if i == cm.cursor && !cm.items[i].separator {
			rows[i] = menuSelectedStyle.Width(innerW).Render(rows[i])
		} else {
			rows[i] = menuBgStyle.Width(innerW).Render(rows[i])
		}
	}

	outerW := innerW + 2
	outerH := len(rows) + 2
	box := RenderBox(strings.Join(rows, "\n"), "", "", hint, b, nil, outerW, outerH)

	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = menuBgStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}
