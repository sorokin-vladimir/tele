package keys

type Context string

const (
	ContextGlobal        Context = "global"
	ContextChatList      Context = "chatlist"
	ContextChat          Context = "chat"
	ContextComposer      Context = "composer"
	ContextSearch        Context = "search"
	ContextContextMenu   Context = "context_menu"
	ContextDeleteSubMenu Context = "delete_submenu"
)

const (
	ActionFocusLeft  Action = "focus_left"
	ActionFocusRight Action = "focus_right"
	ActionQuit       Action = "quit"
)

// ActionMsg wraps an Action as a bubbletea message.
type ActionMsg struct{ Action Action }

// KeyMap maps (context, key) → Action.
type KeyMap map[Context]map[string]Action

func DefaultKeyMap() KeyMap {
	return KeyMap{
		ContextGlobal: {
			"1":      ActionFocusLeft,
			"2":      ActionFocusRight,
			"h":      ActionFocusLeft,
			"l":      ActionFocusRight,
			"left":   ActionFocusLeft,
			"right":  ActionFocusRight,
			"ctrl+c": ActionQuit,
			"ctrl+q": ActionQuit,
			"q":      ActionQuit,
		},
		ContextChatList: {
			"j":      ActionDown,
			"k":      ActionUp,
			"down":   ActionDown,
			"up":     ActionUp,
			"G":      ActionGoBottom,
			"enter":  ActionConfirm,
			"/":      ActionSearch,
			"ctrl+d": ActionScrollHalfDown,
			"ctrl+u": ActionScrollHalfUp,
		},
		// ContextChat is documentation-only; at runtime, chat keys route through VimState.Process.
		ContextChat: {
			"j":     ActionDown,
			"k":     ActionUp,
			"G":     ActionGoBottom,
			"i":     ActionInsert,
			"a":     ActionInsert,
			"esc":   ActionNormal,
			"o":     ActionOpenPhoto,
			"space": ActionOpenContextMenu,
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
