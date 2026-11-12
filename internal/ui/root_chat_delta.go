package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// subscribeChat opens the subscription for a newly opened history - a chat, or
// a topic of a forum - anchored on the first unread so the pane lands on the
// separator. The previous subscription is dropped: a window nobody looks at
// must stop costing.
func (m *RootModel) subscribeChat(h domain.HistoryKey) {
	if m.owner == nil {
		return
	}
	if m.chatSub != 0 {
		m.owner.Unsubscribe(m.chatSub)
		m.chatSub = 0
	}
	// A jump awaited in the history being left lands nowhere, and nothing
	// counted below its screen is below this one.
	m.jumpTo = 0
	m.farNew, m.belowNew = 0, 0
	m.chatWindow = project.HistoryWindow{
		ChatID:  h.ChatID,
		TopicID: h.TopicID,
		Anchor:  project.Anchor{Kind: project.AnchorFirstUnread},
		Before:  m.historyLimit,
		After:   0,
	}
	m.chatSub = m.owner.Subscribe(m.chatWindow)
}

// widenChatWindow asks for another page above the current window. It replaces
// the whole window rather than describing an increment, so repeating it is
// harmless and equivalent to resubscribing.
func (m *RootModel) widenChatWindow() {
	w := m.chatWindow
	w.Before += m.historyLimit
	m.moveChatWindow(w)
}

// widenChatWindowDown asks for another page below the current window, for a
// window whose run does not end at the tail.
func (m *RootModel) widenChatWindowDown() {
	w := m.chatWindow
	w.After += m.historyLimit
	m.moveChatWindow(w)
}

// moveChatWindow makes w the open chat's window and asks the owner for it,
// reporting whether there was a subscription to move.
func (m *RootModel) moveChatWindow(w project.HistoryWindow) bool {
	if m.owner == nil || m.chatSub == 0 {
		return false
	}
	m.chatWindow = w
	m.owner.MoveWindow(m.chatSub, w)
	return true
}

// chatWindowAt is a window on the open history at anchor, reaching before
// messages older than it and after newer.
func (m *RootModel) chatWindowAt(anchor project.Anchor, before, after int) project.HistoryWindow {
	return project.HistoryWindow{
		ChatID: m.chatWindow.ChatID, TopicID: m.chatWindow.TopicID,
		Anchor: anchor, Before: before, After: after,
	}
}

// returnToTail takes a window away from the tail back to the newest messages:
// the window the chat opens with.
func (m *RootModel) returnToTail() {
	if m.moveChatWindow(m.chatWindowAt(project.Anchor{Kind: project.AnchorFirstUnread}, m.historyLimit, 0)) {
		m.jumpTo = 0
	}
}

// jumpToMessage moves the window onto a message, half a page either side of
// it, for a message the window does not hold: a reply's original far back.
// The message is highlighted once the window holds it (#347).
func (m *RootModel) jumpToMessage(id int) {
	was := m.chatWindow
	half := m.historyLimit / 2
	if m.moveChatWindow(m.chatWindowAt(project.Anchor{Kind: project.AnchorMessage, MsgID: id}, half, half)) {
		m.beforeJump = was
		m.jumpTo = id
	}
}

// followRun takes in where the window's run lies. A window moved onto a
// message whose run now ends at the tail becomes the window opening the chat
// gives, pinned on the message it stands on: it runs to the newest message and
// grows as messages arrive, and the messages on screen stay where they are.
func (m RootModel) followRun(c project.HistoryContents) RootModel {
	m.chatAtTail = c.AtTail
	m.farNew = c.NewCount
	m.showNewCount()
	if !c.AtTail || m.chatWindow.Anchor.Kind != project.AnchorMessage || c.AnchorMsgID == 0 {
		return m
	}
	pinned := project.Anchor{Kind: project.AnchorFirstUnread, MsgID: c.AnchorMsgID}
	m.moveChatWindow(m.chatWindowAt(pinned, m.chatWindow.Before, 0))
	return m
}

