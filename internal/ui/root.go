package ui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
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

type RootModel struct {
	screen    Screen
	focus     Focus
	width     int
	height    int
	chatList  *screens.ChatListModel
	chat      *screens.ChatModel
	login     screens.LoginModel
	statusBar *components.StatusBar
	vimState  *keys.VimState
	keyMap    keys.KeyMap
	tgClient  internaltg.Client
	st        store.Store
}

func NewRootModel(client internaltg.Client, st store.Store) RootModel {
	return RootModel{
		screen:    ScreenLogin,
		focus:     FocusChatList,
		chatList:  screens.NewChatListModel(),
		chat:      screens.NewChatModel(80, 24),
		statusBar: components.NewStatusBar(80),
		vimState:  keys.NewVimState(),
		keyMap:    keys.DefaultKeyMap(),
		tgClient:  client,
		st:        st,
	}
}

func (m RootModel) CurrentScreen() Screen { return m.screen }
func (m RootModel) CurrentFocus() Focus   { return m.focus }

// WithScreen returns a copy with the given screen set (used in tests and app init).
func (m RootModel) WithScreen(s Screen) RootModel {
	m.screen = s
	return m
}

// SetLoginModel injects the login model after NewRootModel (called by app.go).
func (m *RootModel) SetLoginModel(lm screens.LoginModel) {
	m.login = lm
}

func (m RootModel) Init() tea.Cmd { return nil }

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.statusBar.SetWidth(msg.Width)
		leftW, rightW := layout.SplitHorizontal(msg.Width, msg.Height, 0.30)
		m.chatList.SetSize(leftW, msg.Height-1)
		m.chat.SetSize(rightW, msg.Height-1)
		return m, nil

	case screens.TransitionToMainMsg:
		m.screen = ScreenMain
		if m.st != nil {
			m.chatList.SetChats(m.st.Chats())
		}
		return m, nil

	case screens.OpenChatMsg:
		m.chat.SetChat(&msg.Chat)
		if m.st != nil {
			m.chat.SetMessages(m.st.Messages(msg.Chat.ID))
		}
		m.focus = FocusChat
		m.chatList.SetFocused(false)
		m.chat.SetFocused(true)
		return m, nil

	case store.Event:
		if msg.Kind == store.EventNewMessage && m.st != nil {
			m.st.AppendMessage(msg.Message)
			// refresh messages for the active chat if it matches
			if m.chat != nil {
				// will be refreshed on next OpenChatMsg; Phase 2 adds live refresh
			}
		}
		return m, nil

	case screens.SendMsgRequest:
		if m.tgClient == nil {
			return m, nil
		}
		client := m.tgClient
		peer := msg.Peer
		text := msg.Text
		return m, func() tea.Msg {
			_ = client.SendMessage(context.Background(), peer, text)
			return nil
		}

	case screens.AuthRequestMsg, screens.ConnectedMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			return m, cmd
		}
		return m.handleMainKey(msg)
	}
	return m, nil
}

func (m RootModel) handleMainKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyStr := msg.String()

	// Global bindings checked first
	switch m.keyMap.Resolve(keys.ContextGlobal, keyStr) {
	case keys.ActionSwitchFocus:
		if m.focus == FocusChatList {
			m.focus = FocusChat
			m.chatList.SetFocused(false)
			m.chat.SetFocused(true)
		} else {
			m.focus = FocusChatList
			m.chatList.SetFocused(true)
			m.chat.SetFocused(false)
		}
		return m, nil
	case keys.ActionQuit:
		return m, tea.Quit
	}

	// Vim processing
	action := m.vimState.Process(keyStr)
	m.statusBar.SetMode(m.vimState.Mode)

	if action == keys.ActionPassthrough {
		newPane, cmd := m.chat.Update(msg)
		m.chat = newPane.(*screens.ChatModel)
		return m, cmd
	}

	if action != keys.ActionNone {
		actionMsg := keys.ActionMsg{Action: action}
		if m.focus == FocusChatList {
			newPane, cmd := m.chatList.Update(actionMsg)
			m.chatList = newPane.(*screens.ChatListModel)
			return m, cmd
		}
		newPane, cmd := m.chat.Update(actionMsg)
		m.chat = newPane.(*screens.ChatModel)
		return m, cmd
	}

	return m, nil
}

func (m RootModel) View() string {
	if m.screen == ScreenLogin {
		return m.login.View()
	}

	leftW, _ := layout.SplitHorizontal(m.width, m.height, 0.30)
	chatListView := lipgloss.NewStyle().Width(leftW).Height(m.height - 1).Render(m.chatList.View())
	chatView := m.chat.View()

	main := lipgloss.JoinHorizontal(lipgloss.Top, chatListView, chatView)
	return main + "\n" + m.statusBar.View()
}
