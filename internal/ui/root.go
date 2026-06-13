package ui

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/sorokin-vladimir/tele/internal/audio"
	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
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

type ChatHistoryMsg struct {
	ChatID   int64
	Messages []store.Message
}

type PhotoReadyMsg struct {
	PhotoID int64
	Image   image.Image
}

type FullPhotoReadyMsg struct {
	PhotoID int64
	Image   image.Image
}

// kittyTransmittedMsg is emitted after a photo's Kitty virtual placement has
// been written to the terminal. Only then is the image marked ready, so the
// placeholder grid is never painted before the placement exists.
type kittyTransmittedMsg struct {
	photoID int64
	cols    int
}

// voicePlayReadyMsg carries a downloaded voice file ready to be played.
type voicePlayReadyMsg struct {
	docID int64
	data  []byte
}

// voiceTickMsg drives the voice playback position/playhead updates.
type voiceTickMsg struct{}

// retransmitTickMsg fires after the photo-width debounce window. Only the tick
// whose gen matches the latest scheduled one performs the retransmit; earlier
// ticks were superseded by a newer width change.
type retransmitTickMsg struct {
	gen int
}

type markReadDoneMsg struct {
	chatID int64
	maxID  int
}

type historyChunkMsg struct {
	chatID   int64
	messages []store.Message
	err      error
}

type FolderFiltersMsg struct {
	Filters []store.FolderFilter
}

type clearTypingMsg struct{ serial int }

// StatusErrMsg surfaces a transient, severity-tagged error in the status bar.
type StatusErrMsg struct {
	Text string
	Sev  components.Severity
}

// ClearStatusErrMsg clears the status-bar error identified by Serial.
type ClearStatusErrMsg struct{ Serial int }

// chatLoadErrMsg reports a failed chat-open history load.
type chatLoadErrMsg struct {
	chatID int64
	text   string
}

// mediaRefRefreshedMsg carries refreshed media refs after a FILE_REFERENCE_EXPIRED,
// so the store can keep the fresh refs for subsequent opens.
type mediaRefRefreshedMsg struct {
	chatID int64
	msgID  int
	photo  *store.PhotoRef
	doc    *store.DocumentRef
}

func durationFor(sev components.Severity) time.Duration {
	switch sev {
	case components.SeverityError:
		return 10 * time.Second
	case components.SeverityWarning:
		return 8 * time.Second
	default:
		return 5 * time.Second
	}
}

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
	imageCache       map[int64]image.Image
	fullImageCache   map[int64]image.Image
	imageMode        media.Mode
	kittyStore       *media.KittyStore
	lastPhotoCols    int
	lastPaneHeight   int
	retransmitGen    int
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
	reactionTargetID  int
	folderBar         *screens.FoldersModel
	activeFilter      *store.FolderFilter
	logo              components.LogoLoader
	typingSerial      int
	tmpDir            string
	voicePlayer       *audio.Player
}

