package keys

type Context string

const (
	ContextGlobal        Context = "global"
	ContextFolders       Context = "folders"
	ContextChatList      Context = "chatlist"
	ContextChat          Context = "chat"
	ContextComposer      Context = "composer"
	ContextSearch        Context = "search"
	ContextContextMenu   Context = "context_menu"
	ContextDeleteSubMenu Context = "delete_submenu"
	ContextChatMenu      Context = "chat_menu"
	ContextFolderSubMenu Context = "folder_submenu"
)

const (
	ActionFocusChatList Action = "focus_chatlist"
	ActionFocusChat     Action = "focus_chat"
	ActionFocusFolders  Action = "focus_folders"
	ActionFocusPrev     Action = "focus_prev"
	ActionFocusNext     Action = "focus_next"
	ActionQuit          Action = "quit"
)

// ActionMsg wraps an Action as a bubbletea message.
type ActionMsg struct{ Action Action }

// KeyMap maps (context, key) → Action.
type KeyMap map[Context]map[string]Action

func DefaultKeyMap() KeyMap {
	return KeyMap{
		ContextGlobal: {
			"0":      ActionFocusFolders,
			"1":      ActionFocusChatList,
			"2":      ActionFocusChat,
			"h":      ActionFocusPrev,
			"l":      ActionFocusNext,
			"left":   ActionFocusPrev,
			"right":  ActionFocusNext,
			"ctrl+c": ActionQuit,
			"ctrl+q": ActionQuit,
			"q":      ActionQuit,
		},
		ContextFolders: {
			"j":     ActionDown,
			"k":     ActionUp,
			"down":  ActionDown,
			"up":    ActionUp,
			"enter": ActionConfirm,
		},
		ContextChatList: {
			"j":      ActionDown,
			"k":      ActionUp,
			"down":   ActionDown,
			"up":     ActionUp,
			"G":      ActionGoBottom,
			"g g":    ActionGoTop,
			"enter":  ActionConfirm,
			"/":      ActionSearch,
			"ctrl+d": ActionScrollHalfDown,
			"ctrl+u": ActionScrollHalfUp,
			"space":  ActionOpenContextMenu,
		},
		// ContextChat is the live source for chat-pane keys, resolved through
		// the Matcher. "g g" is a chord (space-separated key tokens).
		ContextChat: {
			"j":      ActionDown,
			"k":      ActionUp,
			"down":   ActionDown,
			"up":     ActionUp,
			"G":      ActionGoBottom,
			"g g":    ActionGoTop,
			"ctrl+d": ActionScrollHalfDown,
			"ctrl+u": ActionScrollHalfUp,
			"i":      ActionInsert,
			"a":      ActionInsert,
			"esc":    ActionNormal,
			"enter":  ActionConfirm,
			"/":      ActionSearch,
			"space":  ActionOpenContextMenu,
			"r":      ActionReply,
			"e":      ActionEdit,
			"o":      ActionOpenInViewer,
			"p":      ActionPlayVoice,
		},
		ContextComposer: {
			"enter": ActionConfirm,
			"esc":   ActionNormal,
		},
		ContextContextMenu: {
			"j":     ActionDown,
			"down":  ActionDown,
			"k":     ActionUp,
			"up":    ActionUp,
			"enter": ActionConfirm,
			"space": ActionCancel,
			"esc":   ActionCancel,
			"r":     ActionReply,
			"t":     ActionReact,
			"e":     ActionEdit,
			"d":     ActionDelete,
			"g":     ActionJumpToOriginal,
			"o":     ActionOpenInViewer,
			"p":     ActionPlayVoice,
		},
		ContextDeleteSubMenu: {
			"j":     ActionDown,
			"down":  ActionDown,
			"k":     ActionUp,
			"up":    ActionUp,
			"enter": ActionConfirm,
			"esc":   ActionCancel,
			"a":     ActionDeleteRevoke,
			"m":     ActionDeleteMe,
		},
		ContextChatMenu: {
			"j":     ActionDown,
			"down":  ActionDown,
			"k":     ActionUp,
			"up":    ActionUp,
			"enter": ActionConfirm,
			"space": ActionCancel,
			"esc":   ActionCancel,
			"r":     ActionMarkRead,
			"u":     ActionMarkUnread,
			"m":     ActionMute,
			"f":     ActionAddToFolder,
			"a":     ActionArchive,
		},
		ContextFolderSubMenu: {
			"j":     ActionDown,
			"down":  ActionDown,
			"k":     ActionUp,
			"up":    ActionUp,
			"enter": ActionConfirm,
			"esc":   ActionCancel,
		},
		ContextSearch: {
			"esc":    ActionCancel,
			"enter":  ActionConfirm,
			"down":   ActionDown,
			"ctrl+j": ActionDown,
			"up":     ActionUp,
			"ctrl+k": ActionUp,
		},
	}
}
