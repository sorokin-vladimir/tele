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
			"j":     ActionDown,
			"k":     ActionUp,
			"down":  ActionDown,
			"up":    ActionUp,
			"G":     ActionGoBottom,
			"enter": ActionConfirm,
			"/":     ActionSearch,
		},
		ContextChat: {
			"j":   ActionDown,
			"k":   ActionUp,
			"G":   ActionGoBottom,
			"i":   ActionInsert,
			"a":   ActionInsert,
			"esc": ActionNormal,
		},
	}
}
