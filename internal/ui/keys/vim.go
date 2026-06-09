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
	ActionOpenInViewer    Action = "open_in_viewer"
	ActionOpenContextMenu Action = "open_context_menu"
	ActionCancel          Action = "cancel"
	ActionReply           Action = "reply"
	ActionReact           Action = "react"
	ActionEdit            Action = "edit"
	ActionDelete          Action = "delete"
	ActionDeleteRevoke    Action = "delete_revoke"
	ActionDeleteMe        Action = "delete_me"
	ActionJumpToOriginal  Action = "jump_to_original"
	ActionPlayVoice       Action = "play_voice"
	ActionMarkRead        Action = "mark_read"
	ActionMarkUnread      Action = "mark_unread"
	ActionMute            Action = "mute"
	ActionUnmute          Action = "unmute"
	ActionAddToFolder     Action = "add_to_folder"
	ActionArchive         Action = "archive"
	ActionUnarchive       Action = "unarchive"
)

type VimState struct {
	Mode VimMode
}

func NewVimState() *VimState {
	return &VimState{Mode: ModeNormal}
}
