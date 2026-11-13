package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// currentHistory names what the chat pane shows: a chat, or one topic of a
// forum. Every command issued from the pane goes there (#275).
func (m RootModel) currentHistory() domain.HistoryKey {
	return domain.HistoryKey{ChatID: m.currentChatID, TopicID: m.currentTopicID}
}

// enterForum turns the middle pane into a forum's topic list. The chat list
// subscription stays open underneath, so leaving the forum shows it at once.
func (m RootModel) enterForum(chatID int64, title string) RootModel {
	if m.forum != nil && m.forum.ChatID() == chatID {
		return m
	}
	m.leaveForumSub()
	m.forum = screens.NewTopicListModel(chatID, title)
	m.forum.SetSize(m.chatList.Width(), m.chatList.Height())
	m.forum.SetFocused(m.focus == FocusChatList)
	if m.currentChatID == chatID {
		m.forum.SetActive(int64(m.currentTopicID))
	}
	m.forumCursorPending = true
	if m.owner != nil {
		offset, limit, _ := m.forum.WindowRequest()
		m.topicSub = m.owner.Subscribe(project.TopicListWindow{ChatID: chatID, Offset: offset, Limit: limit})
	}
	return m
}

// leaveForum gives the middle pane back to the chat list. Whatever is open in
// the chat pane stays open.
func (m RootModel) leaveForum() RootModel {
	m.leaveForumSub()
	m.forum = nil
	m.chatList.SetFocused(m.focus == FocusChatList)
	return m
}

func (m *RootModel) leaveForumSub() {
	if m.owner != nil && m.topicSub != 0 {
		m.owner.Unsubscribe(m.topicSub)
	}
	m.topicSub = 0
}

// syncTopicListWindow asks for a wider or shifted window when the cursor has
// moved near the edge of the one held.
func (m *RootModel) syncTopicListWindow() {
	if m.owner == nil || m.forum == nil || m.topicSub == 0 {
		return
	}
	offset, limit, changed := m.forum.WindowRequest()
	if !changed {
		return
	}
	m.owner.MoveWindow(m.topicSub, project.TopicListWindow{ChatID: m.forum.ChatID(), Offset: offset, Limit: limit})
}

func (m RootModel) handleTopicListDelta(d *project.TopicListDelta) (RootModel, tea.Cmd) {
	if m.forum == nil {
		return m, nil
	}
	switch d.Kind {
	case project.TopicListReset:
		m.forum.SetWindow(d.Contents.Offset, d.Contents.Total, d.Contents.Rows)
		if m.forumCursorPending {
			m.forumCursorPending = false
			m.placeForumCursor(d.Contents.Rows)
		}
	case project.TopicListRow:
		m.forum.SetRow(d.Row)
	}
	return m, nil
}

// placeForumCursor puts the cursor where a person coming into the forum wants
// it: on the topic they last opened there, else on the first with unread, else
// on the first.
func (m *RootModel) placeForumCursor(rows []project.TopicRow) {
	if last, ok := m.lastTopic[m.forum.ChatID()]; ok {
		m.forum.SetCursorByID(int64(last))
		return
	}
	for _, r := range rows {
		if r.Unread > 0 {
			m.forum.SetCursorByID(int64(r.ID))
			return
		}
	}
}

// openTopic opens one topic of a forum in the chat pane, the way a chat opens.
func (m RootModel) openTopic(msg screens.OpenTopicMsg) (RootModel, tea.Cmd) {
	if m.lastTopic == nil {
		m.lastTopic = make(map[int64]int)
	}
	m.lastTopic[msg.ChatID] = msg.TopicID
	if m.forum != nil && m.forum.ChatID() == msg.ChatID {
		m.forum.SetActiveByID(int64(msg.TopicID))
	}
	forumTitle := msg.Title
	if m.forum != nil {
		forumTitle = m.forum.Title() + " › " + msg.Title
	}
	return m.openHistory(domain.HistoryKey{ChatID: msg.ChatID, TopicID: msg.TopicID}, forumTitle)
}