// showNewCount draws how many arrived messages are below the screen: the
// owner's count for a window far back, this client's own for the tail run,
// which is cleared once the newest message is on screen.
func (m *RootModel) showNewCount() {
	if !m.chatAtTail {
		m.belowNew = 0
		m.chat.SetNewCount(m.farNew)
		return
	}
	if m.chat.AtBottom() {
		m.belowNew = 0
	}
	m.chat.SetNewCount(m.belowNew)
}

// countArrivals counts the messages arriving below a screen scrolled up on
// the tail run: other people's only, since your own is no news to you. Called
// before the list takes them in, while the screen is still where it was.
func (m *RootModel) countArrivals(msgs []domain.Message) {
	if !m.chatAtTail || m.chat.AtBottom() {
		return
	}
	for _, msg := range msgs {
		if !msg.IsOut {
			m.belowNew++
		}
	}
}

// handleMessageGone answers the owner saying the message a jump went to is no
// longer there: the window opens on the nearest one Telegram returned, with
// nothing highlighted, or goes back where it was when there was none.
func (m RootModel) handleMessageGone(msg core.MessageGone) RootModel {
	if msg.ChatID != m.currentChatID || msg.TopicID != m.currentTopicID || msg.MsgID != m.jumpTo {
		return m
	}
	m.jumpTo = 0
	m.statusBar.SetStatus("This message is gone")
	w := m.beforeJump
	if msg.Nearest != 0 {
		w = m.chatWindow
		w.Anchor.MsgID = msg.Nearest
	}
	m.moveChatWindow(w)
	return m
}

// highlightJump starts the highlight on a message the window holds and shows.
func (m RootModel) highlightJump(id int) (RootModel, tea.Cmd) {
	m.jumpTo = 0
	m.chat.HighlightMessage(id)
	m.msgHighlightSerial++
	return m, msgHighlightFadeCmd(m.msgHighlightSerial)
}

