package screens

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

type SendMsgRequest struct {
	Peer store.Peer
	Text string
}

type ChatModel struct {
	chat            *store.Chat
	msgList         *components.MessageList
	composer        *components.Composer
	vimState        *keys.VimState
	width           int
	height          int
	focused         bool
	composerFocused bool
}

func NewChatModel(width, height int) *ChatModel {
	composerHeight := 3
	listHeight := height - composerHeight - 1
	if listHeight < 1 {
		listHeight = 1
	}
	return &ChatModel{
		msgList:  components.NewMessageList(listHeight, width),
		composer: components.NewComposer(width),
		vimState: keys.NewVimState(),
		width:    width,
		height:   height,
	}
}

func (m *ChatModel) SetChat(chat *store.Chat)         { m.chat = chat }
func (m *ChatModel) SetMessages(msgs []store.Message) { m.msgList.SetMessages(msgs) }
func (m *ChatModel) ComposerFocused() bool            { return m.composerFocused }
func (m *ChatModel) Context() keys.Context            { return keys.ContextChat }
func (m *ChatModel) Focused() bool                    { return m.focused }
func (m *ChatModel) SetFocused(f bool)                { m.focused = f }
func (m *ChatModel) SetComposerValue(v string)        { m.composer.SetValue(v) }

func (m *ChatModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	composerHeight := 3
	m.msgList.SetSize(width, height-composerHeight-1)
	m.composer.SetWidth(width)
}

func (m *ChatModel) Init() tea.Cmd { return m.composer.Init() }

func (m *ChatModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case keys.ActionMsg:
		if m.composerFocused {
			if msg.Action == keys.ActionNormal {
				m.composerFocused = false
				m.composer.Blur()
				m.vimState.Mode = keys.ModeNormal
			}
			return m, nil
		}
		switch msg.Action {
		case keys.ActionDown:
			m.msgList.ScrollDown()
		case keys.ActionUp:
			m.msgList.ScrollUp()
		case keys.ActionGoTop:
			// lazy load will be triggered here in Phase 2
		case keys.ActionGoBottom:
			m.msgList.ScrollDown()
		case keys.ActionInsert:
			m.composerFocused = true
			m.vimState.Mode = keys.ModeInsert
			m.composer.Focus()
		}
		return m, nil

	case tea.KeyMsg:
		if m.composerFocused {
			if msg.Type == tea.KeyEnter {
				text := m.composer.Value()
				m.composer.Reset()
				if m.chat != nil && text != "" {
					peer := m.chat.Peer
					return m, func() tea.Msg { return SendMsgRequest{Peer: peer, Text: text} }
				}
				return m, nil
			}
			newC, cmd := m.composer.Update(msg)
			m.composer = newC
			return m, cmd
		}
	}
	return m, nil
}

func (m *ChatModel) View() string {
	title := "(no chat)"
	if m.chat != nil {
		title = m.chat.Title
	}
	divider := "─────────────────────────────────────────"
	return title + "\n" + divider + "\n" + m.msgList.View() + "\n" + m.composer.View()
}
