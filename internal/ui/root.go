package ui

import (
	"context"
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/audio"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/notices"
	"github.com/sorokin-vladimir/tele/internal/settings"
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
	owner     Owner
	// Width negotiation can finish after the first frame was painted.
	widthModeReported bool
	widthRepainted    bool
	// chatListSub is the chatlist subscription; chatSub is the open chat's.
	// Zero means not subscribed.
	chatListSub project.SubID
	chatSub     project.SubID
	// chatWindow is the open chat's current window, kept so a scroll can widen
	// it rather than describe a delta. chatMsgs is the window's contents, which
	// the client maintains from the deltas because the pane renders from a whole
	// slice rather than applying edits itself.
	chatWindow project.HistoryWindow
	chatMsgs   []domain.Message
	// chatUnreadReactions is the open chat's unread-reaction count, from the
	// projection. Kept so focusing the pane can mark them read: a reaction that
	// arrived while you were elsewhere is only seen when you look.
	chatUnreadReactions int
	// chatDraft is the open chat's draft as the server knows it, from the
	// projection. Leaving the chat saves the composer only when it differs.
	chatDraft string
	// readMentionsOnReset is set when a chat is opened and cleared by its first
	// Reset, which is the one that reads the chat's mentions. Later Resets - a
	// moved window, a retried load - are not an opening.
	readMentionsOnReset bool
	// activeFolder is the folder the chatlist window is filtered by, kept so a
	// window move can repeat it. 0 is All Chats.
	activeFolder  int
	currentChatID int64
	// currentTopicID is the forum topic open in the chat pane, 0 for an
	// ordinary chat. With currentChatID it names the history every command
	// from the pane goes to (#275).
	currentTopicID int
	// forum is the forum whose topic list fills the middle pane in place of
	// the chat list, nil while the chat list shows; topicSub is its
	// subscription. lastTopic remembers, per forum and for this run, the topic
	// last opened there, which is where the cursor lands on coming back.
	forum     *screens.TopicListModel
	topicSub  project.SubID
	lastTopic map[int64]int
	// forumCursorPending is set on entering a forum and cleared by the topic
	// list's first Reset, which is when there are rows to place the cursor on.
	forumCursorPending bool
	historyLimit       int
	verbose            bool
	log                *zap.Logger
	// logPath is where this run's log is written, named on the login screen
	// when something goes wrong there. Empty in tests.
	logPath string
	// clockSkew is how far the local clock is from Telegram's, zero when it is
	// close enough to be accepted (#277).
	clockSkew time.Duration
	// throttleUntil is when the flood wait somebody's call ran into is over,
	// and throttleTicking whether its countdown is running (#212).
	throttleUntil   time.Time
	throttleTicking bool
	// hostAway is set while the host is away, and absences counts how many
	// times it has been: the timer that names an absence carries its number,
	// so one that fires after the host is back, or gone again, names nothing
	// (#210).
	hostAway bool
	absences int
	// accountSub is the account subscription, and account what it last said:
	// the login screen, the connection's end and the clock are drawn from it
	// (#309).
	accountSub project.SubID
	account    project.AccountContents
	cfg        *config.Config
	// reloadConfig re-reads this window's config file. Supplied by the app;
	// nil in tests and wherever nothing can be reloaded, in which case the
	// reload action reloads themes alone.
	reloadConfig func() (*config.Config, error)
	// reloadHost asks the host to re-read its settings, and answers with what
	// it found wrong with them. Nil when there is no host to ask (#315).
	reloadHost func() ([]config.Warning, error)
	// settingsStore is where the settings overlay reads from. An interface
	// rather than the config store itself, because the overlay is shaped for a
	// second one - the Telegram account - to arrive beside it (ADR 0010).
	settingsStore settings.Store
	// unsent is what was being typed when the host was lost, kept for the
	// client to hand back on the way out (#208).
	unsent string
	// othersAttached counts the other windows attached to the host this
	// window holds; nil when the host is another tele's (#208).
	othersAttached func() int
	// configWarnings are the non-fatal config notices, shown as toasts once the
	// TUI is up. They are also logged and printed to stderr, but stderr is wiped
	// by the alt-screen a moment later, so on their own those two amount to
	// telling nobody.
	configWarnings []config.Warning
	// themeWarnings are what was wrong with the theme files when the process
	// started, shown the same way. Handed in apart from the config, which is
	// what the file says and is replaced by a reload (#239).
	themeWarnings []string
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
	// Sixel mode paints the photo modal out of band: sixelSeq is the encoded
	// image for the modal currently open, sixelPaintGen invalidates paint ticks
	// scheduled for a modal that has since changed (a repaint must not land
	// under a different photo).
	sixelSeq      string
	sixelSeqCols  int
	sixelSeqRows  int
	sixelPaintGen uint64
	// sixelInline carries the inline message images: the last frame, and what
	// was painted over it. Shared by every copy of the model, like the caches.
	sixelInline      *inlineSixelState
	searchModel      *screens.SearchModel
	contextMenu      *components.ContextMenu
	chatMenu         *components.ChatContextMenu
	reactionPicker   *components.ReactionPicker
	help             *components.HelpModal
	settings         *components.SettingsModal
	confirmQuit      *components.ConfirmModal
	profile          *components.Profile
	profileAsk       int // numbers profile requests; only the latest one's answer opens an overlay
	openPicker       *components.OpenPicker
	reactionTargetID int
	mentionPopup     *components.MentionPopup
	mentionMembers   map[int64][]domain.ChatMember
	folderBar        *screens.FoldersModel
	logo             components.LogoLoader
	typingSerial     int
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

	// logOut is the open log out confirmation, nil when none is (#297).
	logOut *logOutDialog
	// banned is set while the login screen shows a ban, where Enter leaves
	// the banned account to log in with another number (#297).
	banned bool
}

