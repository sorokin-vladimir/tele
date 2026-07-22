package ui

import (
	"context"
	"image"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/sorokin-vladimir/tele/internal/audio"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/mediacache"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/imagecache"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type Screen int

const (
	ScreenLogin Screen = iota
	ScreenMain
)

type Focus int

const (
	FocusFolders Focus = iota
	FocusChatList
	FocusChat
)

// borderSize is the number of characters each border side adds (1 per side = 2 total per axis).
const borderSize = 1

type RootModel struct {
	ctx               context.Context
	screen            Screen
	focus             Focus
	width             int
	height            int
	hasDarkBackground bool
	chatList          *screens.ChatListModel
	chat              *screens.ChatModel
	login             screens.LoginModel
	statusBar         *components.StatusBar
	toasts            *components.ToastStack
	vimState          *keys.VimState
	keyMap            keys.KeyMap
	matcher           *keys.Matcher
	tgClient          internaltg.Client
	st                store.Store
	currentChatID     int64
	// loadingOlderChat is the chat ID with an in-flight "load older history"
	// fetch, or 0 when none. It gates duplicate LoadMore requests: rapid
	// scroll-up would otherwise fire several identical fetches whose chunks stack
	// into a repeating date-range "ring" (issue #120).
	loadingOlderChat int64
	historyLimit     int
	verbose          bool
	cfg              *config.Config
	imageCache       *imagecache.Cache
	fullImageCache   *imagecache.Cache
	mediaCache       *mediacache.Cache
	// gifFrames caches decoded frames per document id for inline GIF looping.
	gifFrames      map[int64][]image.Image
	gifActiveID    int64 // document id currently animating (0 = none)
	gifIdx         int   // current frame index of the active animation
	gifGen         int   // bumped on every (re)start/stop to invalidate stale ticks
	gifSpinnerIdx  int   // loading-spinner glyph index for a GIF being fetched
	imageMode      media.Mode
	kittyStore     *media.KittyStore
	lastPhotoCols  int
	lastPaneHeight int
	retransmitGen  int
	// Kitty placements are a bounded terminal resource: transmitting every chat
	// image at once overruns the terminal and corrupts some. kittyLive tracks the
	// photo ids currently transmitted (or in flight); kittyLRU orders them by last
	// visible (oldest first) for eviction; kittyResetPending requests a delete-all
	// before the next reconcile (chat switch / width change).
	kittyLive         map[int64]bool
	kittyLRU          []int64
	kittyResetPending bool
	kittyCap          int // max live placements; from config, 0 → default
	searchModel       *screens.SearchModel
	onChatOpen        func(int64)
	nextSentinel      int
	contextMenu       *components.ContextMenu
	chatMenu          *components.ChatContextMenu
	reactionPicker    *components.ReactionPicker
	help              *components.HelpModal
	openPicker        *components.OpenPicker
	reactionTargetID  int
	mentionPopup      *components.MentionPopup
	mentionMembers    map[int64][]store.ChatMember
	folderBar         *screens.FoldersModel
	activeFilter      *store.FolderFilter
	logo              components.LogoLoader
	typingSerial      int
	// msgHighlightSerial guards the jump-to message-highlight fade loop so a
	// newer highlight or a stale tick is ignored.
	msgHighlightSerial int
	// chatHighlightSerial guards the chat-list row highlight fade loop.
	chatHighlightSerial int
	tmpDir              string
	voicePlayer         *audio.Player
	filePicker          *screens.FilePickerModel
	videoPlayer         *videoPlayer
	photoViewer         *photoViewer
	pendingAttachment   *pendingAttachment
	lastPickerDir       string
	uploadCancels       map[int]context.CancelFunc
	uploadProgress      map[int]chan uploadProgressMsg

	// logoTicking / spinnerTicking track whether each animation loop is currently
	// scheduled. The loops self-stop when nothing is visible/active and are
	// re-armed by ensureAnimationTicks on the next event, so an idle app issues no
	// periodic repaints (issue #147).
	logoTicking      bool
	spinnerTicking   bool
	toastAnimTicking bool
}

// Image-cache capacities (entry counts). Thumbnails churn fast and are small;
// full-resolution viewer images are larger, so they get a smaller cap.
const (
	thumbCacheCap = 256
	fullCacheCap  = 32
)

func NewRootModel(client internaltg.Client, st store.Store, historyLimit int, verbose bool) RootModel {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(80)
	sb.SetKeyMap(km)
	ts := components.NewToastStack(80, 24, 3, components.ZoneBottomRight, components.ZoneTopRight)
	ts.SetDarkBackground(true) // matches hasDarkBackground default; updated on theme detection
	cl := screens.NewChatListModel()
	cl.SetFocused(true)
	chat := screens.NewChatModel(80, 24)
	chat.SetKeyMap(km)
	return RootModel{
		ctx:               context.Background(),
		screen:            ScreenLogin,
		focus:             FocusChatList,
		hasDarkBackground: true,
		chatList:          cl,
		chat:              chat,
		folderBar:         screens.NewFoldersModel(),
		statusBar:         sb,
		toasts:            ts,
		vimState:          keys.NewVimState(),
		keyMap:            km,
		matcher:           keys.NewMatcher(km),
		tgClient:          client,
		st:                st,
		historyLimit:      historyLimit,
		verbose:           verbose,
		imageCache:        imagecache.New(thumbCacheCap),
		fullImageCache:    imagecache.New(fullCacheCap),
		gifFrames:         make(map[int64][]image.Image),
		mentionMembers:    make(map[int64][]store.ChatMember),
		kittyStore:        media.NewKittyStore(),
		kittyLive:         make(map[int64]bool),
		logo:              components.NewLogoLoader(80),
		uploadCancels:     make(map[int]context.CancelFunc),
		uploadProgress:    make(map[int]chan uploadProgressMsg),
	}
}

func (m RootModel) CurrentScreen() Screen            { return m.screen }
func (m RootModel) CurrentFocus() Focus              { return m.focus }
func (m RootModel) ChatList() *screens.ChatListModel { return m.chatList }
func (m RootModel) Chat() *screens.ChatModel         { return m.chat }
func (m RootModel) VimMode() keys.VimMode            { return m.vimState.Mode }
func (m RootModel) HasFolders() bool                 { return m.folderBar != nil && m.folderBar.HasFolders() }

// WithScreen returns a copy with the given screen set (used in tests and app init).
func (m RootModel) WithScreen(s Screen) RootModel {
	m.screen = s
	return m
}

func (m RootModel) WithFocus(f Focus) RootModel {
	m.focus = f
	return m
}

// WithContext stores the app lifecycle context so that command closures issuing
// Telegram RPCs are cancelled when the app shuts down, instead of leaking
// goroutines against a tearing-down client.
func (m RootModel) WithContext(ctx context.Context) RootModel {
	m.ctx = ctx
	return m
}

func (m RootModel) WithConfig(cfg *config.Config) RootModel {
	m.cfg = cfg
	if cfg.Photos.DiskCacheSize > 0 {
		if base, err := os.UserCacheDir(); err == nil {
			if mc, err := mediacache.New(filepath.Join(base, "tele", "media"), cfg.Photos.DiskCacheSize); err == nil {
				m.mediaCache = mc
			}
		}
	}
	m.imageMode = media.DetectMode(cfg.Photos.Mode, os.Getenv)
	if m.imageMode == media.ModeKitty {
		m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	}
	m.kittyCap = cfg.Photos.KittyPlacementCap
	m.chat.SetMaxMediaPx(cfg.Photos.MaxLongSidePx)
	m.chat.SetImageMode(m.imageMode)
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	m.toasts = components.NewToastStack(w, h, cfg.UI.Toasts.MaxVisible,
		parseToastZone(cfg.UI.Toasts.ErrorZone), parseToastZone(cfg.UI.Toasts.NotifyZone))
	m.toasts.SetDarkBackground(m.hasDarkBackground)
	return m
}

// parseToastZone maps a config zone string to a ToastZone, defaulting unknown
// values to the bottom-right corner.
func parseToastZone(s string) components.ToastZone {
	switch s {
	case "top-right":
		return components.ZoneTopRight
	case "bottom-left":
		return components.ZoneBottomLeft
	default:
		return components.ZoneBottomRight
	}
}

// WithKeyMap replaces the keymap and rebuilds the matcher and status-bar hints.
func (m RootModel) WithKeyMap(km keys.KeyMap) RootModel {
	m.keyMap = km
	m.matcher = keys.NewMatcher(km)
	m.statusBar.SetKeyMap(km)
	m.chat.SetKeyMap(km)
	return m
}

func (m RootModel) SearchActive() bool           { return m.searchModel != nil }
func (m RootModel) Search() *screens.SearchModel { return m.searchModel }
func (m RootModel) ContextMenuOpen() bool        { return m.contextMenu != nil }
func (m RootModel) ChatMenuOpen() bool           { return m.chatMenu != nil }
func (m RootModel) ReactionPickerOpen() bool     { return m.reactionPicker != nil }
func (m RootModel) MentionPopupOpen() bool       { return m.mentionPopup != nil }
func (m RootModel) OpenPickerOpen() bool         { return m.openPicker != nil }
func (m RootModel) FilePickerOpen() bool         { return m.filePicker != nil }

// SetLoginModel injects the login model after NewRootModel (called by app.go).
func (m *RootModel) SetLoginModel(lm screens.LoginModel) {
	m.login = lm
}

// SetOnChatOpen registers a callback invoked whenever the user opens a chat.
func (m *RootModel) SetOnChatOpen(fn func(int64)) {
	m.onChatOpen = fn
}

func (m *RootModel) SetTmpDir(dir string) {
	m.tmpDir = dir
}

func (m RootModel) TmpDir() string {
	return m.tmpDir
}

func (m RootModel) Init() tea.Cmd {
	m.statusBar.SetVerbose(m.verbose)
	m.statusBar.SetActivePane("chatlist")
	// The logo loop is started by ensureAnimationTicks on the first event (the
	// login splash is visible at startup). Init probes the background color once
	// and enables OS color-scheme reports (mode 2031) for event-driven theme
	// updates (issue #148).
	return tea.Batch(requestBGColorCmd(), enableColorSchemeReportsCmd())
}

// SettleToastsForTest advances the toast slide animation to completion so a
// freshly added toast is fully on screen (test-only). The toast stack is held by
// pointer, so this mutates the shared stack even on a value receiver.
func (m RootModel) SettleToastsForTest() {
	for i := 0; i < 100 && m.toasts.Animating(); i++ {
		m.toasts.StepToastAnim()
	}
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.updateInner(msg)
	rm := next.(RootModel)
	// Reconcile Kitty placements after every event: the visible set may have
	// changed (scroll, chat switch, new message, image load, resize), and only
	// on-screen images should hold a placement (issue: burst transmit corrupts
	// images on heavy chats).
	if rcmd := (&rm).reconcileKittyCmd(); rcmd != nil {
		cmd = tea.Batch(cmd, rcmd)
	}
	// Re-arm the logo/spinner loops if this event made their content
	// visible/active while the loop was asleep (issue #147).
	if acmd := (&rm).ensureAnimationTicks(); acmd != nil {
		cmd = tea.Batch(cmd, acmd)
	}
	return rm, cmd
}

func (m RootModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// message operations (steps 1+2)
	case store.Event:
		return m.handleStoreEvent(msg)
	case screens.SendMsgRequest:
		return m.handleSendMsg(msg)
	case screens.EditSendRequest:
		return m.handleEditSend(msg)
	case screens.SetTypingRequest:
		return m.handleSetTyping(msg)
	case screens.FileSelectedMsg:
		return m.handleFileSelected(msg)
	case screens.CloseFilePickerMsg:
		m.filePicker = nil
		m.statusBar.SetPickerOpen(false)
		return m, nil
	case screens.SendMediaRequest:
		if m.pendingAttachment == nil {
			return m, nil
		}
		att := m.pendingAttachment
		job := mediaSendJob{
			peer:         msg.Peer,
			path:         att.path,
			name:         att.name,
			size:         att.size,
			kind:         att.sendAs,
			caption:      msg.Caption,
			entities:     msg.Entities,
			replyToMsgID: msg.ReplyToMsgID,
		}
		if att.sendAs == store.MediaVideo {
			job.buildMediaCtx = videoBuildMediaCtx(att.path, att.name, att.mime)
		} else {
			build, ok := mediaBuilderFor(att)
			if !ok {
				// Unsupported "send as" (voice/round) is #108-109; ignore for now.
				return m, nil
			}
			job.buildMedia = build
		}
		m.clearPendingAttachment()
		return m.handleSendMedia(job)
	case uploadProgressMsg:
		return m.handleUploadProgress(msg)
	case sentMediaConfirmedMsg:
		return m.handleSentMediaConfirmed(msg)
	case sentMediaRefreshedMsg:
		return m.handleSentMediaRefreshed(msg)
	case gifFileReadyMsg:
		return m.handleGifFileReady(msg)
	case gifFramesReadyMsg:
		return m.handleGifFramesReady(msg)
	case gifTickMsg:
		return m.handleGifTick(msg)
	case videoFileReadyMsg:
		return m.handleVideoFileReady(msg)
	case videoProbedMsg:
		return m.handleVideoProbed(msg)
	case videoTickMsg:
		return m.handleVideoTick(msg)
	case sentMsgConfirmedMsg:
		return m.handleSentMsgConfirmed(msg)
	case reactionFailedMsg:
		return m.handleReactionFailed(msg)
	case deleteMsgFailedMsg:
		return m.handleDeleteMsgFailed(msg)
	case editMsgFailedMsg:
		return m.handleEditMsgFailed(msg)
	case components.ReactConfirmedMsg:
		return m.handleReactConfirmed(msg)
	case components.MentionSelectedMsg:
		return m.handleMentionSelected(msg)
	case components.CloseMentionPopupMsg:
		return m.handleCloseMentionPopup()
	case participantsLoadedMsg:
		return m.handleParticipantsLoaded(msg)
	case components.DeleteMsgRequest:
		return m.handleDeleteMsg(msg)
	case screens.ForwardToChatRequest:
		return m.handleForwardToChat(msg)
	case screens.SearchUsersRequest:
		return m.handleSearchUsers(msg)
	case forwardDoneMsg:
		return m.handleForwardDone(msg)
	case StatusErrMsg:
		return m.handleStatusErr(msg)
	case clipboardImagePastedMsg:
		return m.handleClipboardImagePasted(msg)
	case components.ComposerLimitMsg:
		return m.handleComposerLimit(msg)
	case documentOpenDoneMsg:
		return m.handleDocumentOpenDone(msg)
	case fileDownloadDoneMsg:
		return m.handleFileDownloadDone(msg)
	case messageCopiedMsg:
		if msg.ok {
			m.statusBar.SetStatus("Copied")
		}
		return m, nil
	case components.CopyMsgRequest:
		if text, ok := m.chat.SelectedMessageText(); ok {
			return m, copyToClipboardCmd(text)
		}
		return m, nil
	case components.OpenTargetChosenMsg:
		m.openPicker = nil
		return m.openTarget(msg.Target)
	case components.CloseOpenPickerMsg:
		m.openPicker = nil
		return m, nil
	case ClearStatusErrMsg:
		m.toasts.Dismiss(msg.Serial)
		return m, nil
	case notifyOpenMsg:
		// Clicking a notify toast dismisses it and opens the target chat via the
		// existing open path.
		m.toasts.Dismiss(msg.serial)
		chat := msg.chat
		return m, func() tea.Msg { return screens.OpenChatMsg{Chat: chat} }
	case chatLoadErrMsg:
		return m.handleChatLoadErr(msg)
	case retryChatLoadMsg:
		m.chat.SetLoading(true)
		m.chat.SetLoadError("")
		return m, m.retryChatLoadCmd(msg.chatID)
	case mediaRefRefreshedMsg:
		if m.st != nil {
			m.st.UpdateMessageMedia(msg.chatID, msg.msgID, msg.photo, msg.doc)
		}
		return m, nil
	// network/data messages
	case screens.OpenChatMsg,
		ChatHistoryMsg,
		screens.LoadMoreMsg,
		historyChunkMsg,
		markReadDoneMsg,
		PhotoReadyMsg,
		FullPhotoReadyMsg,
		kittyEncodedMsg,
		kittyTransmittedMsg,
		components.OpenInViewerRequest,
		components.OpenExternalRequest,
		components.DownloadFileRequest,
		components.PlayVoiceRequest,
		voicePlayReadyMsg,
		voiceTickMsg:
		return m.updateNetworkMsg(msg)
	// UI/layout/animation messages
	case tea.BackgroundColorMsg,
		tea.WindowSizeMsg,
		retransmitTickMsg,
		FolderFiltersMsg,
		screens.FolderSelectedMsg,
		screens.TransitionToMainMsg,
		screens.CloseSearchMsg,
		screens.SearchUsersResult,
		components.JumpToMsgRequest,
		components.ReplyMsgRequest,
		components.ForwardMsgRequest,
		components.EditMsgRequest,
		components.CloseContextMenuMsg,
		components.ReactMsgRequest,
		components.CloseReactionPickerMsg,
		components.LogoTickMsg,
		components.SpinnerTickMsg,
		toastAnimTickMsg,
		tea.FocusMsg,
		tea.BlurMsg,
		uv.DarkColorSchemeEvent,
		uv.LightColorSchemeEvent,
		components.TypingDotsTickMsg,
		clearTypingMsg,
		msgHighlightFadeMsg,
		chatHighlightFadeMsg,
		screens.AuthRequestMsg,
		screens.ConnectedMsg,
		screens.AuthErrorMsg,
		components.ToggleMuteRequest,
		components.ToggleUnreadRequest,
		components.AddToFolderRequest,
		components.ToggleArchiveRequest,
		components.ComposerFlashOffMsg,
		tea.PasteMsg:
		return m.updateUIMsg(msg)
	// mouse input
	case tea.MouseClickMsg, tea.MouseWheelMsg:
		return m.handleMouse(msg)
	// key input
	case tea.KeyPressMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			return m, cmd
		}
		return m.handleMainKey(msg)
	}
	// Internal SearchModel ticks (debounce/spinner) use unexported types, so they
	// cannot be named in the switch above; forward them to the open overlay (#82).
	if m.searchModel != nil && screens.IsSearchInternalMsg(msg) {
		newSearch, cmd := m.searchModel.Update(msg)
		m.searchModel = newSearch
		return m, cmd
	}
	return m, nil
}
