package ui

import (
	"context"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

type Screen int

const (
	ScreenLogin Screen = iota
	ScreenMain
)

type Focus int

const (
	FocusFolders  Focus = iota
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

type markReadDoneMsg struct {
	chatID int64
	maxID  int
}

type sentMsgConfirmedMsg struct {
	chatID     int64
	sentinelID int
	realID     int
}

type historyChunkMsg struct {
	chatID   int64
	messages []store.Message
}

type reactionFailedMsg struct {
	chatID    int64
	msgID     int
	reactions []store.Reaction
}

type FolderFiltersMsg struct {
	Filters []store.FolderFilter
}

type RootModel struct {
	screen        Screen
	focus         Focus
	width         int
	height        int
	chatList      *screens.ChatListModel
	chat          *screens.ChatModel
	login         screens.LoginModel
	statusBar     *components.StatusBar
	vimState      *keys.VimState
	keyMap        keys.KeyMap
	tgClient      internaltg.Client
	st            store.Store
	currentChatID int64
	historyLimit  int
	verbose       bool
	imageCache         map[int64]image.Image
	searchModel        *screens.SearchModel
	onChatOpen         func(int64)
	nextSentinel       int
	chatListPendingKey string
	contextMenu        *components.ContextMenu
	reactionPicker     *components.ReactionPicker
	reactionTargetID   int
	folderBar          *screens.FoldersModel
	activeFilter       *store.FolderFilter
	logo               components.LogoLoader
}

func NewRootModel(client internaltg.Client, st store.Store, historyLimit int, verbose bool) RootModel {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(80)
	sb.SetKeyMap(km)
	return RootModel{
		screen:       ScreenLogin,
		focus:        FocusChatList,
		chatList:     screens.NewChatListModel(),
		chat:         screens.NewChatModel(80, 24),
		folderBar:    screens.NewFoldersModel(),
		statusBar:    sb,
		vimState:     keys.NewVimState(),
		keyMap:       km,
		tgClient:     client,
		st:           st,
		historyLimit: historyLimit,
		verbose:      verbose,
		imageCache:   make(map[int64]image.Image),
		logo:         components.NewLogoLoader(80),
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

func (m RootModel) SearchActive() bool    { return m.searchModel != nil }
func (m RootModel) ContextMenuOpen() bool    { return m.contextMenu != nil }
func (m RootModel) ReactionPickerOpen() bool { return m.reactionPicker != nil }

// SetLoginModel injects the login model after NewRootModel (called by app.go).
func (m *RootModel) SetLoginModel(lm screens.LoginModel) {
	m.login = lm
}

// SetOnChatOpen registers a callback invoked whenever the user opens a chat.
func (m *RootModel) SetOnChatOpen(fn func(int64)) {
	m.onChatOpen = fn
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
		total := 0
		for _, c := range chats {
			if f.Matches(c) {
				total += c.UnreadCount
			}
		}
		counts[f.ID] = total
	}
	return counts
}

func (m RootModel) Init() tea.Cmd {
	m.statusBar.SetVerbose(m.verbose)
	m.statusBar.SetActivePane("chatlist")
	return logoTickCmd()
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logo.SetWidth(msg.Width)
		m.statusBar.SetWidth(msg.Width)
		paneH := msg.Height - 1
		innerH := paneH - 2*borderSize
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(msg.Width, sidebarW, 0.30)
			m.folderBar.SetSize(sidebarW-2*borderSize, innerH)
			m.chatList.SetSize(chatlistW-2*borderSize, innerH)
			m.chat.SetSize(chatW-2*borderSize, innerH)
		} else {
			leftW, rightW := layout.SplitHorizontal(msg.Width, msg.Height, 0.30)
			m.chatList.SetSize(leftW-2*borderSize, innerH)
			m.chat.SetSize(rightW-2*borderSize, innerH)
		}
		return m, nil

	case FolderFiltersMsg:
		if m.folderBar != nil {
			m.folderBar.SetFolders(msg.Filters)
			if m.width > 0 && m.height > 0 {
				const sidebarW = 18
				paneH := m.height - 1
				innerH := paneH - 2*borderSize
				_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
				m.folderBar.SetSize(sidebarW-2*borderSize, innerH)
				m.chatList.SetSize(chatlistW-2*borderSize, innerH)
				m.chat.SetSize(chatW-2*borderSize, innerH)
			}
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		return m, nil

	case screens.FolderSelectedMsg:
		m.activeFilter = msg.Filter
		m.chatList.SetChats(m.filteredChats())
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		return m, nil

	case screens.TransitionToMainMsg:
		m.screen = ScreenMain
		m.statusBar.SetVerbose(m.verbose)
		m.statusBar.SetActivePane("chatlist")
		if m.st != nil {
			m.chatList.SetChats(m.filteredChats())
		}
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		return m, spinnerTickCmd()

	case screens.CloseSearchMsg:
		m.searchModel = nil
		return m, nil

	case screens.OpenChatMsg:
		m.searchModel = nil
		m.currentChatID = msg.Chat.ID
		m.chatList.SetCursorByID(msg.Chat.ID)
		if m.onChatOpen != nil {
			m.onChatOpen(msg.Chat.ID)
		}
		m.chat.ClearPendingAction()
		m.chat.SetChat(&msg.Chat)
		if m.st != nil {
			m.chat.SetMessages(m.st.Messages(msg.Chat.ID))
		}
		m.chat.SetKnownImages(m.imageCache)
		m.focus = FocusChat
		m.chatList.SetFocused(false)
		m.chat.SetFocused(true)
		m.statusBar.SetActivePane("chat")
		if m.tgClient != nil {
			m.chat.SetLoading(true)
			client := m.tgClient
			peer := msg.Chat.Peer
			chatID := msg.Chat.ID
			limit := m.historyLimit
			return m, func() tea.Msg {
				msgs, err := client.GetHistory(context.Background(), peer, 0, limit)
				if err != nil {
					return nil
				}
				return ChatHistoryMsg{ChatID: chatID, Messages: msgs}
			}
		}
		return m, nil

	case ChatHistoryMsg:
		if m.st != nil {
			m.st.SetMessages(msg.ChatID, msg.Messages)
			if msg.ChatID == m.currentChatID {
				m.chat.SetMessages(m.st.Messages(msg.ChatID))
				m.chat.SetLoading(false)
				if chat, ok := m.st.GetChat(msg.ChatID); ok && chat.UnreadCount > 0 {
					m.chat.ScrollToFirstUnread(chat.ReadInboxMaxID)
				}
			}
		}
		return m, tea.Batch(m.markReadCmd(), m.pendingDownloadCmds(msg.Messages))

	case markReadDoneMsg:
		if m.st != nil {
			m.st.UpdateChatReadMaxID(msg.chatID, msg.maxID)
			if chat, ok := m.st.GetChat(msg.chatID); ok {
				m.chatList.SetChatUnread(msg.chatID, chat.UnreadCount)
			}
		}
		return m, nil

	case screens.LoadMoreMsg:
		if m.st == nil || m.tgClient == nil {
			return m, nil
		}
		chat, ok := m.st.GetChat(msg.ChatID)
		if !ok {
			return m, nil
		}
		client := m.tgClient
		peer := chat.Peer
		offsetID := msg.OffsetID
		limit := m.historyLimit
		chatID := msg.ChatID
		return m, func() tea.Msg {
			msgs, err := client.GetHistory(context.Background(), peer, offsetID, limit)
			if err != nil {
				return nil
			}
			return historyChunkMsg{chatID: chatID, messages: msgs}
		}

	case historyChunkMsg:
		if m.st != nil && msg.chatID == m.currentChatID && len(msg.messages) > 0 {
			existing := m.st.Messages(msg.chatID)
			combined := append(msg.messages, existing...)
			m.st.SetMessages(msg.chatID, combined)
			m.chat.PrependMessages(msg.messages) // preserves viewport position
		}
		return m, nil

	case PhotoReadyMsg:
		m.imageCache[msg.PhotoID] = msg.Image
		m.chat.SetImage(msg.PhotoID, msg.Image)
		return m, nil

	case screens.OpenPhotoMsg:
		if img, ok := m.imageCache[msg.PhotoID]; ok {
			go openInViewer(img)
		}
		return m, nil

	case store.Event:
		if m.st == nil {
			return m, nil
		}
		switch msg.Kind {
		case store.EventNewMessage:
			m.st.AppendMessage(msg.Message)
			m.chatList.SetChats(m.filteredChats())
			if m.folderBar != nil {
				m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
			}
			if msg.Message.ChatID == m.currentChatID {
				m.chat.SetMessages(m.st.Messages(m.currentChatID))
				return m, m.markReadCmd()
			}
			m.chatList.IncrementUnread(msg.Message.ChatID)
		case store.EventReadInbox:
			m.st.UpdateChatReadMaxID(msg.ChatID, msg.ReadMaxID)
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chatList.SetChatUnread(msg.ChatID, chat.UnreadCount)
			}
			if m.folderBar != nil {
				m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
			}
		case store.EventReadOutbox:
			m.st.UpdateChatOutboxReadMaxID(msg.ChatID, msg.ReadMaxID)
			if msg.ChatID == m.currentChatID {
				if chat, ok := m.st.GetChat(msg.ChatID); ok {
					m.chat.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
				}
			}
		case store.EventReactionsUpdate:
			m.st.UpdateMessageReactions(msg.ChatID, msg.MsgID, msg.Reactions)
			if msg.ChatID == m.currentChatID {
				m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
			}
		case store.EventDeleteMessages:
			if msg.ChatID != 0 {
				m.st.RemoveMessages(msg.ChatID, msg.MsgIDs)
			} else {
				for _, chat := range m.st.Chats() {
					m.st.RemoveMessages(chat.ID, msg.MsgIDs)
				}
			}
			if msg.ChatID == 0 || msg.ChatID == m.currentChatID {
				m.chat.SetMessages(m.st.Messages(m.currentChatID))
			}
			m.chatList.SetChats(m.filteredChats())
		}
		return m, nil

	case screens.SendMsgRequest:
		if m.tgClient == nil {
			return m, nil
		}
		m.nextSentinel--
		sentinelID := m.nextSentinel
		sentinel := store.Message{
			ID:           sentinelID,
			ChatID:       m.currentChatID,
			Text:         msg.Text,
			Date:         time.Now(),
			IsOut:        true,
			ReplyToMsgID: msg.ReplyToMsgID,
		}
		if m.st != nil {
			m.st.AppendMessage(sentinel)
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
		}
		client := m.tgClient
		peer := msg.Peer
		text := msg.Text
		replyToMsgID := msg.ReplyToMsgID
		chatID := m.currentChatID
		return m, func() tea.Msg {
			realID, err := client.SendMessage(context.Background(), peer, text, replyToMsgID)
			if err != nil {
				realID = 0
			}
			return sentMsgConfirmedMsg{chatID: chatID, sentinelID: sentinelID, realID: realID}
		}

	case screens.EditSendRequest:
		if m.st == nil || m.tgClient == nil {
			return m, nil
		}
		chatID := m.currentChatID
		m.st.UpdateMessageText(chatID, msg.MsgID, msg.Text, time.Now())
		m.chat.SetMessages(m.st.Messages(chatID))
		client := m.tgClient
		peer := msg.Peer
		msgID := msg.MsgID
		text := msg.Text
		return m, func() tea.Msg {
			_ = client.EditMessage(context.Background(), peer, msgID, text)
			return nil
		}

	case sentMsgConfirmedMsg:
		if m.st == nil {
			return m, nil
		}
		if msg.realID != 0 {
			m.st.UpdateMessageID(msg.chatID, msg.sentinelID, msg.realID)
		} else {
			m.st.RemoveMessage(msg.chatID, msg.sentinelID)
		}
		if msg.chatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(msg.chatID))
		}
		return m, nil

	case components.JumpToMsgRequest:
		m.contextMenu = nil
		if !m.chat.ScrollToMessage(msg.MsgID) {
			m.statusBar.SetStatus("Not in buffer")
		}
		return m, nil

	case components.ReplyMsgRequest:
		m.contextMenu = nil
		return m, m.activateReply(msg.MsgID)

	case components.EditMsgRequest:
		m.contextMenu = nil
		return m, m.activateEdit(msg.MsgID)

	case components.CloseContextMenuMsg:
		m.contextMenu = nil
		return m, nil

	case components.ReactMsgRequest:
		m.contextMenu = nil
		if m.st == nil {
			return m, nil
		}
		var chosen string
		for _, sm := range m.st.Messages(m.currentChatID) {
			if sm.ID == msg.MsgID {
				for _, r := range sm.Reactions {
					if r.IsChosen {
						chosen = r.Emoji
						break
					}
				}
				break
			}
		}
		m.reactionTargetID = msg.MsgID
		m.reactionPicker = components.NewReactionPicker(chosen)
		return m, nil

	case components.CloseReactionPickerMsg:
		m.reactionPicker = nil
		return m, nil

	case reactionFailedMsg:
		if m.st != nil {
			m.st.UpdateMessageReactions(msg.chatID, msg.msgID, msg.reactions)
			if msg.chatID == m.currentChatID {
				m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
			}
		}
		return m, nil

	case components.ReactConfirmedMsg:
		m.reactionPicker = nil
		if m.st == nil || m.tgClient == nil {
			return m, nil
		}
		chatID := m.currentChatID
		msgID := m.reactionTargetID
		emoji := msg.Emoji
		currentReactions := m.st.Messages(chatID)
		var msgReactions []store.Reaction
		for _, sm := range currentReactions {
			if sm.ID == msgID {
				msgReactions = sm.Reactions
				break
			}
		}
		alreadyChosen := false
		for _, r := range msgReactions {
			if r.Emoji == emoji && r.IsChosen {
				alreadyChosen = true
				break
			}
		}
		sendEmoji := emoji
		if alreadyChosen {
			sendEmoji = ""
		}
		origReactions := make([]store.Reaction, len(msgReactions))
		copy(origReactions, msgReactions)
		newReactions := buildOptimisticReactions(msgReactions, emoji)
		m.st.UpdateMessageReactions(chatID, msgID, newReactions)
		m.chat.SetMessagesKeepScroll(m.st.Messages(chatID))
		chat, ok := m.st.GetChat(chatID)
		if !ok {
			return m, nil
		}
		client := m.tgClient
		peer := chat.Peer
		return m, func() tea.Msg {
			if err := client.SendReaction(context.Background(), peer, msgID, sendEmoji); err != nil {
				return reactionFailedMsg{chatID: chatID, msgID: msgID, reactions: origReactions}
			}
			return nil
		}

	case components.DeleteMsgRequest:
		m.contextMenu = nil
		if m.st == nil {
			return m, nil
		}
		m.st.RemoveMessage(m.currentChatID, msg.MsgID)
		m.chat.RemoveMessage(msg.MsgID)
		if m.tgClient == nil {
			return m, nil
		}
		chat, ok := m.st.GetChat(m.currentChatID)
		if !ok {
			return m, nil
		}
		client := m.tgClient
		peer := chat.Peer
		msgID := msg.MsgID
		revoke := msg.Revoke
		return m, func() tea.Msg {
			// TODO: on error, re-insert message (optimistic delete, no rollback yet)
			_ = client.DeleteMessages(context.Background(), peer, []int{msgID}, revoke)
			return nil
		}

	case components.LogoTickMsg:
		m.logo.Tick()
		m.chat.TickLogo()
		return m, logoTickCmd()

	case components.SpinnerTickMsg:
		m.chatList.TickSpinner()
		m.chat.TickSpinner()
		if m.screen == ScreenMain {
			return m, spinnerTickCmd()
		}
		return m, nil

	case screens.AuthRequestMsg, screens.ConnectedMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			if _, ok := msg.(screens.AuthRequestMsg); ok {
				m.logo.SetState(components.LogoStateStatic)
			}
			return m, cmd
		}
		return m, nil

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
		action := m.keyMap.Resolve(keys.ContextFolders, keyStr)
		if action != keys.ActionNone {
			newPane, cmd := m.folderBar.Update(keys.ActionMsg{Action: action})
			m.folderBar = newPane.(*screens.FoldersModel)
			return m, cmd
		}
		return m, nil
	}

	if m.focus == FocusChatList {
		// Handle gg two-key sequence
		if m.chatListPendingKey == "g" {
			m.chatListPendingKey = ""
			if keyStr == "g" {
				newPane, cmd := m.chatList.Update(keys.ActionMsg{Action: keys.ActionGoTop})
				m.chatList = newPane.(*screens.ChatListModel)
				return m, cmd
			}
			// Not gg — fall through and process current key normally
		}
		if keyStr == "g" {
			m.chatListPendingKey = "g"
			return m, nil
		}

		action := m.keyMap.Resolve(keys.ContextChatList, keyStr)
		if action != keys.ActionNone {
			newPane, cmd := m.chatList.Update(keys.ActionMsg{Action: action})
			m.chatList = newPane.(*screens.ChatListModel)
			return m, cmd
		}
		return m, nil
	}

	// Chat pane: route through vim state machine
	action := m.vimState.Process(keyStr)
	m.statusBar.SetMode(m.vimState.Mode)

	if action == keys.ActionOpenContextMenu && m.focus == FocusChat {
		if m.chat != nil {
			msgID := m.chat.SelectedMessageID()
			isOut := m.chat.SelectedMessageIsOut()
			if msgID != 0 {
				replyToMsgID := m.chat.SelectedMessageReplyToMsgID()
				m.contextMenu = components.NewContextMenu(msgID, isOut, replyToMsgID, m.keyMap)
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

	if action == keys.ActionPassthrough {
		newPane, cmd := m.chat.Update(msg)
		m.chat = newPane.(*screens.ChatModel)
		return m, cmd
	}

	if action != keys.ActionNone {
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: action})
		m.chat = newPane.(*screens.ChatModel)
		return m, tea.Batch(cmd, m.markReadCmd())
	}

	return m, nil
}