func NewRootModel(client internaltg.Client, st store.Store, historyLimit int, verbose bool) RootModel {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(80)
	sb.SetKeyMap(km)
	cl := screens.NewChatListModel()
	cl.SetFocused(true)
	return RootModel{
		ctx:               context.Background(),
		screen:            ScreenLogin,
		focus:             FocusChatList,
		hasDarkBackground: true,
		chatList:          cl,
		chat:              screens.NewChatModel(80, 24),
		folderBar:         screens.NewFoldersModel(),
		statusBar:         sb,
		vimState:          keys.NewVimState(),
		keyMap:            km,
		matcher:           keys.NewMatcher(km),
		tgClient:          client,
		st:                st,
		historyLimit:      historyLimit,
		verbose:           verbose,
		imageCache:        make(map[int64]image.Image),
		fullImageCache:    make(map[int64]image.Image),
		kittyStore:        media.NewKittyStore(),
		kittyLive:         make(map[int64]bool),
		logo:              components.NewLogoLoader(80),
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
	m.imageMode = media.DetectMode(cfg.Photos.Mode, os.Getenv)
	if m.imageMode == media.ModeKitty {
		m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	}
	m.kittyCap = cfg.Photos.KittyPlacementCap
	m.chat.SetMaxMediaPx(cfg.Photos.MaxLongSidePx)
	return m
}

// WithKeyMap replaces the keymap and rebuilds the matcher and status-bar hints.
func (m RootModel) WithKeyMap(km keys.KeyMap) RootModel {
	m.keyMap = km
	m.matcher = keys.NewMatcher(km)
	m.statusBar.SetKeyMap(km)
	return m
}

func (m RootModel) SearchActive() bool           { return m.searchModel != nil }
func (m RootModel) Search() *screens.SearchModel { return m.searchModel }
func (m RootModel) ContextMenuOpen() bool        { return m.contextMenu != nil }
func (m RootModel) ChatMenuOpen() bool           { return m.chatMenu != nil }
func (m RootModel) ReactionPickerOpen() bool     { return m.reactionPicker != nil }

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

func (m RootModel) filteredChats() []store.Chat {
	if m.st == nil {
		return nil
	}
	all := m.st.Chats()

	// Archive virtual folder: only archived chats.
	if m.activeFilter != nil && m.activeFilter.ID == store.ArchiveFolderID {
		out := make([]store.Chat, 0)
		for _, c := range all {
			if c.IsArchived {
				out = append(out, c)
			}
		}
		return out
	}

	// All Chats: every non-archived chat.
	if m.activeFilter == nil {
		out := make([]store.Chat, 0, len(all))
		for _, c := range all {
			if !c.IsArchived {
				out = append(out, c)
			}
		}
		return out
	}

	// Custom filter: matches and not archived.
	out := make([]store.Chat, 0, len(all))
	for _, c := range all {
		if !c.IsArchived && m.activeFilter.Matches(c) {
			out = append(out, c)
		}
	}
	return out
}

func (m RootModel) computeFolderUnreads() map[int]int {
	counts := make(map[int]int)
	if m.st == nil || m.folderBar == nil {
		return counts
	}
	chats := m.st.Chats()
	for _, f := range m.folderBar.Folders() {
		// All Chats has no badge; Archive intentionally shows no unread
		// count (mirrors the official client).
		if f.ID == 0 || f.ID == store.ArchiveFolderID {
			continue
		}
		chatsWithUnread := 0
		for _, c := range chats {
			if !c.IsArchived && f.Matches(c) && c.UnreadCount > 0 {
				chatsWithUnread++
			}
		}
		counts[f.ID] = chatsWithUnread
	}
	return counts
}

// syncFolderBar refreshes the folder pane's unread badges and toggles the
// Archive entry's presence based on whether any archived chat exists.
func (m RootModel) syncFolderBar() {
	if m.folderBar == nil || m.st == nil {
		return
	}
	m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
	m.folderBar.SetArchivePresent(m.hasArchivedChats())
}

func (m RootModel) hasArchivedChats() bool {
	if m.st == nil {
		return false
	}
	for _, c := range m.st.Chats() {
		if c.IsArchived {
			return true
		}
	}
	return false
}

func (m RootModel) Init() tea.Cmd {
	m.statusBar.SetVerbose(m.verbose)
	m.statusBar.SetActivePane("chatlist")
	return tea.Batch(logoTickCmd(), requestBGColorCmd())
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
	case components.DeleteMsgRequest:
		return m.handleDeleteMsg(msg)
	case StatusErrMsg:
		return m.handleStatusErr(msg)
	case ClearStatusErrMsg:
		m.statusBar.ClearError(msg.Serial)
		return m, nil
	case chatLoadErrMsg:
		return m.handleChatLoadErr(msg)
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
		kittyTransmittedMsg,
		components.OpenInViewerRequest,
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
		components.JumpToMsgRequest,
		components.ReplyMsgRequest,
		components.EditMsgRequest,
		components.CloseContextMenuMsg,
		components.ReactMsgRequest,
		components.CloseReactionPickerMsg,
		components.LogoTickMsg,
		components.SpinnerTickMsg,
		components.TypingDotsTickMsg,
		clearTypingMsg,
		screens.AuthRequestMsg,
		screens.ConnectedMsg,
		screens.AuthErrorMsg,
		components.ToggleMuteRequest,
		components.ToggleUnreadRequest,
		components.AddToFolderRequest,
		components.ToggleArchiveRequest,
		tea.PasteMsg:
		return m.updateUIMsg(msg)
	// key input
	case tea.KeyPressMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			return m, cmd
		}
		return m.handleMainKey(msg)
	}
	return m, nil
}

