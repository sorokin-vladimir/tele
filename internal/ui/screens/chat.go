package screens

import (
	"image"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
)

type SendMsgRequest struct {
	Peer         store.Peer
	Text         string
	ReplyToMsgID int
}

type EditSendRequest struct {
	Peer  store.Peer
	MsgID int
	Text  string
}

type SetTypingRequest struct {
	Peer   store.Peer
	Action store.TypingAction
}

type LoadMoreMsg struct {
	ChatID   int64
	OffsetID int
}

type ChatModel struct {
	chat            *store.Chat
	msgList         *components.MessageList
	composer        *components.Composer
	width           int
	height          int
	focused         bool
	composerFocused bool
	replyToMsgID    int
	editMsgID       int
	spinner         components.Spinner
	loading         bool
	logo            components.LogoLoader
	typingBase      string
	typingDots      components.TypingDots
	lastTypingAt    time.Time
}

func NewChatModel(width, height int) *ChatModel {
	composer := components.NewComposer(width)
	listH := height - composer.VisualHeight()
	if listH < 1 {
		listH = 1
	}
	ml := components.NewMessageList(listH, width)
	ml.SetShowIndicator(true)
	logo := components.NewLogoLoader(width)
	return &ChatModel{
		msgList:  ml,
		composer: composer,
		width:    width,
		height:   height,
		logo:     logo,
	}
}

// SetLoading shows or hides the loading spinner in the chat pane.
func (m *ChatModel) SetLoading(v bool) { m.loading = v }

// TickSpinner advances the spinner frame. Called by root on SpinnerTickMsg.
func (m *ChatModel) TickSpinner() { m.spinner.Tick() }

// TickLogo advances the chat-pane idle logo. Called by root on LogoTickMsg.
func (m *ChatModel) TickLogo() { m.logo.Tick() }

func (m *ChatModel) SetChat(chat *store.Chat) {
	m.typingBase = ""
	m.lastTypingAt = time.Time{}
	m.chat = chat
	if chat != nil {
		m.msgList.SetIsGroup(chat.Peer.IsGroup() || chat.Peer.IsChannel())
		m.msgList.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
	} else {
		m.msgList.SetIsGroup(false)
		m.msgList.SetOutboxReadMaxID(0)
	}
}
func (m *ChatModel) SetMessages(msgs []store.Message) { m.msgList.SetMessages(msgs) }
func (m *ChatModel) SetMessagesKeepScroll(msgs []store.Message) {
	m.msgList.SetMessagesKeepScroll(msgs)
}
func (m *ChatModel) RemoveMessage(id int)                       { m.msgList.RemoveMessage(id) }
func (m *ChatModel) PrependMessages(older []store.Message)      { m.msgList.PrependMessages(older) }
func (m *ChatModel) SetImage(photoID int64, img image.Image)    { m.msgList.SetImage(photoID, img) }
func (m *ChatModel) SetKnownImages(cache map[int64]image.Image) { m.msgList.SetKnownImages(cache) }
func (m *ChatModel) SetRenderer(r media.Renderer)               { m.msgList.SetRenderer(r) }
func (m *ChatModel) PhotoContentCols() int                      { return m.msgList.PhotoContentCols() }
func (m *ChatModel) SetOutboxReadMaxID(id int)                  { m.msgList.SetOutboxReadMaxID(id) }
func (m *ChatModel) SetInboxReadMaxID(id int)                   { m.msgList.SetInboxReadMaxID(id) }
func (m *ChatModel) ScrollToFirstUnread(readMaxID int) bool {
	return m.msgList.ScrollToFirstUnread(readMaxID)
}
func (m *ChatModel) VisibleReadMaxID() int            { return m.msgList.VisibleReadMaxID() }
func (m *ChatModel) ComposerFocused() bool            { return m.composerFocused }
func (m *ChatModel) ComposerValue() string            { return m.composer.Value() }
func (m *ChatModel) ComposerHeight() int              { return m.composer.VisualHeight() }
func (m *ChatModel) SelectedMessageID() int           { return m.msgList.SelectedMessageID() }
func (m *ChatModel) SelectedMessageIsOut() bool       { return m.msgList.SelectedMessageIsOut() }
func (m *ChatModel) SelectedMessageReplyToMsgID() int { return m.msgList.SelectedMessageReplyToMsgID() }
func (m *ChatModel) SelectedMessagePhotoID() int64    { return m.msgList.SelectedMessagePhotoID() }
func (m *ChatModel) ScrollToMessage(id int) bool      { return m.msgList.ScrollToMessage(id) }
func (m *ChatModel) ReplyToMsgID() int                { return m.replyToMsgID }
func (m *ChatModel) EditMsgID() int                   { return m.editMsgID }

// SetTypingLabel sets the active typing label and resets the animation frame.
func (m *ChatModel) SetTypingLabel(base string) {
	m.typingBase = base
	m.typingDots = components.TypingDots{}
}

