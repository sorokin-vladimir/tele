package keys

// Label is the human-readable text for an action. Short is the terse footer
// label ("scroll"); Long is the fuller help-modal description. Long defaults to
// Short when empty in the table.
type Label struct {
	Short string
	Long  string
}

// Describe returns the Label for action in ctx. It applies a per-context
// override when present, otherwise the shared default. The bool is false when
// neither defines the action. Long is filled from Short when the table leaves
// it empty.
func Describe(ctx Context, action Action) (Label, bool) {
	if overrides, ok := contextLabels[ctx]; ok {
		if lbl, ok := overrides[action]; ok {
			return withLong(lbl), true
		}
	}
	if lbl, ok := defaultLabels[action]; ok {
		return withLong(lbl), true
	}
	return Label{}, false
}

func withLong(l Label) Label {
	if l.Long == "" {
		l.Long = l.Short
	}
	return l
}

// defaultLabels holds the wording shared across contexts. Per-context wording
// that differs lives in contextLabels and wins over these.
var defaultLabels = map[Action]Label{
	// Navigation / scrolling.
	ActionUp:             {Short: "up"},
	ActionDown:           {Short: "down"},
	ActionCursorUp:       {Short: "select", Long: "select message"},
	ActionCursorDown:     {Short: "select", Long: "select message"},
	ActionGoTop:          {Short: "top", Long: "go to top"},
	ActionGoBottom:       {Short: "bottom", Long: "go to bottom"},
	ActionScrollHalfUp:   {Short: "half up", Long: "half page up"},
	ActionScrollHalfDown: {Short: "half down", Long: "half page down"},
	// Mode / confirm / cancel.
	ActionInsert:  {Short: "write"},
	ActionNormal:  {Short: "normal", Long: "normal mode"},
	ActionConfirm: {Short: "select"},
	ActionCancel:  {Short: "close"},
	// Pane focus / app (global).
	ActionFocusFolders:  {Short: "folders", Long: "focus folders"},
	ActionFocusChatList: {Short: "chats", Long: "focus chat list"},
	ActionFocusChat:     {Short: "chat", Long: "focus chat"},
	ActionFocusPrev:     {Short: "prev pane"},
	ActionFocusNext:     {Short: "next pane"},
	ActionQuit:          {Short: "quit"},
	ActionDismissToast:  {Short: "dismiss", Long: "dismiss toast"},
	ActionShowHelp:      {Short: "help", Long: "keyboard shortcuts"},
	// Chat / message actions.
	ActionSearch:          {Short: "search"},
	ActionOpenContextMenu: {Short: "menu"},
	ActionReply:           {Short: "reply"},
	ActionReact:           {Short: "react"},
	ActionEdit:            {Short: "edit"},
	ActionForward:         {Short: "forward"},
	ActionDelete:          {Short: "delete"},
	ActionDeleteRevoke:    {Short: "for everyone", Long: "delete for everyone"},
	ActionDeleteMe:        {Short: "for me", Long: "delete for me"},
	ActionJumpToOriginal:  {Short: "jump", Long: "jump to original"},
	ActionOpenInViewer:    {Short: "open"},
	ActionOpenExternal:    {Short: "open ext", Long: "open externally"},
	ActionPlayVoice:       {Short: "play"},
	ActionDownloadFile:    {Short: "download"},
	ActionCopyMessage:     {Short: "copy"},
	ActionAttach:          {Short: "upload"},
	ActionToggleSendAs:    {Short: "photo/file"},
	ActionCancelUpload:    {Short: "drop file"},
	ActionPasteImage:      {Short: "paste image", Long: "paste image from clipboard as photo"},
	// Chat-menu actions.
	ActionMarkRead:    {Short: "read", Long: "mark read"},
	ActionMarkUnread:  {Short: "unread", Long: "mark unread"},
	ActionMute:        {Short: "mute"},
	ActionUnmute:      {Short: "unmute"},
	ActionArchive:     {Short: "archive"},
	ActionUnarchive:   {Short: "unarchive"},
	ActionAddToFolder: {Short: "folder", Long: "add to folder"},
}

// contextLabels holds per-context overrides. down carries the navigation-pair
// wording for its context ("move" in lists, "scroll" in chat); confirm carries
// the context-appropriate verb ("open"/"send"/"open/select").
var contextLabels = map[Context]map[Action]Label{
	ContextFolders: {
		ActionDown: {Short: "move"},
	},
	ContextChatList: {
		ActionDown:    {Short: "move"},
		ActionConfirm: {Short: "open"},
	},
	ContextChat: {
		ActionDown:    {Short: "scroll"},
		ActionConfirm: {Short: "open"},
	},
	ContextComposer: {
		ActionConfirm: {Short: "send"},
	},
	ContextSearch: {
		ActionDown:    {Short: "move"},
		ActionConfirm: {Short: "open"},
	},
	ContextFilePicker: {
		ActionDown:    {Short: "move"},
		ActionConfirm: {Short: "open/select"},
	},
	ContextContextMenu: {
		ActionDown: {Short: "move"},
	},
	ContextDeleteSubMenu: {
		ActionDown: {Short: "move"},
	},
	ContextChatMenu: {
		ActionDown: {Short: "move"},
	},
	ContextFolderSubMenu: {
		ActionDown: {Short: "move"},
	},
}