func (m RootModel) handleMainKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.statusBar.SetStatus("")
	if m.reactionPicker != nil {
		newPicker, cmd := m.reactionPicker.Update(msg)
		m.reactionPicker = newPicker
		return m, cmd
	}
	// While context menu is open, route all keys to it.
	if m.contextMenu != nil {
		newCM, cmd := m.contextMenu.Update(msg)
		m.contextMenu = newCM
		return m, cmd
	}
	if m.chatMenu != nil {
		newCM, cmd := m.chatMenu.Update(msg)
		m.chatMenu = newCM
		return m, cmd
	}

	keyStr := msg.String()
	if m.verbose {
		m.statusBar.SetLastKey(keyStr)
	}

	if m.searchModel != nil {
		if keyStr == "ctrl+v" {
			return m, readClipboardCmd()
		}
		newSearch, cmd := m.searchModel.Update(msg)
		m.searchModel = newSearch
		return m, cmd
	}

	// In insert mode, bypass global bindings and pass key directly to chat/composer
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeInsert {
		if keyStr == "esc" {
			m.vimState.Mode = keys.ModeNormal
			m.statusBar.SetMode(keys.ModeNormal)
			newPane, cmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
			m.chat = newPane.(*screens.ChatModel)
			return m, cmd
		}
		if keyStr == "ctrl+v" {
			return m, readClipboardCmd()
		}
		newPane, cmd := m.chat.Update(msg)
		m.chat = newPane.(*screens.ChatModel)
		return m, tea.Batch(cmd, m.markReadCmd())
	}

	// Global bindings always take priority
	switch m.keyMap.Resolve(keys.ContextGlobal, keyStr) {
	case keys.ActionFocusChatList:
		return m.focusPane(FocusChatList)
	case keys.ActionFocusChat:
		return m.focusPane(FocusChat)
	case keys.ActionFocusPrev:
		return m.focusPrev()
	case keys.ActionFocusNext:
		return m.focusNext()
	case keys.ActionFocusFolders:
		if m.folderBar != nil && m.folderBar.HasFolders() {
			return m.focusPane(FocusFolders)
		}
		return m, nil
	case keys.ActionQuit:
		return m, tea.Quit
	}

	if keyStr == "/" {
		if m.st != nil {
			m.searchModel = screens.NewSearchModel(m.st.Chats(), m.width, m.height, m.keyMap)
		}
		return m, nil
	}

	if m.focus == FocusFolders {
		action, res := m.matcher.Resolve(keys.ContextFolders, keyStr)
		if res == keys.MatchPending {
			return m, nil
		}
		if action != keys.ActionNone {
			newPane, cmd := m.folderBar.Update(keys.ActionMsg{Action: action})
			m.folderBar = newPane.(*screens.FoldersModel)
			return m, cmd
		}
		return m, nil
	}

	if m.focus == FocusChatList {
		action, res := m.matcher.Resolve(keys.ContextChatList, keyStr)
		if res == keys.MatchPending {
			return m, nil
		}
		if action == keys.ActionOpenContextMenu {
			if chat, ok := m.chatList.CursorChat(); ok && m.st != nil {
				m.chatMenu = components.NewChatContextMenu(chat, m.st.FolderFilters(), m.keyMap)
			}
			return m, nil
		}
		if action != keys.ActionNone {
			newPane, cmd := m.chatList.Update(keys.ActionMsg{Action: action})
			m.chatList = newPane.(*screens.ChatListModel)
			return m, cmd
		}
		return m, nil
	}

	// Chat pane: resolve through the matcher (supports chords).
	action, res := m.matcher.Resolve(keys.ContextChat, keyStr)
	if res == keys.MatchPending {
		return m, nil
	}
	// Mode is a consequence of the resolved action.
	switch action {
	case keys.ActionInsert:
		m.vimState.Mode = keys.ModeInsert
	case keys.ActionNormal:
		m.vimState.Mode = keys.ModeNormal
	}
	m.statusBar.SetMode(m.vimState.Mode)

	// Esc in normal mode: close active chat and return to chatlist.
	if action == keys.ActionNormal && m.focus == FocusChat {
		m.chat.ClearPendingAction()
		m.chat.SetChat(nil)
		m.chat.SetMessages(nil)
		m.currentChatID = 0
		m.chatList.SetActiveByID(0)
		return m.focusPane(FocusChatList)
	}

	if action == keys.ActionOpenInViewer && m.focus == FocusChat {
		photoID := m.chat.SelectedMessagePhotoID()
		if photoID != 0 {
			img := m.fullImageCache[photoID]
			if img == nil {
				img = m.imageCache[photoID]
			}
			if img != nil {
				go openInViewer(img, m.tmpDir)
			}
			return m, nil
		}
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			return m, openDocumentCmd(m.ctx, m.tgClient, m.currentPeer(), m.chat.SelectedMessageID(), ref, m.tmpDir)
		}
		return m, nil
	}

	if action == keys.ActionPlayVoice && m.focus == FocusChat {
		return m.handlePlayVoice()
	}

	if action == keys.ActionOpenContextMenu && m.focus == FocusChat {
		if m.chat != nil {
			msgID := m.chat.SelectedMessageID()
			isOut := m.chat.SelectedMessageIsOut()
			if msgID != 0 {
				replyToMsgID := m.chat.SelectedMessageReplyToMsgID()
				photoID := m.chat.SelectedMessagePhotoID()
				_, hasVideo := m.chat.SelectedMessageVideo()
				_, hasVoice := m.chat.SelectedMessageVoice()
				m.contextMenu = components.NewContextMenu(msgID, isOut, replyToMsgID, photoID, hasVideo, hasVoice, m.keyMap)
			}
		}
		return m, nil
	}

	if action == keys.ActionReply && m.focus == FocusChat {
		if m.chat != nil {
			return m, m.activateReply(m.chat.SelectedMessageID())
		}
		return m, nil
	}

	if action == keys.ActionEdit && m.focus == FocusChat {
		if m.chat != nil && m.chat.SelectedMessageIsOut() {
			return m, m.activateEdit(m.chat.SelectedMessageID())
		}
		return m, nil
	}

	if action != keys.ActionNone {
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: action})
		m.chat = newPane.(*screens.ChatModel)
		return m, tea.Batch(cmd, m.markReadCmd())
	}

	return m, nil
}

// writeTempMediaFile writes data to a private (0600) temp file in tmpDir with
// the given extension and returns its path.
func writeTempMediaFile(data []byte, tmpDir, ext string) (string, error) {
	f, err := os.CreateTemp(tmpDir, "tele-media-*"+ext)
	if err != nil {
		return "", err
	}
	name := f.Name()
	_ = os.Chmod(name, 0600)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// openPath hands a file to the OS default application.
func openPath(name string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", name)
	default:
		cmd = exec.Command("xdg-open", name)
	}
	_ = cmd.Start()
}

func openInViewer(img image.Image, tmpDir string) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return
	}
	name, err := writeTempMediaFile(buf.Bytes(), tmpDir, ".jpg")
	if err != nil {
		return
	}
	openPath(name)
}

func (m RootModel) handleStatusErr(msg StatusErrMsg) (RootModel, tea.Cmd) {
	serial := m.statusBar.SetError(msg.Text, msg.Sev)
	d := durationFor(msg.Sev)
	return m, tea.Tick(d, func(time.Time) tea.Msg { return ClearStatusErrMsg{Serial: serial} })
}

