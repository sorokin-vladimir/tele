package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// subscribeChat opens the chat:<id> subscription for a newly opened chat,
// anchored on the first unread so the pane lands on the separator. The previous
// subscription is dropped: a window nobody looks at must stop costing.
func (m *RootModel) subscribeChat(chatID int64) {
	if m.owner == nil {
		return
	}
	if m.chatSub != 0 {
		m.owner.Unsubscribe(m.chatSub)
		m.chatSub = 0
	}
	m.chatWindow = project.ChatWindow{
		ChatID: chatID,
		Anchor: project.Anchor{Kind: project.AnchorFirstUnread},
		Before: m.historyLimit,
		After:  0,
	}
	m.chatSub = m.owner.Subscribe(m.chatWindow)
}

// widenChatWindow asks for another page above the current window. It replaces
// the whole window rather than describing an increment, so repeating it is
// harmless and equivalent to resubscribing.
func (m *RootModel) widenChatWindow() {
	if m.owner == nil || m.chatSub == 0 {
		return
	}
	m.chatWindow.Before += m.historyLimit
	m.owner.MoveWindow(m.chatSub, m.chatWindow)
}

// handleChatDelta renders one chat:<id> delta into the chat pane.
func (m RootModel) handleChatDelta(d *project.ChatDelta) (RootModel, tea.Cmd) {
	switch d.Kind {
	case project.ChatReset:
		c := d.Contents
		m.chat.SetHeader(screens.ChatHeader{
			ChatID:          c.ChatID,
			Title:           c.Title,
			IsUser:          c.IsUser,
			IsGroup:         c.IsGroup,
			Online:          c.Online,
			ReadOutboxMaxID: c.ReadOutboxMaxID,
		})
		m.chat.SeedDraft(c.ChatID, c.Draft)
		m.chat.SetInboxReadMaxID(c.ReadInboxMaxID)
		m.chatMsgs = c.Messages
		m.chat.SetMessages(m.chatMsgs)
		// After SetMessages: it rebuilds the item list wholesale, so the queue
		// has to be re-seated on top of the fresh window (#193).
		m.chat.SetOutbox(c.Outbox)
		m.chat.SetLoading(false)
		m.chat.SetLoadError("")
		if cmd := m.readReactionsOnScreen(c); cmd != nil {
			return m, cmd
		}
		// The window was anchored on the first unread: it already opens on that
		// message, so scrolling to it again would fight the anchor. Only the
		// scroll is skipped. Returning early here instead also skipped the media
		// fetch, the read pointer and the GIF animation, which left every photo
		// in a chat with unread messages blank until the user scrolled.
		anchored := c.AnchorMsgID != 0 && c.AnchorMsgID <= c.ReadInboxMaxID
		if !anchored && c.ReadInboxMaxID > 0 {
			m.chat.ScrollToFirstUnread(c.ReadInboxMaxID)
		}
		// A chat may open with a GIF already selected (newest message) and its
		// thumbnail already cached from a prior visit; start its animation here
		// since no key event will.
		nm, gifCmd := m.ensureGifAnimForSelection()
		return nm, tea.Batch(nm.markReadCmd(), nm.pendingDownloadCmds(c.Messages), gifCmd)

	case project.ChatHeaderUpdate:
		// Only the state around the window changed. The message list is left
		// alone: re-seating it would re-anchor the viewport, so a contact coming
		// online would scroll the chat you are reading back to the bottom.
		c := d.Contents
		m.chat.SetHeader(screens.ChatHeader{
			ChatID:          c.ChatID,
			Title:           c.Title,
			IsUser:          c.IsUser,
			IsGroup:         c.IsGroup,
			Online:          c.Online,
			ReadOutboxMaxID: c.ReadOutboxMaxID,
		})
		// Assigned first: the call records the count on the model, and a return
		// statement gives no guarantee that m is read after it.
		cmd := m.readReactionsOnScreen(c)
		return m, cmd

	case project.ChatOlder:
		if len(d.Messages) == 0 {
			break
		}
		m.chatMsgs = append(append([]domain.Message{}, d.Messages...), m.chatMsgs...)
		m.chat.PrependMessages(d.Messages) // dedups + preserves viewport position
		return m, m.pendingDownloadCmds(d.Messages)

	case project.ChatNewer:
		if len(d.Messages) == 0 {
			break
		}
		m.chatMsgs = append(m.chatMsgs, d.Messages...)
		m.chat.SetMessagesKeepScroll(m.chatMsgs)
		return m, m.pendingDownloadCmds(d.Messages)

	case project.ChatAppend:
		m.chatMsgs = append(m.chatMsgs, d.Message)
		// KeepScroll re-anchors to the bottom only when the viewport was already
		// there, so a message arriving while you are reading history does not
		// yank you out of it.
		m.chat.SetMessagesKeepScroll(m.chatMsgs)
		cmds := []tea.Cmd{m.markReadCmd(), m.pendingDownloadCmds([]domain.Message{d.Message})}
		if m.focus == FocusChat && d.Message.Mentioned {
			cmds = append(cmds, m.readMentionsCmd(m.currentChatID))
		}
		return m, tea.Batch(cmds...)

	case project.ChatUpdate:
		// An edit or a reaction on a message already on screen: re-render in
		// place and keep the scroll position. An arriving reaction also needs
		// marking read while the user is looking at the chat (#199).
		for i := range m.chatMsgs {
			if m.chatMsgs[i].ID == d.Message.ID {
				m.chatMsgs[i] = d.Message
				break
			}
		}
		m.chat.SetMessagesKeepScroll(m.chatMsgs)
		// A replaced media reference arrives here and nowhere else: the fetch
		// that failed is what made the owner refresh it, so without this the
		// preview stays blank until the window happens to move. Media already
		// cached costs nothing here.
		cmds := []tea.Cmd{m.pendingDownloadCmds([]domain.Message{d.Message})}
		if m.focus == FocusChat && d.Message.HasUnreadReactions {
			cmds = append(cmds, m.readReactionsCmd(m.currentChatID))
		}
		return m, tea.Batch(cmds...)

	case project.ChatRemove:
		gone := make(map[int]struct{}, len(d.MsgIDs))
		for _, id := range d.MsgIDs {
			gone[id] = struct{}{}
			m.chat.RemoveMessage(id)
		}
		kept := m.chatMsgs[:0]
		for _, msg := range m.chatMsgs {
			if _, drop := gone[msg.ID]; !drop {
				kept = append(kept, msg)
			}
		}
		m.chatMsgs = kept

	case project.ChatRead:
		// Only the outbox pointer, which moves the delivery ticks. The inbound
		// one is deliberately left where the chat was opened: opening marks the
		// chat read, and following that live would erase the unread separator
		// the moment it appeared, leaving no sign of where to start reading.
		m.chat.SetOutboxReadMaxID(d.ReadOutboxMaxID)

	case project.ChatDraft:
		// Reflect a draft synced from another device only while the user is not
		// typing — otherwise it would clobber an in-progress local edit (#62).
		if !m.chat.ComposerFocused() {
			m.chat.SetComposerValue(d.Draft)
		}

	case project.ChatOutbox:
		m.chat.SetOutbox(d.Outbox)
	}
	return m, nil
}

