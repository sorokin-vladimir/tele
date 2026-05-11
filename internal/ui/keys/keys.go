package keys

type Context string

const (
	ContextGlobal   Context = "global"
	ContextChatList Context = "chatlist"
	ContextChat     Context = "chat"
	ContextComposer Context = "composer"
	ContextSearch   Context = "search"
)

const (
	ActionSwitchFocus Action = "switch_focus"
	ActionQuit        Action = "quit"
)

// ActionMsg wraps an Action as a bubbletea message.
type ActionMsg struct{ Action Action }

// KeyMap maps (context, key) → Action.
type KeyMap map[Context]map[string]Action

func DefaultKeyMap() KeyMap {
	return KeyMap{
		ContextGlobal: {
			"tab":    ActionSwitchFocus,
			"ctrl+c": ActionQuit,
			"ctrl+q": ActionQuit,
		},
		ContextChatList: {
			"j":     ActionDown,
			"k":     ActionUp,
			"G":     ActionGoBottom,
			"enter": ActionConfirm,
			"/":     ActionSearch,
		},
		ContextChat: {
			"j":   ActionDown,
			"k":   ActionUp,
			"G":   ActionGoBottom,
			"i":   ActionInsert,
			"esc": ActionNormal,
		},
	}
}