// handleChatDelta renders one chat:<id> delta into the chat pane.
func (m RootModel) handleChatDelta(d *project.HistoryDelta) (RootModel, tea.Cmd) {
	switch d.Kind {
	case project.HistoryReset:
		c := d.Contents
		m.chat.SetHeader(chatHeader(c))
		m.chat.SeedDraft(domain.HistoryKey{ChatID: c.ChatID, TopicID: c.TopicID}, c.Draft)
		m.chatDraft = c.Draft
		m.chat.SetInboxReadMaxID(c.ReadInboxMaxID)
		m.chatMsgs = c.Messages
		m.chat.SetMessages(m.chatMsgs)
		// After SetMessages: it rebuilds the item list wholesale, so the queue
		// has to be re-seated on top of the fresh window (#193).
		m.chat.SetOutbox(c.Outbox)
		m.chat.SetLoading(false)
		m.chat.SetLoadError("")
		m = m.followRun(c)
		// A window moved onto a message lands on it: scrolled to and
		// highlighted, rather than placed at the first unread.
		var jumpCmd tea.Cmd
		landed := m.jumpTo != 0 && m.chat.ScrollToMessage(m.jumpTo)
		if landed {
			m, jumpCmd = m.highlightJump(m.jumpTo)
		}
		// Opening a chat reads its mentions, once. Reactions need no such
		// step: readReactionsOnScreen reads them on every Reset while the pane
		// is focused, and that covers the opening one too (#278).
		var mentionsCmd tea.Cmd
		if m.readMentionsOnReset {
			m.readMentionsOnReset = false
			if c.UnreadMentions > 0 {
				mentionsCmd = m.readMentionsCmd(domain.HistoryKey{ChatID: c.ChatID, TopicID: c.TopicID})
			}
		}
		if cmd := m.readReactionsOnScreen(c); cmd != nil {
			return m, tea.Batch(cmd, mentionsCmd, jumpCmd)
		}
		// The window was anchored on the first unread: it already opens on that
		// message, so scrolling to it again would fight the anchor. Only the
		// scroll is skipped. Returning early here instead also skipped the media
		// fetch, the read pointer and the GIF animation, which left every photo
		// in a chat with unread messages blank until the user scrolled.
		anchored := c.AnchorMsgID != 0 && c.AnchorMsgID <= c.ReadInboxMaxID
		if !landed && !anchored && c.ReadInboxMaxID > 0 {
			m.chat.ScrollToFirstUnread(c.ReadInboxMaxID)
		}
		// A chat may open with a GIF already selected (newest message) and its
		// thumbnail already cached from a prior visit; start its animation here
		// since no key event will.
		nm, gifCmd := m.ensureGifAnimForSelection()
		return nm, tea.Batch(nm.markReadCmd(), nm.pendingDownloadCmds(c.Messages), gifCmd, mentionsCmd, jumpCmd)

	case project.HistoryHeaderUpdate:
		// Only the state around the window changed. The message list is left
		// alone: re-seating it would re-anchor the viewport, so a contact coming
		// online would scroll the chat you are reading back to the bottom.
		c := d.Contents
		m.chat.SetHeader(chatHeader(c))
		m = m.followRun(c)
		// Assigned first: the call records the count on the model, and a return
		// statement gives no guarantee that m is read after it.
		cmd := m.readReactionsOnScreen(c)
		return m, cmd

	case project.HistoryOlder:
		if len(d.Messages) == 0 {
			break
		}
		m.chatMsgs = append(append([]domain.Message{}, d.Messages...), m.chatMsgs...)
		m.chat.PrependMessages(d.Messages) // dedups + preserves viewport position
		return m, m.pendingDownloadCmds(d.Messages)

	case project.HistoryNewer:
		if len(d.Messages) == 0 {
			break
		}
		m.chatMsgs = append(m.chatMsgs, d.Messages...)
		m.countArrivals(d.Messages)
		m.chat.SetMessagesKeepScroll(m.chatMsgs)
		m.showNewCount()
		return m, m.pendingDownloadCmds(d.Messages)

	case project.HistoryAppend:
		m.chatMsgs = append(m.chatMsgs, d.Message)
		m.countArrivals([]domain.Message{d.Message})
		// KeepScroll re-anchors to the bottom only when the viewport was already
		// there, so a message arriving while you are reading history does not
		// yank you out of it.
		m.chat.SetMessagesKeepScroll(m.chatMsgs)
		m.showNewCount()
		cmds := []tea.Cmd{m.markReadCmd(), m.pendingDownloadCmds([]domain.Message{d.Message})}
		if m.focus == FocusChat && d.Message.Mentioned {
			cmds = append(cmds, m.readMentionsCmd(m.currentHistory()))
		}
		return m, tea.Batch(cmds...)

	case project.HistoryUpdate:
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
			cmds = append(cmds, m.readReactionsCmd(m.currentHistory()))
		}
		return m, tea.Batch(cmds...)

	case project.HistoryRemove:
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

	case project.HistoryRead:
		// Only the outbox pointer, which moves the delivery ticks. The inbound
		// one is deliberately left where the chat was opened: opening marks the
		// chat read, and following that live would erase the unread separator
		// the moment it appeared, leaving no sign of where to start reading.
		m.chat.SetOutboxReadMaxID(d.ReadOutboxMaxID)

	case project.HistoryDraft:
		m.chatDraft = d.Draft
		// Reflect a draft synced from another device only while the user is not
		// typing — otherwise it would clobber an in-progress local edit (#62).
		if !m.chat.ComposerFocused() {
			m.chat.SetComposerValue(d.Draft)
		}

	case project.HistoryOutbox:
		m.chat.SetOutbox(d.Outbox)
	}
	return m, nil
}

// readReactionsOnScreen marks a chat's reactions read when the user is looking
// at it. The count is per-chat state, so the message the reaction landed on need
// not be in the window (#199).
func (m *RootModel) readReactionsOnScreen(c project.HistoryContents) tea.Cmd {
	m.chatUnreadReactions = c.UnreadReactions
	if m.focus != FocusChat || c.UnreadReactions == 0 {
		return nil
	}
	return m.readReactionsCmd(domain.HistoryKey{ChatID: c.ChatID, TopicID: c.TopicID})
}

// handleTyping shows a composing indicator and arms its expiry. The owner sends
// no state to clear, so the client's own timeout is what ends it.
func (m RootModel) handleTyping(t core.Typing) (RootModel, tea.Cmd) {
	// In a forum somebody types in one topic, and only that topic shows it.
	if t.ChatID != m.currentChatID || t.TopicID != m.currentTopicID {
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
