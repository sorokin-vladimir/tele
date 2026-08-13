package ui

import (
	"context"
	"image"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/audio"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/imagecache"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/sorokin-vladimir/tele/internal/version"
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
	ctx       context.Context
	screen    Screen
	focus     Focus
	width     int
	height    int
	chatList  *screens.ChatListModel
	chat      *screens.ChatModel
	login     screens.LoginModel
	statusBar *components.StatusBar
	toasts    *components.ToastStack
	vimState  *keys.VimState
	keyMap    keys.KeyMap
	matcher   *keys.Matcher
	st        store.Store
	owner     Owner
	// chatListSub is the chatlist subscription; chatSub is the open chat's.
	// Zero means not subscribed.
	chatListSub project.SubID
	chatSub     project.SubID
	// chatWindow is the open chat's current window, kept so a scroll can widen
	// it rather than describe a delta. chatMsgs is the window's contents, which
	// the client maintains from the deltas because the pane renders from a whole
	// slice rather than applying edits itself.
	chatWindow project.ChatWindow
	chatMsgs   []domain.Message
	// chatUnreadReactions is the open chat's unread-reaction count, from the
	// projection. Kept so focusing the pane can mark them read: a reaction that
	// arrived while you were elsewhere is only seen when you look.
	chatUnreadReactions int
	// activeFolder is the folder the chatlist window is filtered by, kept so a
	// window move can repeat it. 0 is All Chats.
	activeFolder  int
	currentChatID int64
	historyLimit  int
	verbose       bool
	log           *zap.Logger
	cfg           *config.Config
	// configWarnings are the non-fatal config notices, shown as toasts once the
	// TUI is up. They are also logged and printed to stderr, but stderr is wiped
	// by the alt-screen a moment later, so on their own those two amount to
	// telling nobody.
	configWarnings []config.Warning
	// Pending one-time startup notices (#197). The head of the queue is shown
	// above every screen, including login; noticeLeft counts down whole seconds
	// of blocked dismissal.
	noticeQueue []notices.Notice
	noticeLeft  int
	noticeSeen  notices.Seen
	// The decoded in-memory image caches stay here: decoding serves rendering.
	// The on-disk media cache belongs to the owner (#196).
	imageCache     *imagecache.Cache
	fullImageCache *imagecache.Cache
	// avatars remembers people's pictures between openings of the profile
	// overlay. Kept apart from imageCache on purpose: a scrolled chat must not
	// evict the faces of the people you talk to (#223).
	avatars *avatarStore
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
	contextMenu       *components.ContextMenu
	chatMenu          *components.ChatContextMenu
	reactionPicker    *components.ReactionPicker
	help              *components.HelpModal
	profile           *components.Profile
	openPicker        *components.OpenPicker
	reactionTargetID  int
	mentionPopup      *components.MentionPopup
	mentionMembers    map[int64][]domain.ChatMember
	folderBar         *screens.FoldersModel
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
	pendingAttachments  []pendingAttachment
	lastPickerDir       string

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

// NewRootModel builds the TUI. It takes no Telegram client: since #195 every
// call a client makes goes through the Owner, so nothing here can reach the
// connection directly (#198).
func NewRootModel(st store.Store, historyLimit int, verbose bool) RootModel {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(80)
	sb.SetKeyMap(km)
	sb.SetVersion(version.Version)
	ts := components.NewToastStack(80, 24, 3, components.ZoneBottomRight, components.ZoneTopRight)
	// Assume a dark terminal until detection reports otherwise. Which themes
	// are switched between was decided at startup, before the TUI existed.
	theme.Apply(true)
	cl := screens.NewChatListModel()
	cl.SetFocused(true)
	chat := screens.NewChatModel(80, 24)
	chat.SetKeyMap(km)
	return RootModel{
		ctx:            context.Background(),
		screen:         ScreenLogin,
		focus:          FocusChatList,
		chatList:       cl,
		chat:           chat,
		folderBar:      screens.NewFoldersModel(),
		statusBar:      sb,
		toasts:         ts,
		vimState:       keys.NewVimState(),
		keyMap:         km,
		matcher:        keys.NewMatcher(km),
		st:             st,
		historyLimit:   historyLimit,
		verbose:        verbose,
		imageCache:     imagecache.New(thumbCacheCap),
		fullImageCache: imagecache.New(fullCacheCap),
		avatars:        newAvatarStore(),
		gifFrames:      make(map[int64][]image.Image),
		mentionMembers: make(map[int64][]domain.ChatMember),
		kittyStore:     media.NewKittyStore(),
		kittyLive:      make(map[int64]bool),
		logo:           components.NewLogoLoader(80),
	}
}

func (m RootModel) CurrentScreen() Screen            { return m.screen }
func (m RootModel) CurrentFocus() Focus              { return m.focus }
func (m RootModel) ChatList() *screens.ChatListModel { return m.chatList }

// Owner returns the attached connection owner, nil when none is. Tests drive the
// model through a stand-in and need to reach it.
func (m RootModel) Owner() Owner { return m.owner }

// CurrentChatID is the chat the client has open, 0 when none.
func (m RootModel) CurrentChatID() int64     { return m.currentChatID }
func (m RootModel) Chat() *screens.ChatModel { return m.chat }
func (m RootModel) VimMode() keys.VimMode    { return m.vimState.Mode }
func (m RootModel) HasFolders() bool         { return m.folderBar != nil && m.folderBar.HasFolders() }

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

// WithLogger attaches the app logger so the client can say what it did with a
// delta. Optional: every use goes through m.debug, which is a no-op when unset,
// so tests and any other caller need not supply one.
func (m RootModel) WithLogger(log *zap.Logger) RootModel {
	m.log = log
	return m
}

// debug logs at debug level when a logger is attached, and does nothing when it
// is not.
func (m RootModel) debug(msg string, fields ...zap.Field) {
	if m.log != nil {
		m.log.Debug(msg, fields...)
	}
}

func (m RootModel) WithConfig(cfg *config.Config) RootModel {
	m.cfg = cfg
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
	m.configWarnings = cfg.Warnings
	return m
}

// configWarningDuration is how long a config warning stays on screen. It is
// longer than an ordinary warning toast because it appears during startup, when
// the user is not necessarily looking yet, and because acting on it means
// editing a file rather than retrying something.
const configWarningDuration = 15 * time.Second

// configWarningCmds shows each config warning as a toast and returns the timers
// that retire them.
//
// A warning carrying an ID is advisory — a dead key that changes nothing — and
// is shown only until it has been seen once. The rest describe something still
// broken and reappear every launch, because every launch they are still true.
func (m RootModel) configWarningCmds() []tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.configWarnings))
	for _, w := range m.configWarnings {
		if w.ID != "" && m.noticeSeen != nil {
			if m.noticeSeen.IsSeen(w.ID) {
				continue
			}
			m.noticeSeen.MarkSeen(w.ID)
		}
		serial := m.toasts.Add(components.ToastWarning, w.Text)
		cmds = append(cmds, tea.Tick(configWarningDuration, func(time.Time) tea.Msg {
			return ClearStatusErrMsg{Serial: serial}
		}))
	}
	return cmds
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