// ClearTypingLabel removes the typing indicator.
func (m *ChatModel) ClearTypingLabel() { m.typingBase = "" }

// IsTyping reports whether a typing indicator is currently active.
func (m *ChatModel) IsTyping() bool { return m.typingBase != "" }

// TickTypingDots advances the dots animation by one frame.
func (m *ChatModel) TickTypingDots() { m.typingDots.Tick() }

// TypingLabel returns the animated typing label, or "" if no typing is active.
func (m *ChatModel) TypingLabel() string { return m.typingDots.View(m.typingBase) }

func (m *ChatModel) SetDarkBackground(isDark bool) {
	m.composer.SetDarkBackground(isDark)
	m.logo.SetDarkBackground(isDark)
	m.msgList.SetDarkBackground(isDark)
}

func (m *ChatModel) clearPendingAction() {
	if m.editMsgID != 0 {
		m.composer.Reset()
	} else {
		m.composer.ClearReplyPreview()
	}
	m.replyToMsgID = 0
	m.editMsgID = 0
}

// ClearPendingAction clears any active reply (or future forward) state.
func (m *ChatModel) ClearPendingAction() {
	m.clearPendingAction()
	m.syncMsgListHeight()
}

// SetEdit activates edit mode. Clears any existing pending action first.
func (m *ChatModel) SetEdit(msgID int, preview string) {
	m.clearPendingAction()
	m.editMsgID = msgID
	m.composer.SetReplyPreview(preview)
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
	m.msgList.SetShowIndicator(false)
	return m.composer.Focus()
}

func (m *ChatModel) Context() keys.Context { return keys.ContextChat }
func (m *ChatModel) Focused() bool         { return m.focused }
func (m *ChatModel) SetFocused(f bool)     { m.focused = f }
func (m *ChatModel) SetComposerValue(v string) {
	m.composer.SetValue(v)
	m.syncMsgListHeight()
}

func (m *ChatModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.logo.SetWidth(width)
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
				m.msgList.SetShowIndicator(true)
				if !m.lastTypingAt.IsZero() && m.chat != nil {
					peer := m.chat.Peer
					m.lastTypingAt = time.Time{}
					return m, func() tea.Msg {
						return SetTypingRequest{Peer: peer, Action: store.TypingActionCancel}
					}
				}
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
			focusCmd := m.composer.Focus()
			m.msgList.SetShowIndicator(false)
			return m, focusCmd
		}
		return m, nil

	case tea.PasteMsg:
		if m.composerFocused {
			newC, cmd := m.composer.Update(msg)
			m.composer = newC
			m.syncMsgListHeight()
			return m, cmd
		}

	case tea.KeyPressMsg:
		if m.composerFocused {
			if msg.Code == tea.KeyEnter && msg.Mod == 0 {
				text := m.composer.Value()
				replyID := m.replyToMsgID
				editID := m.editMsgID
				wasTyping := !m.lastTypingAt.IsZero()
				m.clearPendingAction()
				m.composer.Reset()
				m.syncMsgListHeight()
				m.lastTypingAt = time.Time{}
				if m.chat != nil && text != "" {
					peer := m.chat.Peer
					var sendCmd tea.Cmd
					if editID != 0 {
						sendCmd = func() tea.Msg {
							return EditSendRequest{Peer: peer, MsgID: editID, Text: text}
						}
					} else {
						sendCmd = func() tea.Msg {
							return SendMsgRequest{Peer: peer, Text: text, ReplyToMsgID: replyID}
						}
					}
					if wasTyping {
						cancelCmd := func() tea.Msg {
							return SetTypingRequest{Peer: peer, Action: store.TypingActionCancel}
						}
						return m, tea.Batch(sendCmd, cancelCmd)
					}
					return m, sendCmd
				}
				return m, nil
			}
			newC, cmd := m.composer.Update(msg)
			m.composer = newC
			m.syncMsgListHeight()
			if m.chat != nil && time.Since(m.lastTypingAt) >= 4*time.Second {
				peer := m.chat.Peer
				m.lastTypingAt = time.Now()
				typingCmd := func() tea.Msg {
					return SetTypingRequest{Peer: peer, Action: store.TypingActionTyping}
				}
				if cmd != nil {
					return m, tea.Batch(cmd, typingCmd)
				}
				return m, typingCmd
			}
			return m, cmd
		}
	}
	return m, nil
}

func (m *ChatModel) View() string {
	if m.loading {
		listH := m.height - m.composer.VisualHeight()
		if listH < 1 {
			listH = 1
		}
		centered := lipgloss.Place(m.width, listH, lipgloss.Center, lipgloss.Center, m.spinner.View()+" Loading...")
		return centered
	}
	if m.chat == nil && m.msgList.Count() == 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.logo.View())
	}
	return m.msgList.View() + "\n" + m.composer.View()
}