func (m RootModel) handleChatLoadErr(msg chatLoadErrMsg) (RootModel, tea.Cmd) {
	if msg.chatID == m.currentChatID {
		m.chat.SetLoading(false)
		m.chat.SetLoadError(msg.text)
	}
	serial := m.statusBar.SetError(msg.text, components.SeverityError)
	return m, tea.Tick(durationFor(components.SeverityError), func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}

// downloadWithRefresh runs download(ref); on a FILE_REFERENCE_EXPIRED error it
// refreshes the message's media refs once via RefreshMessage and retries with the
// fresh ref. On a successful retry it returns the refreshed message so the caller
// can persist the new ref.
func downloadWithRefresh[T any, R any](
	ctx context.Context,
	client internaltg.Client,
	peer store.Peer,
	msgID int,
	ref R,
	download func(R) (T, error),
	pickRef func(store.Message) (R, bool),
) (result T, refreshed *store.Message, err error) {
	result, err = download(ref)
	if err == nil {
		return result, nil, nil
	}
	if !internaltg.IsFileReferenceExpired(err) {
		return result, nil, err
	}
	msg, rerr := client.RefreshMessage(ctx, peer, msgID)
	if rerr != nil {
		return result, nil, err
	}
	newRef, ok := pickRef(msg)
	if !ok {
		return result, nil, err
	}
	result, err = download(newRef)
	if err != nil {
		return result, nil, err
	}
	return result, &msg, nil
}

func downloadPhotoCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.PhotoRef) (image.Image, error) {
				return client.DownloadPhoto(ctx, r)
			},
			func(m store.Message) (store.PhotoRef, bool) {
				if m.Photo == nil {
					return store.PhotoRef{}, false
				}
				return *m.Photo, true
			},
		)
		if err != nil {
			return StatusErrMsg{Text: "photo download failed: " + err.Error(), Sev: components.SeverityWarning}
		}
		ready := PhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, photo: refreshed.Photo})
		}
		return ready
	}
}

// DownloadPhotoCmdForTest exposes downloadPhotoCmd for tests.
func DownloadPhotoCmdForTest(c internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	return downloadPhotoCmd(context.Background(), c, peer, msgID, ref)
}

// HistoryChunkMsgForTest builds a historyChunkMsg for tests.
func HistoryChunkMsgForTest(chatID int64, msgs []store.Message) tea.Msg {
	return historyChunkMsg{chatID: chatID, messages: msgs}
}

// refreshedBatch emits both the ready image and the store-update message after a
// successful refresh+retry.
func refreshedBatch(ready, refreshed tea.Msg) tea.Msg {
	return tea.BatchMsg{
		func() tea.Msg { return ready },
		func() tea.Msg { return refreshed },
	}
}

// currentPeer returns the peer of the currently open chat, or the zero peer.
func (m RootModel) currentPeer() store.Peer {
	if m.st != nil {
		if chat, ok := m.st.GetChat(m.currentChatID); ok {
			return chat.Peer
		}
	}
	return store.Peer{}
}

// openDocumentCmd downloads a document in full and opens it in the OS default
// application (e.g. a video player). Runs async; the download may be large.
func openDocumentCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, tmpDir string) tea.Cmd {
	return func() tea.Msg {
		data, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) ([]byte, error) {
				return client.DownloadDocument(ctx, r)
			},
			pickDocumentRef,
		)
		if err != nil {
			return StatusErrMsg{Text: "open file failed: " + err.Error(), Sev: components.SeverityWarning}
		}
		if len(data) == 0 {
			return nil
		}
		ext := filepath.Ext(ref.FileName)
		if ext == "" {
			ext = extFromMime(ref.MimeType)
		}
		name, werr := writeTempMediaFile(data, tmpDir, ext)
		if werr != nil {
			return StatusErrMsg{Text: "open file failed: " + werr.Error(), Sev: components.SeverityWarning}
		}
		openPath(name)
		if refreshed != nil {
			return mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document}
		}
		return nil
	}
}

// pickDocumentRef extracts a message's fresh document ref, used as the refresh
// selector for document downloads.
func pickDocumentRef(m store.Message) (store.DocumentRef, bool) {
	if m.Document == nil {
		return store.DocumentRef{}, false
	}
	return *m.Document, true
}

// extFromMime maps common video MIME types to a file extension so the OS picks
// the right player. Defaults to .mp4 (the usual Telegram video container).
func extFromMime(mime string) string {
	switch mime {
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/x-matroska":
		return ".mkv"
	default:
		return ".mp4"
	}
}

// handlePlayVoice toggles or starts in-app playback of the selected voice
// message. Degrades silently when no audio device is available.
func (m RootModel) handlePlayVoice() (RootModel, tea.Cmd) {
	ref, ok := m.chat.SelectedMessageVoice()
	if !ok {
		return m, nil
	}
	if m.voicePlayer == nil {
		pl, err := audio.NewPlayer()
		if err != nil {
			return m, nil // no audio device
		}
		m.voicePlayer = pl
	}
	if m.voicePlayer.Toggle(ref.ID) {
		return m, nil // same message: paused/resumed
	}
	return m, downloadVoiceCmd(m.ctx, m.tgClient, m.currentPeer(), m.chat.SelectedMessageID(), ref)
}

func downloadVoiceCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef) tea.Cmd {
	return func() tea.Msg {
		data, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) ([]byte, error) {
				return client.DownloadDocument(ctx, r)
			},
			pickDocumentRef,
		)
		if err != nil {
			return StatusErrMsg{Text: "voice download failed: " + err.Error(), Sev: components.SeverityWarning}
		}
		if len(data) == 0 {
			return nil
		}
		ready := voicePlayReadyMsg{docID: ref.ID, data: data}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document})
		}
		return ready
	}
}

func voiceTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return voiceTickMsg{} })
}

func downloadVideoThumbCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.DocumentRef, crop bool) tea.Cmd {
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, ref,
			func(r store.DocumentRef) (image.Image, error) {
				return client.DownloadDocumentThumb(ctx, r)
			},
			func(m store.Message) (store.DocumentRef, bool) {
				if m.Document == nil {
					return store.DocumentRef{}, false
				}
				return *m.Document, true
			},
		)
		if err != nil || img == nil {
			if err != nil {
				return StatusErrMsg{Text: "video thumb download failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return nil
		}
		if crop {
			img = media.CircleCrop(img) // round video note → circle
		}
		// Reuse the photo-ready path; the cache is keyed by id (here the document id).
		ready := PhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, doc: refreshed.Document})
		}
		return ready
	}
}

func downloadFullPhotoCmd(ctx context.Context, client internaltg.Client, peer store.Peer, msgID int, ref store.PhotoRef) tea.Cmd {
	fullRef := ref
	fullRef.ThumbSize = ref.FullThumbSize
	return func() tea.Msg {
		img, refreshed, err := downloadWithRefresh(ctx, client, peer, msgID, fullRef,
			func(r store.PhotoRef) (image.Image, error) {
				return client.DownloadPhoto(ctx, r)
			},
			func(m store.Message) (store.PhotoRef, bool) {
				if m.Photo == nil {
					return store.PhotoRef{}, false
				}
				r := *m.Photo
				r.ThumbSize = r.FullThumbSize
				return r, true
			},
		)
		if err != nil || img == nil {
			if err != nil {
				return StatusErrMsg{Text: "full photo download failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return nil
		}
		ready := FullPhotoReadyMsg{PhotoID: ref.ID, Image: img}
		if refreshed != nil {
			return refreshedBatch(ready, mediaRefRefreshedMsg{chatID: peer.ID, msgID: msgID, photo: refreshed.Photo})
		}
		return ready
	}
}

func (m RootModel) pendingDownloadCmds(msgs []store.Message) tea.Cmd {
	var cmds []tea.Cmd
	for _, msg := range msgs {
		var peer store.Peer
		if m.st != nil {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				peer = chat.Peer
			}
		}
		if msg.Photo != nil {
			if _, ok := m.imageCache[msg.Photo.ID]; !ok {
				cmds = append(cmds, downloadPhotoCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Photo))
			}
			if m.cfg != nil && m.cfg.Photos.EagerFullQuality && msg.Photo.FullThumbSize != "" {
				if _, ok := m.fullImageCache[msg.Photo.ID]; !ok {
					cmds = append(cmds, downloadFullPhotoCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Photo))
				}
			}
		}
		// Video thumbnails reuse the inline-image cache, keyed by document id.
		if msg.Media != nil && msg.Media.Kind.IsVideo() && msg.Document != nil && msg.Document.ThumbSize != "" {
			if _, ok := m.imageCache[msg.Document.ID]; !ok {
				// Round video notes are cropped to a circle, but only in Kitty mode
				// (PNG alpha); block-art has no transparency, so keep it square there.
				crop := msg.Media.Kind == store.MediaVideoNote && m.imageMode == media.ModeKitty
				cmds = append(cmds, downloadVideoThumbCmd(m.ctx, m.tgClient, peer, msg.ID, *msg.Document, crop))
			}
		}
	}
	return tea.Batch(cmds...)
}

// transmitPhotoCmd transmits one photo to the terminal at the chat's current
// photo width and creates its virtual placement. No-op unless in Kitty mode.
func (m RootModel) transmitPhotoCmd(photoID int64, img image.Image) tea.Cmd {
	if m.imageMode != media.ModeKitty || img == nil {
		return nil
	}
	id := m.kittyStore.IDFor(photoID)
	b := img.Bounds()
	cols, rows := m.chat.PhotoBox(b.Dx(), b.Dy())
	// Order matters: write the placement to the terminal FIRST, then mark the
	// image ready (kittyTransmittedMsg) so the next render emits placeholders
	// only once the placement exists. Marking ready before the transmit lands
	// races the repaint and intermittently leaves the photo mispositioned.
	return tea.Sequence(
		func() tea.Msg {
			seq, err := media.TransmitSeq(id, img, cols, rows)
			if err != nil {
				return nil
			}
			return tea.Raw(seq)()
		},
		func() tea.Msg {
			return kittyTransmittedMsg{photoID: photoID, cols: cols}
		},
	)
}

// retransmitDebounce is the quiet period after the last photo-width change
// before images are re-transmitted. A resize drag fires many WindowSizeMsgs in
// quick succession; debouncing collapses them into a single retransmit at the
// final width. Without it, overlapping async transmits land out of order and
// leave the Kitty placement at a stale size (photo renders smaller than grid).
const retransmitDebounce = 90 * time.Millisecond

