package screens

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

type SendMsgRequest struct {
	Peer store.Peer
	Text string
}

type LoadMoreMsg struct {
	ChatID   int64
	OffsetID int
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

func (m *ChatModel) SetChat(chat *store.Chat) {
	m.chat = chat
	if chat != nil {
		m.msgList.SetIsGroup(chat.Peer.Type == store.PeerGroup || chat.Peer.Type == store.PeerChannel)
	} else {
		m.msgList.SetIsGroup(false)
	}
}
func (m *ChatModel) SetMessages(msgs []store.Message)       { m.msgList.SetMessages(msgs) }
func (m *ChatModel) PrependMessages(older []store.Message)  { m.msgList.PrependMessages(older) }
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
			atTop := m.msgList.AtTop()
			m.msgList.ScrollUp()
			if atTop && m.chat != nil && m.msgList.Count() > 0 {
				chatID := m.chat.ID
				offsetID := m.msgList.OldestID()
				return m, func() tea.Msg { return LoadMoreMsg{ChatID: chatID, OffsetID: offsetID} }
			}
		case keys.ActionGoTop:
			if m.chat != nil && m.msgList.Count() > 0 {
				chatID := m.chat.ID
				offsetID := m.msgList.OldestID()
				return m, func() tea.Msg { return LoadMoreMsg{ChatID: chatID, OffsetID: offsetID} }
			}
		case keys.ActionGoBottom:
			m.msgList.ScrollDown()
		case keys.ActionInsert:
			m.composerFocused = true
			m.vimState.Mode = keys.ModeInsert
			m.composer.Focus()
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.composerFocused {
			if msg.Code == tea.KeyEnter {
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
	w := m.width
	if w < 1 {
		w = 1
	}
	titleLine := lipgloss.NewStyle().Inline(true).Width(w).MaxWidth(w).Render(title)
	divider := strings.Repeat("─", w)
	return titleLine + "\n" + divider + "\n" + m.msgList.View() + "\n" + m.composer.View()
}
