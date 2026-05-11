package keys

type VimMode int

const (
	ModeNormal VimMode = iota
	ModeInsert
	ModeSearch
)

type Action string

const (
	ActionNone        Action = ""
	ActionPassthrough Action = "passthrough"
	ActionUp          Action = "up"
	ActionDown        Action = "down"
	ActionLeft        Action = "left"
	ActionRight       Action = "right"
	ActionGoTop       Action = "go_top"
	ActionGoBottom    Action = "go_bottom"
	ActionInsert      Action = "insert"
	ActionNormal      Action = "normal"
	ActionConfirm     Action = "confirm"
	ActionSearch      Action = "search"
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
	case "j":
		return ActionDown
	case "k":
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
	case "i":
		vs.Mode = ModeInsert
		return ActionInsert
	case "esc":
		return ActionNormal
	case "enter":
		return ActionConfirm
	case "/":
		return ActionSearch
	}
	return ActionNone
}