// openHistory opens a history in the chat pane: a chat, or a topic of a forum.
// title is what the header shows until the history's own projection arrives.
func (m RootModel) openHistory(h domain.HistoryKey, title string) (RootModel, tea.Cmd) {
	if h == m.currentHistory() {
		result, cmd := m.focusPane(FocusChat)
		return result.(RootModel), cmd
	}
	// A message search of the chat being left is kept until another one is
	// opened, and this is that moment (#349).
	if m.searchMemory.chatFor != h {
		m.searchMemory.chat = nil
	}
	// Persist the history we are leaving as a Telegram draft before switching
	// (#62). Captured here while the current history still points at the old one.
	draftFlush := m.flushCurrentDraftCmd()
	m.currentChatID, m.currentTopicID = h.ChatID, h.TopicID
	m.stopGifAnim()
	// Drop decoded GIF frames from the previous chat; they are large (up to
	// gifMaxFrames RGBA images each) and otherwise accumulate for the whole
	// session. They re-decode on demand if a GIF is selected again.
	clear(m.gifFrames)
	m.chatList.SetActiveByID(h.ChatID)
	if m.owner != nil {
		m.owner.SetFocus(h)
	}
	m.chat.ClearPendingAction()
	// Paint the title immediately; everything else arrives on the
	// subscription's first delta, which is always a full Reset.
	m.chat.SetHeader(screens.ChatHeader{ChatID: h.ChatID, TopicID: h.TopicID, Title: title})
	m.chat.SetLoading(true)
	m.chat.SetKnownImages(m.imageCache)
	m.focus = FocusChat
	m.chatList.SetFocused(false)
	if m.forum != nil {
		m.forum.SetFocused(false)
	}
	m.chat.SetFocused(true)
	m.statusBar.SetActivePane("chat")
	// Drop the previous chat's placements; reconcile (after this update)
	// transmits the now-visible images.
	m.requestKittyReset()

	// The previous history's window and draft stop describing anything; the new
	// one's arrive on its opening Reset, which also reads its mentions.
	m.chatMsgs = nil
	m.chatDraft = ""
	m.readMentionsOnReset = true
	m.subscribeChat(h)
	return m, draftFlush
}

// openTopicMenu opens the menu over the topic row under the cursor. Everything
// it offers is on the row, so it opens at once.
func (m RootModel) openTopicMenu() (RootModel, tea.Cmd) {
	if m.forum == nil || m.owner == nil {
		return m, nil
	}
	row, ok := m.forum.CursorTopic()
	if !ok {
		return m, nil
	}
	m.chatMenu = components.NewTopicContextMenu(m.forum.ChatID(), row, m.keyMap)
	return m, nil
}

// forwardTopicsMsg carries a forum's topics to the forward picker, which then
// chooses among them.
type forwardTopicsMsg struct {
	chosen screens.ForwardForumChosen
	topics []project.TopicRow
	err    error
}

// handleForwardForumChosen lists the topics of the forum a forward is going
// into, since it goes into one of them (#275).
func (m RootModel) handleForwardForumChosen(msg screens.ForwardForumChosen) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	ctx, owner := m.ctx, m.owner
	return m, func() tea.Msg {
		topics, err := owner.Topics(ctx, msg.ChatID)
		return forwardTopicsMsg{chosen: msg, topics: topics, err: err}
	}
}

func (m RootModel) handleForwardTopics(msg forwardTopicsMsg) (RootModel, tea.Cmd) {
	if msg.err != nil {
		text, sev, ok := errText("forward", msg.err)
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return StatusErrMsg{Text: text, Sev: sev} }
	}
	m.searchModel = screens.NewForwardTopicPicker(msg.chosen.ChatID, msg.chosen.Title, msg.topics,
		msg.chosen.MsgID, m.width, m.height, m.keyMap)
	return m, nil
}

// historyTitle is what the chat pane's header calls a history: the chat's
// title, and in a forum the topic's after it.
func historyTitle(c project.HistoryContents) string {
	if c.TopicID == 0 {
		return c.Title
	}
	return c.Title + " › " + screens.TopicTitle(project.TopicRow{ID: c.TopicID, Title: c.TopicTitle})
}

// chatHeader is the chat pane's header for a history's contents.
func chatHeader(c project.HistoryContents) screens.ChatHeader {
	return screens.ChatHeader{
		ChatID:          c.ChatID,
		TopicID:         c.TopicID,
		Title:           historyTitle(c),
		IsUser:          c.IsUser,
		IsGroup:         c.IsGroup,
		Online:          c.Online,
		ReadOutboxMaxID: c.ReadOutboxMaxID,
	}
}