// retransmitOnColsChange schedules a debounced retransmit when the photo content
// width (in cells) actually changed. Photo width is photoContentCols (chat-pane,
// capped), not the window width, so this fires on any layout change that affects
// it (window resize, folder bar show/hide) and skips changes that leave the
// column count unchanged. Only the latest scheduled tick performs the work.
func (m *RootModel) retransmitOnColsChange() tea.Cmd {
	cols := m.chat.PhotoContentCols()
	// A tall photo's effective width depends on the pane height (the 2/3-viewport
	// and 480px height caps shrink cols), so a height-only resize can change a
	// photo's box without changing photoContentCols. Track both.
	paneH := m.chat.PhotoViewHeight()
	if cols == m.lastPhotoCols && paneH == m.lastPaneHeight {
		return nil
	}
	m.lastPhotoCols = cols
	m.lastPaneHeight = paneH
	m.retransmitGen++
	gen := m.retransmitGen
	return tea.Tick(retransmitDebounce, func(time.Time) tea.Msg {
		return retransmitTickMsg{gen: gen}
	})
}

// requestKittyReset asks the next reconcile to delete every placement and
// re-transmit the now-visible images. Used on chat switch and photo-width change
// (the deleted images belong to a different chat or a stale cell width).
func (m *RootModel) requestKittyReset() {
	if m.imageMode == media.ModeKitty {
		m.kittyResetPending = true
	}
}

// defaultKittyPlacementCap is the fallback cap when photos.kitty_placement_cap
// is unset (or non-positive). See PhotosConfig.KittyPlacementCap.
const defaultKittyPlacementCap = 16

// reconcileKittyCmd is the single place that issues Kitty transmits and deletes.
// It transmits visible images that are not yet live and evicts the
// least-recently-visible placements beyond the cap. No-op outside Kitty mode or
// the main screen.
func (m *RootModel) reconcileKittyCmd() tea.Cmd {
	if m.imageMode != media.ModeKitty || m.screen != ScreenMain {
		return nil
	}

	var pre tea.Cmd
	if m.kittyResetPending {
		m.kittyResetPending = false
		m.kittyStore.Clear()
		m.kittyLive = make(map[int64]bool)
		m.kittyLRU = nil
		pre = func() tea.Msg { return tea.Raw(media.DeleteAllSeq())() }
	}

	visible := m.chat.VisiblePhotoIDs()
	visSet := make(map[int64]bool, len(visible))
	for _, id := range visible {
		visSet[id] = true
	}

	var cmds []tea.Cmd
	for _, id := range visible {
		if m.kittyLive[id] {
			m.kittyLRU = touchID(m.kittyLRU, id)
			continue
		}
		if img, ok := m.imageCache[id]; ok {
			if c := m.transmitPhotoCmd(id, img); c != nil {
				cmds = append(cmds, c)
				m.kittyLive[id] = true
				m.kittyLRU = append(m.kittyLRU, id)
			}
		}
	}

	// Evict the least-recently-visible placements beyond the cap; never evict a
	// currently-visible image.
	capN := m.kittyCap
	if capN <= 0 {
		capN = defaultKittyPlacementCap
	}
	for len(m.kittyLive) > capN {
		evicted := false
		for i, id := range m.kittyLRU {
			if visSet[id] {
				continue
			}
			cmds = append(cmds, tea.Raw(media.DeleteSeq(m.kittyStore.IDFor(id))))
			m.kittyStore.Untransmit(id)
			delete(m.kittyLive, id)
			m.kittyLRU = append(m.kittyLRU[:i], m.kittyLRU[i+1:]...)
			evicted = true
			break
		}
		if !evicted {
			break
		}
	}

	body := tea.Batch(cmds...)
	switch {
	case pre != nil && len(cmds) > 0:
		// The delete-all must reach the terminal before the re-transmits, so
		// sequence them; the transmits/deletes among themselves target distinct
		// ids and can run concurrently.
		return tea.Sequence(pre, body)
	case pre != nil:
		return pre
	case len(cmds) > 0:
		return body
	default:
		return nil
	}
}

// touchID moves id to the most-recently-visible end of the LRU order.
func touchID(s []int64, id int64) []int64 {
	out := make([]int64, 0, len(s))
	for _, v := range s {
		if v != id {
			out = append(out, v)
		}
	}
	return append(out, id)
}

func (m RootModel) markReadCmd() tea.Cmd {
	if m.st == nil || m.tgClient == nil || m.currentChatID == 0 || m.focus != FocusChat {
		return nil
	}
	chat, ok := m.st.GetChat(m.currentChatID)
	if !ok {
		return nil
	}
	maxID := m.chat.VisibleReadMaxID()
	if maxID <= 0 || maxID <= chat.ReadInboxMaxID {
		return nil
	}
	ctx := m.ctx
	client := m.tgClient
	peer := chat.Peer
	chatID := chat.ID
	return func() tea.Msg {
		if err := client.MarkRead(ctx, peer, maxID); err != nil {
			return StatusErrMsg{Text: "mark read failed: " + err.Error(), Sev: components.SeverityInfo}
		}
		return markReadDoneMsg{chatID: chatID, maxID: maxID}
	}
}

