package ui

import (
	"context"
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
	FocusChatList Focus = iota
	FocusChat
)

// borderSize is the number of characters each border side adds (1 per side = 2 total per axis).
const borderSize = 1

type chatHistoryMsg struct {
	chatID   int64
	messages []store.Message
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
	searchModel   *screens.SearchModel
	onChatOpen    func(int64)
	nextSentinel  int
}

func NewRootModel(client internaltg.Client, st store.Store, historyLimit int, verbose bool) RootModel {
	return RootModel{
		screen:       ScreenLogin,
		focus:        FocusChatList,
		chatList:     screens.NewChatListModel(),
		chat:         screens.NewChatModel(80, 24),
		statusBar:    components.NewStatusBar(80),
		vimState:     keys.NewVimState(),
		keyMap:       keys.DefaultKeyMap(),
		tgClient:     client,
		st:           st,
		historyLimit: historyLimit,
		verbose:      verbose,
	}
}

func (m RootModel) CurrentScreen() Screen            { return m.screen }
func (m RootModel) CurrentFocus() Focus              { return m.focus }
func (m RootModel) ChatList() *screens.ChatListModel { return m.chatList }

// WithScreen returns a copy with the given screen set (used in tests and app init).
func (m RootModel) WithScreen(s Screen) RootModel {
	m.screen = s
	return m
}

func (m RootModel) WithFocus(f Focus) RootModel {
	m.focus = f
	return m
}

func (m RootModel) SearchActive() bool { return m.searchModel != nil }

// SetLoginModel injects the login model after NewRootModel (called by app.go).
func (m *RootModel) SetLoginModel(lm screens.LoginModel) {
	m.login = lm
}

// SetOnChatOpen registers a callback invoked whenever the user opens a chat.
func (m *RootModel) SetOnChatOpen(fn func(int64)) {
	m.onChatOpen = fn
}