// readReactionsOnScreen marks a chat's reactions read when the user is looking
// at it. The count is per-chat state, so the message the reaction landed on need
// not be in the window (#199).
func (m *RootModel) readReactionsOnScreen(c project.ChatContents) tea.Cmd {
	m.chatUnreadReactions = c.UnreadReactions
	if m.focus != FocusChat || c.UnreadReactions == 0 {
		return nil
	}
	return m.readReactionsCmd(c.ChatID)
}

// handleTyping shows a composing indicator and arms its expiry. The owner sends
// no state to clear, so the client's own timeout is what ends it.
func (m RootModel) handleTyping(t core.Typing) (RootModel, tea.Cmd) {
	if t.ChatID != m.currentChatID {
		return m, nil
	}
	return m.applyTypingLabelCmd(t.Label)
}

// applyTypingLabelCmd sets the typing indicator and starts the dot animation and
// the timeout that clears a label the peer never withdrew.
func (m RootModel) applyTypingLabelCmd(label string) (RootModel, tea.Cmd) {
	if label == "" {
		m.chat.ClearTypingLabel()
		return m, nil
	}
	alreadyActive := m.chat.IsTyping()
	m.typingSerial++
	serial := m.typingSerial
	m.chat.SetTypingLabel(label)
	cmds := []tea.Cmd{
		tea.Tick(6*time.Second, func(time.Time) tea.Msg { return clearTypingMsg{serial: serial} }),
	}
	if !alreadyActive {
		cmds = append(cmds, typingDotsTickCmd())
	}
	return m, tea.Batch(cmds...)
}

// clearChatBadgesOnOpen optimistically clears a chat's unread reactions and
// mentions when it is opened, and returns the commands that reconcile that with
// the server.
//
// The commands clear the badges themselves, before their request goes out, so
// the indicators drop as soon as the chat is open and every attached client
// sees it (#198).
func (m RootModel) clearChatBadgesOnOpen(chatID int64) (reactions, mentions tea.Cmd) {
	if m.st == nil {
		return nil, nil
	}
	c, ok := m.st.GetChat(chatID)
	if !ok {
		return nil, nil
	}
	if c.UnreadReactionsCount > 0 {
		reactions = m.readReactionsCmd(c.ID)
	}
	if c.UnreadMentionsCount > 0 {
		mentions = m.readMentionsCmd(c.ID)
	}
	return reactions, mentions
}