func openInViewer(img image.Image) {
	f, err := os.CreateTemp("", "tele-photo-*.jpg")
	if err != nil {
		return
	}
	name := f.Name()
	if err := jpeg.Encode(f, img, nil); err != nil {
		f.Close()
		os.Remove(name)
		return
	}
	f.Close()

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", name)
	default:
		cmd = exec.Command("xdg-open", name)
	}
	_ = cmd.Start()
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

func (m RootModel) pendingDownloadCmds(msgs []store.Message) tea.Cmd {
	var cmds []tea.Cmd
	for _, msg := range msgs {
		if msg.Photo != nil {
			if _, ok := m.imageCache[msg.Photo.ID]; !ok {
				cmds = append(cmds, downloadPhotoCmd(m.tgClient, *msg.Photo))
			}
		}
	}
	return tea.Batch(cmds...)
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

func buildOptimisticReactions(current []store.Reaction, emoji string) []store.Reaction {
	alreadyChosen := false
	for _, r := range current {
		if r.Emoji == emoji && r.IsChosen {
			alreadyChosen = true
			break
		}
	}
	out := make([]store.Reaction, 0, len(current)+1)
	emojiFound := false
	for _, r := range current {
		nr := r
		if r.Emoji == emoji {
			emojiFound = true
			if alreadyChosen {
				nr.IsChosen = false
				nr.Count--
				if nr.Count <= 0 {
					continue
				}
			} else {
				nr.IsChosen = true
				nr.Count++
			}
		} else if r.IsChosen {
			nr.IsChosen = false
			nr.Count--
			if nr.Count <= 0 {
				continue
			}
		}
		out = append(out, nr)
	}
	if !alreadyChosen && !emojiFound && emoji != "" {
		out = append(out, store.Reaction{Emoji: emoji, Count: 1, IsChosen: true})
	}
	return out
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
	// Exit insert mode when leaving chat
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeInsert {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
		newPane, _ := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
		m.chat = newPane.(*screens.ChatModel)
	}
	if target == FocusChat {
		// Open the currently highlighted chat (triggers history load + focus switch)
		if chat, ok := m.chatList.SelectedChat(); ok {
			return m, func() tea.Msg { return screens.OpenChatMsg{Chat: chat} }
		}
		// No chats loaded yet — just switch focus
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
			loginBox := components.RenderBox(padded, "Telegram", "", b, innerW+2, innerH+2)
			combined := lipgloss.JoinVertical(lipgloss.Center, logoView, "\n", loginBox)
			content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, combined)
		}
	} else {
		paneH := m.height + 1
		innerH := paneH - 2*borderSize

		activeBorder := lipgloss.DoubleBorder()
		inactiveBorder := lipgloss.NormalBorder()

		foldersBorder := inactiveBorder
		chatListBorder := inactiveBorder
		chatBorder := inactiveBorder
		switch m.focus {
		case FocusFolders:
			foldersBorder = activeBorder
		case FocusChatList:
			chatListBorder = activeBorder
		case FocusChat:
			chatBorder = activeBorder
		}

		chatListTitle := "[1] Chats"
		chatTitle := "[2] " + m.chat.Title()

		var main string
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
			foldersView := components.RenderBox(m.folderBar.View(), "[0] Folders", "", foldersBorder, sidebarW, innerH)
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", chatListBorder, chatlistW, innerH)
			chatView := components.RenderBox(m.chat.View(), chatTitle, "", chatBorder, chatW, innerH)
			main = lipgloss.JoinHorizontal(lipgloss.Top, foldersView, chatListView, chatView)
		} else {
			leftW, rightW := layout.SplitHorizontal(m.width, m.height, 0.30)
			chatListWidth := leftW - 2*borderSize + 2
			chatWidth := rightW - 2*borderSize + 2
			chatListView := components.RenderBox(m.chatList.View(), chatListTitle, "", chatListBorder, chatListWidth, innerH)
			chatView := components.RenderBox(m.chat.View(), chatTitle, "", chatBorder, chatWidth, innerH)
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

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return components.SpinnerTickMsg{}
	})
}

