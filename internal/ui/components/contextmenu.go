package components

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	lipcompat "charm.land/lipgloss/v2/compat"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// CloseContextMenuMsg is emitted when the context menu closes without an action.
type CloseContextMenuMsg struct{}

// DeleteMsgRequest is emitted when the user confirms deletion.
type DeleteMsgRequest struct {
	MsgID  int
	Revoke bool
}

// JumpToMsgRequest is emitted when the user selects "Jump to original".
type JumpToMsgRequest struct {
	MsgID int
}

// ReplyMsgRequest is emitted when the user activates reply for a message.
type ReplyMsgRequest struct {
	MsgID int
}

// ForwardMsgRequest is emitted when the user activates forward for a message.
type ForwardMsgRequest struct {
	MsgID int
}

// ReactMsgRequest is emitted when the user opens the reaction picker for a message.
type ReactMsgRequest struct {
	MsgID int
}

// EditMsgRequest is emitted when the user activates edit for a message.
type EditMsgRequest struct {
	MsgID int
}

// OpenInViewerRequest is emitted when the user selects "Open in app" for a
// media message (the in-app modal).
type OpenInViewerRequest struct{}

// OpenExternalRequest is emitted when the user selects "Open externally" for a
// photo or video message.
type OpenExternalRequest struct{}

// PlayVoiceRequest is emitted when the user selects "Play" for a voice message.
type PlayVoiceRequest struct{}

// DownloadFileRequest is emitted when the user selects "Download" for a generic
// file message.
type DownloadFileRequest struct{}

type menuState int

const (
	stateMain menuState = iota
	stateDeleteSub
)

type menuItem struct {
	label  string
	action keys.Action
	// Chat-menu-only fields (ignored by the message menu).
	separator bool // non-navigable divider row
	isFolder  bool // folder-picker entry in the add-to-folder submenu
	filterID  int  // folder id for isFolder entries
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
	items        []menuItem
	list         *ListView
	state        menuState
	msgID        int
	isOut        bool
	replyToMsgID int
	mediaKind    store.MediaKind
	hasMedia     bool
	keyMap       keys.KeyMap
}

// NewContextMenu builds the chat message context menu. mediaKind is the kind of
// the selected message's media and hasMedia reports whether the message carries
// any media (when false, mediaKind is ignored and no media actions are shown).
func NewContextMenu(msgID int, isOut bool, replyToMsgID int, mediaKind store.MediaKind, hasMedia bool, km keys.KeyMap) *ContextMenu {
	cm := &ContextMenu{
		msgID:        msgID,
		isOut:        isOut,
		replyToMsgID: replyToMsgID,
		mediaKind:    mediaKind,
		hasMedia:     hasMedia,
		keyMap:       km,
		list:         NewListView(true),
	}
	cm.setItems(mainItems(isOut, replyToMsgID != 0, mediaKind, hasMedia))
	return cm
}

// setItems swaps the menu items and re-seeds the list: non-navigable rows
// (ActionNone separators) are skipped and the cursor resets to the first
// selectable row.
func (cm *ContextMenu) setItems(items []menuItem) {
	cm.items = items
	cm.list.SetSelectable(func(i int) bool { return items[i].action != keys.ActionNone })
	cm.list.SetCount(len(items))
	cm.list.SetCursor(0)
}

func (cm *ContextMenu) Cursor() int { return cm.list.Cursor() }

func mainItems(isOut bool, isReply bool, mediaKind store.MediaKind, hasMedia bool) []menuItem {
	var items []menuItem
	if isReply {
		items = append(items, menuItem{label: "Jump to original", action: keys.ActionJumpToOriginal})
	}
	items = append(items,
		menuItem{label: "Reply", action: keys.ActionReply},
		menuItem{label: "React", action: keys.ActionReact},
		menuItem{label: "Forward", action: keys.ActionForward},
	)
	if isOut {
		items = append(items, menuItem{label: "Edit", action: keys.ActionEdit})
	}
	if hasMedia {
		items = append(items, mediaItems(mediaKind)...)
	}
	items = append(items, menuItem{label: "Delete", action: keys.ActionDelete})
	return items
}