// activateReply sets reply state for msgID, switches to insert mode, and returns the FocusComposer cmd.
// Returns nil if msgID is zero.
func (m *RootModel) activateReply(msgID int) tea.Cmd {
	if msgID == 0 {
		return nil
	}
	preview := "▌ Reply to message"
	if m.st != nil {
		for _, storeMsg := range m.st.Messages(m.currentChatID) {
			if storeMsg.ID == msgID {
				preview = components.BuildReplyPreview(storeMsg)
				break
			}
		}
	}
	m.chat.SetReply(msgID, preview)
	m.vimState.Mode = keys.ModeInsert
	m.statusBar.SetMode(keys.ModeInsert)
	return m.chat.FocusComposer()
}

// activateEdit sets edit state for msgID, pre-fills the composer with the
// original text, switches to insert mode, and returns the FocusComposer cmd.
// Returns nil if msgID is zero or the message is not found in the store.
func (m *RootModel) activateEdit(msgID int) tea.Cmd {
	if msgID == 0 {
		return nil
	}
	if m.st == nil {
		return nil
	}
	for _, storeMsg := range m.st.Messages(m.currentChatID) {
		if storeMsg.ID == msgID {
			preview := components.BuildEditPreview(storeMsg)
			m.chat.SetEdit(msgID, preview)
			m.chat.SetComposerValue(storeMsg.Text)
			m.vimState.Mode = keys.ModeInsert
			m.statusBar.SetMode(keys.ModeInsert)
			return m.chat.FocusComposer()
		}
	}
	return nil
}

func (m RootModel) focusPrev() (tea.Model, tea.Cmd) {
	hasFolders := m.folderBar != nil && m.folderBar.HasFolders()
	switch m.focus {
	case FocusChat:
		return m.focusPane(FocusChatList)
	case FocusChatList:
		if hasFolders {
			return m.focusPane(FocusFolders)
		}
		return m, nil
	case FocusFolders:
		return m.focusPane(FocusChat)
	}
	return m, nil
}

func (m RootModel) focusNext() (tea.Model, tea.Cmd) {
	hasFolders := m.folderBar != nil && m.folderBar.HasFolders()
	switch m.focus {
	case FocusFolders:
		return m.focusPane(FocusChatList)
	case FocusChatList:
		return m.focusPane(FocusChat)
	case FocusChat:
		if hasFolders {
			return m.focusPane(FocusFolders)
		}
		return m, nil
	}
	return m, nil
}

func (m RootModel) focusPane(target Focus) (tea.Model, tea.Cmd) {
	if target == m.focus {
		return m, nil
	}
	m.matcher.Reset()
	// Exit insert mode when leaving chat
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeInsert {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
		newPane, _ := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
		m.chat = newPane.(*screens.ChatModel)
	}
	m.focus = target
	m.chatList.SetFocused(target == FocusChatList)
	m.chat.SetFocused(target == FocusChat)
	if m.folderBar != nil {
		m.folderBar.SetFocused(target == FocusFolders)
	}
	switch target {
	case FocusFolders:
		m.statusBar.SetActivePane("folders")
	case FocusChatList:
		m.statusBar.SetActivePane("chatlist")
	case FocusChat:
		m.statusBar.SetActivePane("chat")
	}
	return m, nil
}

