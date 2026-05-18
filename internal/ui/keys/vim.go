package keys

type VimMode int

const (
	ModeNormal VimMode = iota
	ModeInsert
	ModeSearch
)

type Action string

const (
	ActionNone            Action = ""
	ActionPassthrough     Action = "passthrough"
	ActionUp              Action = "up"
	ActionDown            Action = "down"
	ActionLeft            Action = "left"
	ActionRight           Action = "right"
	ActionGoTop           Action = "go_top"
	ActionGoBottom        Action = "go_bottom"
	ActionScrollHalfDown  Action = "scroll_half_down"
	ActionScrollHalfUp    Action = "scroll_half_up"
	ActionInsert          Action = "insert"
	ActionNormal          Action = "normal"
	ActionConfirm         Action = "confirm"
	ActionSearch          Action = "search"
	ActionOpenPhoto       Action = "open_photo"
	ActionOpenContextMenu Action = "open_context_menu"
	ActionCancel          Action = "cancel"
	ActionReply           Action = "reply"
	ActionReact           Action = "react"
	ActionEdit            Action = "edit"
	ActionDelete          Action = "delete"
	ActionDeleteRevoke    Action = "delete_revoke"
	ActionDeleteMe        Action = "delete_me"
)

type VimState struct {
	Mode    VimMode
	Pending string
}

func NewVimState() *VimState {
	return &VimState{Mode: ModeNormal}
}

func (vs *VimState) Process(key string) Action {
	if vs.Mode == ModeInsert {
		if key == "esc" {
			vs.Mode = ModeNormal
			return ActionNormal
		}
		return ActionPassthrough
	}

	if vs.Pending == "g" {
		vs.Pending = ""
		if key == "g" {
			return ActionGoTop
		}
		return ActionNone
	}

	switch key {
	case "j", "down":
		return ActionDown
	case "k", "up":
		return ActionUp
	case "h":
		return ActionLeft
	case "l":
		return ActionRight
	case "G":
		return ActionGoBottom
	case "g":
		vs.Pending = "g"
		return ActionNone
	case "ctrl+d":
		return ActionScrollHalfDown
	case "ctrl+u":
		return ActionScrollHalfUp
	case "i", "a":
		vs.Mode = ModeInsert
		return ActionInsert
	case "esc":
		return ActionNormal
	case "enter":
		return ActionConfirm
	case "/":
		return ActionSearch
	case "o":
		return ActionOpenPhoto
	case "space":
		return ActionOpenContextMenu
	}
	return ActionNone
}
