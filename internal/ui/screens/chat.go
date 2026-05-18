package screens

import (
	"image"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
)

type SendMsgRequest struct {
	Peer         store.Peer
	Text         string
	ReplyToMsgID int
}

type OpenPhotoMsg struct {
	PhotoID int64
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
	replyToMsgID    int
}

func NewChatModel(width, height int) *ChatModel {
	composer := components.NewComposer(width)
	listH := height - composer.VisualHeight()
	if listH < 1 {
		listH = 1
	}
	ml := components.NewMessageList(listH, width)
	ml.SetShowIndicator(true)
	return &ChatModel{
		msgList:  ml,
		composer: composer,
		vimState: keys.NewVimState(),
		width:    width,
		height:   height,
	}
}

func (m *ChatModel) SetChat(chat *store.Chat) {
	m.chat = chat
	if chat != nil {
		m.msgList.SetIsGroup(chat.Peer.Type == store.PeerGroup || chat.Peer.Type == store.PeerChannel)
		m.msgList.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
	} else {
		m.msgList.SetIsGroup(false)
		m.msgList.SetOutboxReadMaxID(0)
	}
}
func (m *ChatModel) SetMessages(msgs []store.Message)                  { m.msgList.SetMessages(msgs) }
func (m *ChatModel) RemoveMessage(id int)                              { m.msgList.RemoveMessage(id) }
func (m *ChatModel) PrependMessages(older []store.Message)             { m.msgList.PrependMessages(older) }
func (m *ChatModel) SetImage(photoID int64, img image.Image)           { m.msgList.SetImage(photoID, img) }
func (m *ChatModel) SetKnownImages(cache map[int64]image.Image)        { m.msgList.SetKnownImages(cache) }
func (m *ChatModel) SetOutboxReadMaxID(id int)             { m.msgList.SetOutboxReadMaxID(id) }
func (m *ChatModel) ScrollToFirstUnread(readMaxID int) bool { return m.msgList.ScrollToFirstUnread(readMaxID) }
func (m *ChatModel) VisibleReadMaxID() int                  { return m.msgList.VisibleReadMaxID() }
func (m *ChatModel) ComposerFocused() bool            { return m.composerFocused }
func (m *ChatModel) ComposerHeight() int              { return m.composer.VisualHeight() }
func (m *ChatModel) SelectedMessageID() int           { return m.msgList.SelectedMessageID() }
func (m *ChatModel) SelectedMessageIsOut() bool        { return m.msgList.SelectedMessageIsOut() }
func (m *ChatModel) SelectedMessageReplyToMsgID() int  { return m.msgList.SelectedMessageReplyToMsgID() }
func (m *ChatModel) ScrollToMessage(id int) bool       { return m.msgList.ScrollToMessage(id) }
func (m *ChatModel) ReplyToMsgID() int                 { return m.replyToMsgID }

func (m *ChatModel) clearPendingAction() {
	m.replyToMsgID = 0
	m.composer.ClearReplyPreview()
}

// ClearPendingAction clears any active reply (or future forward) state.
func (m *ChatModel) ClearPendingAction() {
	m.clearPendingAction()
	m.syncMsgListHeight()
}

// SetReply activates reply mode. Clears any existing pending action first.
func (m *ChatModel) SetReply(msgID int, preview string) {
	m.clearPendingAction()
	m.replyToMsgID = msgID
	m.composer.SetReplyPreview(preview)
	m.syncMsgListHeight()
}

// FocusComposer focuses the composer and switches to insert mode.
// Returns a blink Cmd that must be returned from the parent Update.
func (m *ChatModel) FocusComposer() tea.Cmd {
	m.composerFocused = true
	m.vimState.Mode = keys.ModeInsert
	m.msgList.SetShowIndicator(false)
	return m.composer.Focus()
}

func (m *ChatModel) Context() keys.Context            { return keys.ContextChat }
func (m *ChatModel) Focused() bool                    { return m.focused }
func (m *ChatModel) SetFocused(f bool)                { m.focused = f }
func (m *ChatModel) SetComposerValue(v string) {
	m.composer.SetValue(v)
	m.syncMsgListHeight()
}

func (m *ChatModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.composer.SetWidth(width)
	m.syncMsgListHeight()
}

func (m *ChatModel) syncMsgListHeight() {
	listH := m.height - m.composer.VisualHeight()
	if listH < 1 {
		listH = 1
	}
	m.msgList.SetSize(m.width, listH)
}

func (m *ChatModel) Title() string {
	if m.chat == nil {
		return "(no chat)"
	}
	return m.chat.Title
}

func (m *ChatModel) Init() tea.Cmd { return m.composer.Init() }

func (m *ChatModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case keys.ActionMsg:
		if m.composerFocused {
			if msg.Action == keys.ActionNormal {
				m.clearPendingAction()
				m.syncMsgListHeight()
				m.composerFocused = false
				m.composer.Blur()
				m.vimState.Mode = keys.ModeNormal
				m.msgList.SetShowIndicator(true)
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
			m.msgList.ScrollToTop()
			if m.chat != nil && m.msgList.Count() > 0 {
				chatID := m.chat.ID
				offsetID := m.msgList.OldestID()
				return m, func() tea.Msg { return LoadMoreMsg{ChatID: chatID, OffsetID: offsetID} }
			}
		case keys.ActionGoBottom:
			m.msgList.ScrollToBottom()
		case keys.ActionScrollHalfDown:
			n := m.msgList.ViewHeight() * 2 / 3
			if n < 1 {
				n = 1
			}
			m.msgList.ScrollDownBy(n)
		case keys.ActionScrollHalfUp:
			n := m.msgList.ViewHeight() * 2 / 3
			if n < 1 {
				n = 1
			}
			m.msgList.ScrollUpBy(n)
			if m.msgList.AtTop() && m.chat != nil && m.msgList.Count() > 0 {
				chatID := m.chat.ID
				offsetID := m.msgList.OldestID()
				return m, func() tea.Msg { return LoadMoreMsg{ChatID: chatID, OffsetID: offsetID} }
			}
		case keys.ActionInsert:
			m.composerFocused = true
			m.vimState.Mode = keys.ModeInsert
			focusCmd := m.composer.Focus()
			m.msgList.SetShowIndicator(false)
			return m, focusCmd
		case keys.ActionOpenPhoto:
			photoID := m.msgList.LastVisiblePhotoID()
			if photoID != 0 {
				id := photoID
				return m, func() tea.Msg { return OpenPhotoMsg{PhotoID: id} }
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.composerFocused {
			if msg.Code == tea.KeyEnter && msg.Mod == 0 {
				text := m.composer.Value()
				replyID := m.replyToMsgID
				m.clearPendingAction()
				m.composer.Reset()
				m.syncMsgListHeight()
				if m.chat != nil && text != "" {
					peer := m.chat.Peer
					return m, func() tea.Msg {
						return SendMsgRequest{Peer: peer, Text: text, ReplyToMsgID: replyID}
					}
				}
				return m, nil
			}
			newC, cmd := m.composer.Update(msg)
			m.composer = newC
			m.syncMsgListHeight()
			return m, cmd
		}
	}
	return m, nil
}

func (m *ChatModel) View() string {
	return m.msgList.View() + "\n" + m.composer.View()
}