func (m RootModel) View() tea.View {
	var content string
	if m.screen == ScreenLogin {
		logoView := m.logo.View()
		if m.login.CurrentStep() < 0 {
			combined := lipgloss.JoinVertical(lipgloss.Center, logoView, "\n"+"connecting...")
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
		} else {
			loginContent := m.login.View().Content
			b := lipgloss.RoundedBorder()
			loginLines := strings.Split(loginContent, "\n")
			loginContentH := len(loginLines)
			loginContentW := 0
			for _, l := range loginLines {
				if w := lipgloss.Width(l); w > loginContentW {
					loginContentW = w
				}
			}
			const loginPadV, loginPadH = 1, 3
			innerW := loginContentW + 2*loginPadH
			innerH := loginContentH + 2*loginPadV
			padded := lipgloss.NewStyle().Padding(loginPadV, loginPadH).Render(loginContent)
			loginBox := components.RenderBox(padded, "Telegram", "", "", b, nil, innerW+2, innerH+2)
			combined := lipgloss.JoinVertical(lipgloss.Center, logoView, "\n", loginBox)
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
		}
	} else {
		paneH := m.height + 1
		innerH := paneH - 2*borderSize

		activeBorder := lipgloss.DoubleBorder()
		inactiveBorder := lipgloss.NormalBorder()

		lightDark := lipgloss.LightDark(m.hasDarkBackground)
		activeFg := lightDark(lipgloss.Color("22"), lipgloss.Color("10"))

		foldersBorder := inactiveBorder
		chatListBorder := inactiveBorder
		chatBorder := inactiveBorder
		var foldersFg, chatListFg, chatFg color.Color
		switch m.focus {
		case FocusFolders:
			foldersBorder = activeBorder
			foldersFg = activeFg
		case FocusChatList:
			chatListBorder = activeBorder
			chatListFg = activeFg
		case FocusChat:
			chatBorder = activeBorder
			chatFg = activeFg
		}

		chatListTitle := "[1] Chats"
		chatTitle := "[2] " + m.chat.Title()
		chatDot := ""
		if m.chat.IsTyping() {
			chatDot = m.chat.TypingLabel()
		} else if m.currentChatID != 0 && m.st != nil {
			if chat, ok := m.st.GetChat(m.currentChatID); ok && chat.Peer.IsUser() && chat.Online {
				chatDot = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("●")
			}
		}

		var main string
		var chatPanelLeft, chatBoxW int
		var chatListLeft, chatListBoxW int
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
			foldersSB := &components.Scrollbar{Info: m.folderBar.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatListSB := &components.Scrollbar{Info: m.chatList.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatSB := &components.Scrollbar{Info: m.chat.ScrollInfo(), TrackTop: 0, TrackLen: m.chat.MessageListHeight()}
			foldersView := components.RenderBox(m.folderBar.View(), "[0] Folders", "", "", foldersBorder, foldersFg, sidebarW, innerH, foldersSB)
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatlistW, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatW, innerH, chatSB)
			main = lipgloss.JoinHorizontal(lipgloss.Top, foldersView, chatListView, chatView)
			chatPanelLeft = sidebarW + chatlistW
			chatBoxW = chatW
			chatListLeft = sidebarW
			chatListBoxW = chatlistW
		} else {
			leftW, rightW := layout.SplitHorizontal(m.width, m.height, 0.30)
			chatListWidth := leftW - 2*borderSize + 2
			chatWidth := rightW - 2*borderSize + 2
			chatListSB := &components.Scrollbar{Info: m.chatList.ScrollInfo(), TrackTop: 0, TrackLen: innerH}
			chatSB := &components.Scrollbar{Info: m.chat.ScrollInfo(), TrackTop: 0, TrackLen: m.chat.MessageListHeight()}
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatListWidth, innerH, chatListSB)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatWidth, innerH, chatSB)
			main = lipgloss.JoinHorizontal(lipgloss.Top, chatListView, chatView)
			chatPanelLeft = chatListWidth
			chatBoxW = chatWidth
			chatListLeft = 0
			chatListBoxW = chatListWidth
		}

		content = main + "\n" + m.statusBar.View()
		if m.searchModel != nil {
			content = overlayCenter(content, m.searchModel.View(), m.width, m.height)
		}
		if m.contextMenu != nil {
			content = m.overlayMenuNearBubble(content, m.contextMenu.View(), chatPanelLeft, chatBoxW)
		}
		if m.chatMenu != nil {
			content = m.overlayMenuNearChatRow(content, m.chatMenu.View(), chatListLeft, chatListBoxW)
		}
		if m.reactionPicker != nil {
			content = m.overlayMenuNearBubble(content, m.reactionPicker.View(), chatPanelLeft, chatBoxW)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// overlayMenuNearBubble places a menu next to the selected message bubble: left
// of outgoing bubbles, right of incoming, top-aligned, clamped to the chat
// panel. If the bubble geometry is unavailable (no selection, scrolled out,
// empty chat) it falls back to the bottom-right corner.
func (m RootModel) overlayMenuNearBubble(content, menu string, chatPanelLeft, chatBoxW int) string {
	rect, ok := m.chat.SelectedBubbleRect()
	if !ok {
		return overlayBottomRight(content, menu, m.width, m.height, m.chat.ComposerHeight()+1)
	}

	// rect is local to the message list's output. The chat box sits at terminal
	// row 0; RenderBox adds a 1-cell top/left border; the message list is at the
	// top of the chat content, so no extra vertical offset is needed.
	bubble := components.Rect{
		Top:    1 + rect.Top,
		Left:   chatPanelLeft + 1 + rect.Left,
		Height: rect.Height,
		Width:  rect.Width,
	}
	area := components.Rect{
		Top:    1,
		Left:   chatPanelLeft + 1,
		Height: m.chat.MessageListHeight(),
		Width:  chatBoxW - 2,
	}

	menuW, menuH := measureBox(menu)
	top, left := anchorMenu(bubble, area, menuW, menuH, m.chat.SelectedMessageIsOut())
	return overlayAt(content, menu, m.width, m.height, top, left)
}

// overlayMenuNearChatRow places a menu to the right of the selected
// chat-list row, top-aligned to that row and clamped to the main content
// area so it stays on screen.
func (m RootModel) overlayMenuNearChatRow(content, menu string, chatListLeft, chatListBoxW int) string {
	row := m.chatList.CursorViewportRow()
	// The chat-list box sits at terminal row 0; RenderBox adds a 1-cell
	// top/left border, so the first row of content is terminal row 1.
	rowRect := components.Rect{
		Top:    1 + row,
		Left:   chatListLeft,
		Height: 1,
		Width:  chatListBoxW,
	}
	area := components.Rect{
		Top:    1,
		Left:   chatListLeft,
		Height: m.chatList.Height(),
		Width:  m.width - chatListLeft,
	}
	menuW, menuH := measureBox(menu)
	// onLeft=false anchors to the right of the row (into the chat pane).
	top, left := anchorMenu(rowRect, area, menuW, menuH, false)
	return overlayAt(content, menu, m.width, m.height, top, left)
}

func logoTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return components.LogoTickMsg{}
	})
}

func requestBGColorCmd() tea.Cmd {
	return func() tea.Msg { return tea.RequestBackgroundColor() }
}

func bgColorPollCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tea.RequestBackgroundColor()
	})
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return components.SpinnerTickMsg{}
	})
}

func typingDotsTickCmd() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return components.TypingDotsTickMsg{}
	})
}

func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		str, err := clipboard.ReadAll()
		if err != nil || str == "" {
			return nil
		}
		return tea.PasteMsg{Content: str}
	}
}