// mediaItems returns the media actions for a message of the given kind:
// download is offered for everything downloadable; external open for photo and
// video; the in-app modal for video; in-app playback for voice. Stickers and
// non-file media (location, etc.) get no media actions.
func mediaItems(kind store.MediaKind) []menuItem {
	switch kind {
	case store.MediaPhoto:
		return []menuItem{
			{label: "Open externally", action: keys.ActionOpenExternal},
			{label: "Download", action: keys.ActionDownloadFile},
		}
	case store.MediaVideo, store.MediaVideoNote:
		return []menuItem{
			{label: "Open in app", action: keys.ActionOpenInViewer},
			{label: "Open externally", action: keys.ActionOpenExternal},
			{label: "Download", action: keys.ActionDownloadFile},
		}
	case store.MediaVoice:
		return []menuItem{
			{label: "Play", action: keys.ActionPlayVoice},
			{label: "Download", action: keys.ActionDownloadFile},
		}
	case store.MediaAudio, store.MediaGIF, store.MediaFile:
		return []menuItem{
			{label: "Download", action: keys.ActionDownloadFile},
		}
	default:
		return nil
	}
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

func (cm *ContextMenu) moveDown() { cm.list.MoveDown() }
func (cm *ContextMenu) moveUp()   { cm.list.MoveUp() }

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
			cm.setItems(mainItems(cm.isOut, cm.replyToMsgID != 0, cm.mediaKind, cm.hasMedia))
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
				cm.list.SetCursor(i)
				return cm.execute()
			}
		}
	}

	return cm, nil
}

func (cm *ContextMenu) execute() (*ContextMenu, tea.Cmd) {
	action := cm.items[cm.list.Cursor()].action
	switch action {
	case keys.ActionJumpToOriginal:
		replyToMsgID := cm.replyToMsgID
		return nil, func() tea.Msg { return JumpToMsgRequest{MsgID: replyToMsgID} }
	case keys.ActionReply:
		msgID := cm.msgID
		return nil, func() tea.Msg { return ReplyMsgRequest{MsgID: msgID} }
	case keys.ActionEdit:
		msgID := cm.msgID
		return nil, func() tea.Msg { return EditMsgRequest{MsgID: msgID} }
	case keys.ActionReact:
		msgID := cm.msgID
		return nil, func() tea.Msg { return ReactMsgRequest{MsgID: msgID} }
	case keys.ActionForward:
		msgID := cm.msgID
		return nil, func() tea.Msg { return ForwardMsgRequest{MsgID: msgID} }
	case keys.ActionCancel:
		return nil, func() tea.Msg { return CloseContextMenuMsg{} }
	case keys.ActionDelete:
		cm.state = stateDeleteSub
		cm.setItems(deleteSubItems())
		return cm, nil
	case keys.ActionDeleteMe:
		msgID := cm.msgID
		return nil, func() tea.Msg { return DeleteMsgRequest{MsgID: msgID, Revoke: false} }
	case keys.ActionDeleteRevoke:
		msgID := cm.msgID
		return nil, func() tea.Msg { return DeleteMsgRequest{MsgID: msgID, Revoke: true} }
	case keys.ActionOpenInViewer:
		return nil, func() tea.Msg { return OpenInViewerRequest{} }
	case keys.ActionOpenExternal:
		return nil, func() tea.Msg { return OpenExternalRequest{} }
	case keys.ActionDownloadFile:
		return nil, func() tea.Msg { return DownloadFileRequest{} }
	case keys.ActionPlayVoice:
		return nil, func() tea.Msg { return PlayVoiceRequest{} }
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

	// build bottom nav hint (status-bar style)
	down := cm.keyMap.KeyFor(ctx, keys.ActionDown)
	up := cm.keyMap.KeyFor(ctx, keys.ActionUp)
	confirm := cm.keyMap.KeyFor(ctx, keys.ActionConfirm)
	cancel := cm.keyMap.KeyFor(ctx, keys.ActionCancel)
	hint := OverlayHint([][2]string{{down + "/" + up, "move"}, {confirm, "select"}, {cancel, "close"}}, OverlayMenuBg)

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
		if i == cm.list.Cursor() && cm.items[i].action != keys.ActionNone {
			rows[i] = menuSelectedStyle.Width(innerW).Render(rows[i])
		} else {
			rows[i] = menuBgStyle.Width(innerW).Render(rows[i])
		}
	}

	outerW := innerW + 2
	outerH := len(rows) + 2
	box := RenderBox(strings.Join(rows, "\n"), "", "", hint, b, nil, outerW, outerH)

	// apply background to border rows (top, bottom) so the entire box shares the bg
	lines := strings.Split(box, "\n")
	for i, l := range lines {
		lines[i] = menuBgStyle.Render(l)
	}
	return strings.Join(lines, "\n")
}