// Image-cache capacities (entry counts). Thumbnails churn fast and are small;
// full-resolution viewer images are larger, so they get a smaller cap.
const (
	thumbCacheCap = 256
	fullCacheCap  = 32
)

// NewRootModel builds the TUI. It takes no Telegram client and no store: every
// call a client makes goes through the Owner, and everything it shows arrives
// from it, so nothing here can reach the connection or the data directly
// (#198, #278). Settings arrive through WithConfig, the same path a reload
// takes (#239).
func NewRootModel(verbose bool) RootModel {
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
		verbose:        verbose,
		imageCache:     imagecache.New(thumbCacheCap),
		fullImageCache: imagecache.New(fullCacheCap),
		avatars:        newAvatarStore(),
		gifFrames:      make(map[int64][]image.Image),
		mentionMembers: make(map[int64][]domain.ChatMember),
		kittyStore:     media.NewKittyStore(),
		kittyLive:      make(map[int64]bool),
		logo:           components.NewLogoLoader(80),
		sixelInline:    newInlineSixelState(),
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

// WithHostWarnings adds what the host found wrong with its own settings when
// it started, to be shown with this window's (#315).
func (m RootModel) WithHostWarnings(warnings []config.Warning) RootModel {
	m.configWarnings = append(m.configWarnings, warnings...)
	return m
}

// WithLogPath records where this run's log is written.
func (m RootModel) WithLogPath(path string) RootModel {
	m.logPath = path
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

// WithImageMode installs how images are drawn. photos.mode is a startup
// setting: the renderer and everything already transmitted to the terminal are
// chosen from it, so the host reads it once per process and every account's
// model is handed the same mode rather than reading the file again (#239).
func (m RootModel) WithImageMode(mode media.Mode) RootModel {
	m.imageMode = mode
	switch mode {
	case media.ModeKitty:
		m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	case media.ModeSixel:
		// Inline images keep their block art in the cells - the layout and the
		// height accounting depend on it - and carry a marker the root model
		// turns into an out-of-band paint.
		m.chat.SetRenderer(media.NewSixelRenderer())
	}
	if m.sixelInline == nil {
		m.sixelInline = newInlineSixelState()
	}
	m.chat.SetImageMode(mode)
	return m
}

// WithConfig installs the config when the model is built. It does the one thing
// that can only be done once - building the toast stack - and then hands over
// to applyConfig, which is the same path a reload takes.
//
// Splitting it this way is what keeps "which settings are live" honest. There is
// one place where a config becomes running state, so a setting is live because
// applyConfig touches it, not because somebody remembered to say so.
func (m RootModel) WithConfig(cfg *config.Config) RootModel {
	w, h := m.width, m.height
	if w == 0 {
		w, h = 80, 24
	}
	m.toasts = components.NewToastStack(w, h, cfg.UI.Toasts.MaxVisible,
		parseToastZone(cfg.UI.Toasts.ErrorZone), parseToastZone(cfg.UI.Toasts.NotifyZone))
	m.configWarnings = windowsWarnings(cfg)
	return m.applyConfig(cfg)
}

// applyConfig makes a config the one the model is running on. Called at startup
// and again on every reload, whether the change was made in the overlay or in an
// editor.
//
// Components are changed rather than rebuilt. Rebuilding the toast stack would
// throw away whatever is on screen, and the timers retiring those toasts are
// already in flight with serials only that stack knows - a setting taking effect
// must not cost the person the notification they were reading.
func (m RootModel) applyConfig(cfg *config.Config) RootModel {
	m.cfg = cfg

	// Immediate.
	m.toasts.SetZones(parseToastZone(cfg.UI.Toasts.ErrorZone), parseToastZone(cfg.UI.Toasts.NotifyZone))
	m.toasts.SetMaxVisible(cfg.UI.Toasts.MaxVisible)

	// Next-use: what is drawn stays as it was drawn, and the new value is what
	// the next image is drawn at. How much history a window holds is not here:
	// it is the host's setting, and arrives with the account (#208).
	m.kittyCap = cfg.Photos.KittyPlacementCap
	m.chat.SetMaxMediaPx(cfg.Photos.MaxLongSidePx)

	return m
}

// windowsWarnings is what is wrong with this window's own settings in cfg. The
// host reads the same file and says what is wrong with its own, so a warning
// about a host setting is left to it and said once (#315).
func windowsWarnings(cfg *config.Config) []config.Warning {
	var out []config.Warning
	for _, w := range cfg.Warnings {
		if w.Process != settings.Host {
			out = append(out, w)
		}
	}
	return out
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
	cmds := make([]tea.Cmd, 0, len(m.configWarnings)+len(m.themeWarnings))
	for _, w := range m.configWarnings {
		if w.ID != "" && m.noticeSeen != nil {
			if m.noticeSeen.IsSeen(w.ID) {
				continue
			}
			m.noticeSeen.MarkSeen(w.ID)
		}
		cmds = append(cmds, m.warningToast(w.Text))
	}
	// A theme problem is still true next launch, so it carries no ID and is
	// shown every time.
	for _, w := range m.themeWarnings {
		cmds = append(cmds, m.warningToast("theme: "+w))
	}
	return cmds
}

// WithThemeWarnings installs what was wrong with the theme files when the
// process started, to be shown with the config warnings.
func (m RootModel) WithThemeWarnings(warnings []string) RootModel {
	m.themeWarnings = warnings
	return m
}

// warningToast shows a startup warning and returns the timer that retires it.
func (m RootModel) warningToast(text string) tea.Cmd {
	serial := m.toasts.Add(components.ToastWarning, text)
	return tea.Tick(configWarningDuration, func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}

// parseToastZone translates a zone name into a ToastZone. It decides nothing:
// which names are legal is declared once, in the settings registry, and a name
// that is not one of them is repaired and reported when the config is loaded.
// What is left here is the empty string - a config that named no zone at all -
// and the corner it falls to.
func parseToastZone(s string) components.ToastZone {
	switch s {
	case "top-right":
		return components.ZoneTopRight
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
	if m.imageMode == media.ModeSixel {
		// Only sixel needs the cell pixel size, and only a terminal that speaks
		// sixel but not TIOCGWINSZ pixels (foot) has to be asked for it.
		cmds = append(cmds, requestCellSizeCmd())
	}
	if m.screen == ScreenLogin {
		cmds = append(cmds, screens.SlowConnectTick())
	}
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
	// Sixel mode paints inline images out of band, after the frame. Any event
	// may have rewritten the cells under one, so ask for a paint after this one;
	// the paint itself is the one event that must not ask again.
	_, isPaint := msg.(inlineSixelTickMsg)
	if encoded, ok := msg.(inlineSixelEncodedMsg); ok && encoded.seq == "" {
		encoded.state.mu.Lock()
		// A genuine failure must not create an idle loop. A cancelled old
		// viewport still owes the current frame a paint after its worker exits.
		isPaint = encoded.state == rm.sixelInline && encoded.revision == encoded.state.revision
		encoded.state.mu.Unlock()
	}
	if !isPaint {
		if pcmd := rm.scheduleInlineSixelPaint(); pcmd != nil {
			cmd = tea.Batch(cmd, pcmd)
		}
	}
	return rm, cmd
}

func (m RootModel) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.ModeReportMsg:
		if msg.Mode != ansi.ModeUnicodeCore || m.widthModeReported ||
			(msg.Value != ansi.ModeReset && msg.Value != ansi.ModeSet && msg.Value != ansi.ModePermanentlySet) {
			return m, nil
		}
		m.widthModeReported = true
		// Bubble Tea switches its width method before forwarding this reply, but
		// does not invalidate the previous frame. Clear first, then change the view
		// identity so even an otherwise idle window flushes that pending erase.
		return m, tea.Sequence(tea.ClearScreen, func() tea.Msg { return widthRepaintMsg{} })
	case widthRepaintMsg:
		m.widthRepainted = true
		m.sixelInline.invalidatePaints()
		return m, m.scheduleSixelPaint()
	case noticeTickMsg:
		if !m.noticeActive() {
			return m, nil
		}
		return m.noticeTick(), noticeTickCmd()
	case throttleTickMsg:
		return m.handleThrottleTick(msg)
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
	case core.DownloadProgress:
		return m.handleDownloadProgress(msg)
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
	case reactionNotKeptMsg:
		return m.handleReactionNotKept(msg)
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
	case screens.ForwardForumChosen:
		return m.handleForwardForumChosen(msg)
	case forwardTopicsMsg:
		return m.handleForwardTopics(msg)
	case screens.ForwardToChatRequest:
		return m.handleForwardToChat(msg)
	case screens.SearchUsersRequest:
		return m.handleSearchUsers(msg)
	case searchChatsMsg:
		return m.handleSearchChats(msg)
	case chatMenuFoldersMsg:
		return m.handleChatMenuFolders(msg)
	case forwardDoneMsg:
		return m.handleForwardDone(msg)
	case StatusErrMsg:
		return m.handleStatusErr(msg)
	case logOutPreviewMsg:
		return m.handleLogOutPreview(msg)
	case logOutDoneMsg:
		return m.handleLogOutDone(msg)
	case core.HostFailure:
		// What the host could not do lands where a connection that ended
		// does: on the login screen, or in a toast that stays (#309).
		if msg.Err == nil {
			return m, nil
		}
		return m.handleConnectFailed(msg.Err)
	case screens.LoginAnswerMsg:
		return m.handleLoginAnswer(msg)
	case HostLostMsg:
		return m.handleHostLost(msg)
	case HostAwayMsg:
		return m.handleHostAway()
	case HostBackMsg:
		return m.handleHostBack(), nil
	case hostAwayTickMsg:
		return m.handleHostAwayTick(msg), nil
	case screens.NotSentWhileAwayMsg:
		return m.handleStatusErr(StatusErrMsg{
			Text: "Not sent: tele is reconnecting to the tele that holds this account. The text is kept.",
			Sev:  components.SeverityWarning,
		})
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
		knownUserMsg,
		profileLoadedMsg,
		profileDialogMsg,
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
		// A notification from a forum topic opens that topic (#275).
		m.toasts.Dismiss(msg.serial)
		open := screens.OpenChatMsg{
			ChatID: msg.chatID, Title: msg.title,
			IsForum: msg.topicID != 0, TopicID: msg.topicID, TopicTitle: msg.topicTitle,
		}
		return m, func() tea.Msg { return open }
	case chatLoadErrMsg:
		return m.handleChatLoadErr(msg)
	case retryChatLoadMsg:
		// Retrying is resubscribing: the subscription's first delta is a full
		// Reset, and the owner refills the window from Telegram if the store
		// still falls short.
		m.chat.SetLoading(true)
		m.chat.SetLoadError("")
		h := domain.HistoryKey{ChatID: msg.chatID}
		if msg.chatID == m.currentChatID {
			h = m.currentHistory()
		}
		m.subscribeChat(h)
		return m, nil
	// network/data messages
	case screens.OpenChatMsg,
		screens.OpenTopicMsg,
		screens.LeaveForumMsg,
		screens.LoadMoreMsg,
		PhotoReadyMsg,
		FullPhotoReadyMsg,
		kittyEncodedMsg,
		kittyTransmittedMsg,
		sixelEncodedMsg,
		sixelPaintMsg,
		inlineSixelTickMsg,
		inlineSixelEncodedMsg,
		inlineSixelClearedMsg,
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
		uv.CellSizeEvent,
		components.TypingDotsTickMsg,
		clearTypingMsg,
		msgHighlightFadeMsg,
		chatHighlightFadeMsg,
		screens.AuthRequestMsg,
		screens.ConnectedMsg,
		screens.AuthErrorMsg,
		screens.SlowConnectMsg,
		screens.LoginRefusedMsg,
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
		// consumes one more to dismiss itself; one that asks waits for its answer.
		if next, cmd, handled := m.handleNoticeAnswer(msg); handled {
			return next, cmd
		}
		if next, handled := m.handleNoticeKey(); handled {
			return next, nil
		}
		if m.screen == ScreenLogin {
			if m.isLoginQuit(msg) {
				return m, tea.Quit
			}
			if next, cmd, handled := m.handleBannedKey(msg); handled {
				return next, cmd
			}
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