func (m RootModel) Init() tea.Cmd {
	m.statusBar.SetVerbose(m.verbose)
	m.statusBar.SetActivePane("chatlist")
	return nil
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)
		leftW, rightW := layout.SplitHorizontal(msg.Width, msg.Height, 0.30)
		paneH := msg.Height - 1
		m.chatList.SetSize(leftW-2*borderSize, paneH-2*borderSize)
		m.chat.SetSize(rightW-2*borderSize, paneH-2*borderSize)
		return m, nil

	case screens.TransitionToMainMsg:
		m.screen = ScreenMain
		m.statusBar.SetVerbose(m.verbose)
		m.statusBar.SetActivePane("chatlist")
		if m.st != nil {
			m.chatList.SetChats(m.st.Chats())
		}
		return m, nil

	case screens.CloseSearchMsg:
		m.searchModel = nil
		return m, nil

	case screens.OpenChatMsg:
		m.searchModel = nil
		m.currentChatID = msg.Chat.ID
		if m.onChatOpen != nil {
			m.onChatOpen(msg.Chat.ID)
		}
		m.chat.SetChat(&msg.Chat)
		if m.st != nil {
			m.chat.SetMessages(m.st.Messages(msg.Chat.ID))
		}
		m.focus = FocusChat
		m.chatList.SetFocused(false)
		m.chat.SetFocused(true)
		if m.tgClient != nil {
			client := m.tgClient
			peer := msg.Chat.Peer
			chatID := msg.Chat.ID
			limit := m.historyLimit
			return m, func() tea.Msg {
				msgs, err := client.GetHistory(context.Background(), peer, 0, limit)
				if err != nil {
					return nil
				}
				return chatHistoryMsg{chatID: chatID, messages: msgs}
			}
		}
		return m, nil

	case chatHistoryMsg:
		if m.st != nil {
			m.st.SetMessages(msg.chatID, msg.messages)
			if msg.chatID == m.currentChatID {
				m.chat.SetMessages(m.st.Messages(msg.chatID))
				if chat, ok := m.st.GetChat(msg.chatID); ok && chat.UnreadCount > 0 {
					m.chat.ScrollToFirstUnread(chat.ReadInboxMaxID)
				}
			}
		}
		return m, m.markReadCmd()

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

	case store.Event:
		if m.st == nil {
			return m, nil
		}
		switch msg.Kind {
		case store.EventNewMessage:
			m.st.AppendMessage(msg.Message)
			if msg.Message.ChatID == m.currentChatID {
				m.chat.SetMessages(m.st.Messages(m.currentChatID))
			}
			m.chatList.SetChats(m.st.Chats())
			if msg.Message.ChatID != m.currentChatID {
				m.chatList.IncrementUnread(msg.Message.ChatID)
			}
		case store.EventReadInbox:
			m.st.UpdateChatReadMaxID(msg.ChatID, msg.ReadMaxID)
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chatList.SetChatUnread(msg.ChatID, chat.UnreadCount)
			}
		case store.EventReadOutbox:
			m.st.UpdateChatOutboxReadMaxID(msg.ChatID, msg.ReadMaxID)
			if msg.ChatID == m.currentChatID {
				if chat, ok := m.st.GetChat(msg.ChatID); ok {
					m.chat.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
				}
			}
		}
		return m, nil

	case screens.SendMsgRequest:
		if m.tgClient == nil {
			return m, nil
		}
		m.nextSentinel--
		sentinelID := m.nextSentinel
		sentinel := store.Message{
			ID:     sentinelID,
			ChatID: m.currentChatID,
			Text:   msg.Text,
			Date:   time.Now(),
			IsOut:  true,
		}
		if m.st != nil {
			m.st.AppendMessage(sentinel)
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
		}
		client := m.tgClient
		peer := msg.Peer
		text := msg.Text
		chatID := m.currentChatID
		return m, func() tea.Msg {
			realID, err := client.SendMessage(context.Background(), peer, text)
			if err != nil {
				realID = 0
			}
			return sentMsgConfirmedMsg{chatID: chatID, sentinelID: sentinelID, realID: realID}
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

	case screens.AuthRequestMsg, screens.ConnectedMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
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
	keyStr := msg.String()
	if m.verbose {
		m.statusBar.SetLastKey(keyStr)
	}

	if m.searchModel != nil {
		newSearch, cmd := m.searchModel.Update(msg)
		m.searchModel = newSearch
		return m, cmd
	}

	// Global bindings always take priority
	switch m.keyMap.Resolve(keys.ContextGlobal, keyStr) {
	case keys.ActionFocusLeft:
		return m.focusPane(FocusChatList)
	case keys.ActionFocusRight:
		return m.focusPane(FocusChat)
	case keys.ActionQuit:
		return m, tea.Quit
	}

	if keyStr == "/" {
		if m.st != nil {
			m.searchModel = screens.NewSearchModel(m.st.Chats(), m.width, m.height)
		}
		return m, nil
	}

	if m.focus == FocusChatList {
		// Chatlist uses keymap directly, no vim state machine
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
	if m.verbose {
		if target == FocusChatList {
			m.statusBar.SetActivePane("chatlist")
		} else {
			m.statusBar.SetActivePane("chat")
		}
	}
	return m, nil
}

func (m RootModel) View() tea.View {
	var content string
	if m.screen == ScreenLogin {
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
		loginBox := renderWithTitle(padded, "Telegram", b, innerW+2, innerH+2)
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, loginBox)
	} else {
		leftW, rightW := layout.SplitHorizontal(m.width, m.height, 0.30)
		paneH := m.height + 1
		innerH := paneH - 2*borderSize

		activeBorder := lipgloss.DoubleBorder()
		inactiveBorder := lipgloss.NormalBorder()

		chatListBorder, chatBorder := inactiveBorder, inactiveBorder
		if m.focus == FocusChatList {
			chatListBorder = activeBorder
		} else {
			chatBorder = activeBorder
		}

		chatListWidth := leftW - 2*borderSize + 2
		chatWidth := rightW - 2*borderSize + 2

		chatListView := renderWithTitle(m.chatList.View(), "[1] Chats", chatListBorder, chatListWidth, innerH)
		chatView := renderWithTitle(m.chat.View(), "[2] "+m.chat.Title(), chatBorder, chatWidth, innerH)

		main := lipgloss.JoinHorizontal(lipgloss.Top, chatListView, chatView)
		content = main + "\n" + m.statusBar.View()
		if m.searchModel != nil {
			content = overlayCenter(content, m.searchModel.View(), m.width, m.height)
		}
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderWithTitle renders a bordered box with a title in the top border line.
// w and h are the total outer dimensions (including the 1-char border on each side).
func renderWithTitle(content, title string, b lipgloss.Border, w, h int) string {
	innerW := w - 2
	innerH := h - 2

	titleStr := " " + title + " "
	titleW := lipgloss.Width(titleStr)
	fillW := innerW - titleW

	var top string
	if fillW >= 2 {
		rightFill := fillW - 1
		top = b.TopLeft + b.Top + titleStr + strings.Repeat(b.Top, rightFill) + b.TopRight
	} else {
		top = b.TopLeft + strings.Repeat(b.Top, innerW) + b.TopRight
	}
	bottom := b.BottomLeft + strings.Repeat(b.Bottom, innerW) + b.BottomRight

	lines := strings.Split(content, "\n")
	for len(lines) < innerH {
		lines = append(lines, strings.Repeat(" ", innerW))
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	result := make([]string, 0, innerH+2)
	result = append(result, top)
	for _, l := range lines {
		lw := lipgloss.Width(l)
		if lw < innerW {
			l += strings.Repeat(" ", innerW-lw)
		}
		result = append(result, b.Left+l+b.Right)
	}
	result = append(result, bottom)
	return strings.Join(result, "\n")
}
