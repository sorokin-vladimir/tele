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
}

type FolderFiltersMsg struct {
	Filters []store.FolderFilter
}

type clearTypingMsg struct{ serial int }

type RootModel struct {
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
	historyLimit      int
	verbose           bool
	cfg               *config.Config
	imageCache        map[int64]image.Image
	fullImageCache    map[int64]image.Image
	imageMode         media.Mode
	kittyStore        *media.KittyStore
	lastPhotoCols     int
	retransmitGen     int
	searchModel       *screens.SearchModel
	onChatOpen        func(int64)
	nextSentinel      int
	contextMenu       *components.ContextMenu
	reactionPicker    *components.ReactionPicker
	reactionTargetID  int
	folderBar         *screens.FoldersModel
	activeFilter      *store.FolderFilter
	logo              components.LogoLoader
	typingSerial      int
	tmpDir            string
}

func NewRootModel(client internaltg.Client, st store.Store, historyLimit int, verbose bool) RootModel {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(80)
	sb.SetKeyMap(km)
	cl := screens.NewChatListModel()
	cl.SetFocused(true)
	return RootModel{
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

func (m RootModel) WithConfig(cfg *config.Config) RootModel {
	m.cfg = cfg
	m.imageMode = media.DetectMode(cfg.Photos.Mode, os.Getenv)
	if m.imageMode == media.ModeKitty {
		m.chat.SetRenderer(media.NewKittyRenderer(m.kittyStore))
	}
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
	if m.activeFilter == nil {
		return all
	}
	out := make([]store.Chat, 0, len(all))
	for _, c := range all {
		if m.activeFilter.Matches(c) {
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
		if f.ID == 0 {
			continue
		}
		chatsWithUnread := 0
		for _, c := range chats {
			if f.Matches(c) && c.UnreadCount > 0 {
				chatsWithUnread++
			}
		}
		counts[f.ID] = chatsWithUnread
	}
	return counts
}

func (m RootModel) Init() tea.Cmd {
	m.statusBar.SetVerbose(m.verbose)
	m.statusBar.SetActivePane("chatlist")
	return tea.Batch(logoTickCmd(), requestBGColorCmd())
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	// network/data messages
	case screens.OpenChatMsg,
		ChatHistoryMsg,
		screens.LoadMoreMsg,
		historyChunkMsg,
		markReadDoneMsg,
		PhotoReadyMsg,
		FullPhotoReadyMsg,
		kittyTransmittedMsg,
		components.OpenInViewerRequest:
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
			return m, openDocumentCmd(m.tgClient, ref, m.tmpDir)
		}
		return m, nil
	}

	if action == keys.ActionOpenContextMenu && m.focus == FocusChat {
		if m.chat != nil {
			msgID := m.chat.SelectedMessageID()
			isOut := m.chat.SelectedMessageIsOut()
			if msgID != 0 {
				replyToMsgID := m.chat.SelectedMessageReplyToMsgID()
				photoID := m.chat.SelectedMessagePhotoID()
				_, hasVideo := m.chat.SelectedMessageVideo()
				m.contextMenu = components.NewContextMenu(msgID, isOut, replyToMsgID, photoID, hasVideo, m.keyMap)
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

func downloadPhotoCmd(client internaltg.Client, ref store.PhotoRef) tea.Cmd {
	return func() tea.Msg {
		img, err := client.DownloadPhoto(context.Background(), ref)
		if err != nil {
			return nil
		}
		return PhotoReadyMsg{PhotoID: ref.ID, Image: img}
	}
}

// openDocumentCmd downloads a document in full and opens it in the OS default
// application (e.g. a video player). Runs async; the download may be large.
func openDocumentCmd(client internaltg.Client, ref store.DocumentRef, tmpDir string) tea.Cmd {
	return func() tea.Msg {
		data, err := client.DownloadDocument(context.Background(), ref)
		if err != nil || len(data) == 0 {
			return nil
		}
		ext := filepath.Ext(ref.FileName)
		if ext == "" {
			ext = extFromMime(ref.MimeType)
		}
		name, err := writeTempMediaFile(data, tmpDir, ext)
		if err != nil {
			return nil
		}
		openPath(name)
		return nil
	}
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

func downloadVideoThumbCmd(client internaltg.Client, ref store.DocumentRef) tea.Cmd {
	return func() tea.Msg {
		img, err := client.DownloadDocumentThumb(context.Background(), ref)
		if err != nil || img == nil {
			return nil
		}
		// Reuse the photo-ready path; the cache is keyed by id (here the document id).
		return PhotoReadyMsg{PhotoID: ref.ID, Image: img}
	}
}

func downloadFullPhotoCmd(client internaltg.Client, ref store.PhotoRef) tea.Cmd {
	fullRef := ref
	fullRef.ThumbSize = ref.FullThumbSize
	return func() tea.Msg {
		img, err := client.DownloadPhoto(context.Background(), fullRef)
		if err != nil || img == nil {
			return nil
		}
		return FullPhotoReadyMsg{PhotoID: ref.ID, Image: img}
	}
}

func (m RootModel) pendingDownloadCmds(msgs []store.Message) tea.Cmd {
	var cmds []tea.Cmd
	for _, msg := range msgs {
		if msg.Photo != nil {
			if _, ok := m.imageCache[msg.Photo.ID]; !ok {
				cmds = append(cmds, downloadPhotoCmd(m.tgClient, *msg.Photo))
			}
			if m.cfg != nil && m.cfg.Photos.EagerFullQuality && msg.Photo.FullThumbSize != "" {
				if _, ok := m.fullImageCache[msg.Photo.ID]; !ok {
					cmds = append(cmds, downloadFullPhotoCmd(m.tgClient, *msg.Photo))
				}
			}
		}
		// Video thumbnails reuse the inline-image cache, keyed by document id.
		if msg.Media != nil && msg.Media.Kind == store.MediaVideo && msg.Document != nil && msg.Document.ThumbSize != "" {
			if _, ok := m.imageCache[msg.Document.ID]; !ok {
				cmds = append(cmds, downloadVideoThumbCmd(m.tgClient, *msg.Document))
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
	cols := m.chat.PhotoContentCols()
	id := m.kittyStore.IDFor(photoID)
	b := img.Bounds()
	rows := m.chat.PhotoFootprint(b.Dx(), b.Dy(), cols)
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
	if cols == m.lastPhotoCols {
		return nil
	}
	m.lastPhotoCols = cols
	m.retransmitGen++
	gen := m.retransmitGen
	return tea.Tick(retransmitDebounce, func(time.Time) tea.Msg {
		return retransmitTickMsg{gen: gen}
	})
}

// retransmitChatCmd deletes all terminal images and re-transmits the current
// chat's downloaded photos at the current width. No-op unless in Kitty mode.
func (m RootModel) retransmitChatCmd() tea.Cmd {
	if m.imageMode != media.ModeKitty {
		return nil
	}
	deleteAll := func() tea.Msg { return tea.Raw(media.DeleteAllSeq())() }
	m.kittyStore.Clear()
	var transmits []tea.Cmd
	if m.st != nil && m.currentChatID != 0 {
		for _, msg := range m.st.Messages(m.currentChatID) {
			id, ok := components.PreviewImageID(msg)
			if !ok {
				continue
			}
			if img, ok := m.imageCache[id]; ok {
				transmits = append(transmits, m.transmitPhotoCmd(id, img))
			}
		}
	}
	// Sequence, not Batch: the delete-all must reach the terminal before the
	// re-transmits. tea.Batch runs cmds concurrently with no ordering guarantee,
	// so a transmit could land first and then be wiped by the delete-all.
	return tea.Sequence(deleteAll, tea.Batch(transmits...))
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
	client := m.tgClient
	peer := chat.Peer
	chatID := chat.ID
	return func() tea.Msg {
		if err := client.MarkRead(context.Background(), peer, maxID); err != nil {
			return nil
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
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
			foldersView := components.RenderBox(m.folderBar.View(), "[0] Folders", "", "", foldersBorder, foldersFg, sidebarW, innerH)
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatlistW, innerH)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatW, innerH)
			main = lipgloss.JoinHorizontal(lipgloss.Top, foldersView, chatListView, chatView)
		} else {
			leftW, rightW := layout.SplitHorizontal(m.width, m.height, 0.30)
			chatListWidth := leftW - 2*borderSize + 2
			chatWidth := rightW - 2*borderSize + 2
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", "", chatListBorder, chatListFg, chatListWidth, innerH)
			chatView := components.RenderBox(m.chat.View(), chatTitle, chatDot, "", chatBorder, chatFg, chatWidth, innerH)
			main = lipgloss.JoinHorizontal(lipgloss.Top, chatListView, chatView)
		}

		content = main + "\n" + m.statusBar.View()
		if m.searchModel != nil {
			content = overlayCenter(content, m.searchModel.View(), m.width, m.height)
		}
		if m.contextMenu != nil {
			content = overlayBottomRight(content, m.contextMenu.View(), m.width, m.height, m.chat.ComposerHeight()+1)
		}
		if m.reactionPicker != nil {
			content = overlayBottomRight(content, m.reactionPicker.View(), m.width, m.height, m.chat.ComposerHeight()+1)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
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