func (m RootModel) StatusText() string           { return m.statusBar.Status() }
func (m RootModel) SearchActive() bool           { return m.searchModel != nil }
func (m RootModel) Search() *screens.SearchModel { return m.searchModel }
func (m RootModel) ContextMenuOpen() bool        { return m.contextMenu != nil }
func (m RootModel) ChatMenuOpen() bool           { return m.chatMenu != nil }
func (m RootModel) ReactionPickerOpen() bool     { return m.reactionPicker != nil }
func (m RootModel) MentionPopupOpen() bool       { return m.mentionPopup != nil }
func (m RootModel) OpenPickerOpen() bool         { return m.openPicker != nil }
func (m RootModel) FilePickerOpen() bool         { return m.filePicker != nil }
func (m RootModel) ProfileOpen() bool            { return m.profile != nil }

// Profile is the open profile overlay, nil when none is. Tests read what it drew.
func (m RootModel) Profile() *components.Profile { return m.profile }

// SetLoginModel injects the login model after NewRootModel (called by app.go).
func (m *RootModel) SetLoginModel(lm screens.LoginModel) {
	m.login = lm
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
	cmds := []tea.Cmd{requestBGColorCmd(), enableColorSchemeReportsCmd()}
	if m.noticeActive() {
		cmds = append(cmds, noticeTickCmd())
	}
	cmds = append(cmds, m.configWarningCmds()...)
	return tea.Batch(cmds...)
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
	case noticeTickMsg:
		if !m.noticeActive() {
			return m, nil
		}
		return m.noticeTick(), noticeTickCmd()
	// projection deltas and events from the connection owner (#194)
	case project.Delta:
		return m.handleDelta(msg)
	case core.Incoming:
		return m.handleIncoming(msg)
	case core.Notification:
		return m.handleNotification(msg)
	case core.Failure:
		return m.handleFailure(msg)
	case core.Typing:
		return m.handleTyping(msg)
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
		return m.handleSendMedia(msg)
	case core.Progress:
		return m.handleUploadProgress(msg)
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
	case components.RetryOutboxRequest:
		return m.handleRetryOutbox(msg)
	case components.DiscardOutboxRequest:
		return m.handleDiscardOutbox(msg)
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
	case usernameCopiedMsg:
		if msg.ok {
			m.statusBar.SetStatus("Copied " + msg.handle)
		}
		return m, nil
	case components.OpenProfileRequest,
		components.CloseProfileMsg,
		components.ProfileOpenChatRequest,
		components.ProfileMuteRequest,
		components.ProfileCopyUsernameRequest,
		profileLoadedMsg,
		avatarReadyMsg:
		next, cmd, _ := m.handleProfileRequest(msg)
		return next, cmd
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
		chatID, title := msg.chatID, msg.title
		return m, func() tea.Msg { return screens.OpenChatMsg{ChatID: chatID, Title: title} }
	case chatLoadErrMsg:
		return m.handleChatLoadErr(msg)
	case retryChatLoadMsg:
		// Retrying is resubscribing: the subscription's first delta is a full
		// Reset, and the owner refills the window from Telegram if the store
		// still falls short.
		m.chat.SetLoading(true)
		m.chat.SetLoadError("")
		m.subscribeChat(msg.chatID, domain.Peer{})
		return m, nil
	// network/data messages
	case screens.OpenChatMsg,
		screens.LoadMoreMsg,
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
		// A startup notice swallows every key until its countdown ends, then
		// consumes one more to dismiss itself.
		if next, handled := m.handleNoticeKey(); handled {
			return next, nil
		}
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
