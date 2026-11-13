package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// messageSearchMemory is what a message search window keeps between openings:
// the window for the open chat, for as long as that chat stays open, and the
// window for every chat, for as long as this window runs (#349).
type messageSearchMemory struct {
	chat    *screens.MessageSearchModel
	chatFor domain.HistoryKey
	all     *screens.MessageSearchModel
}

// openMessageSearch opens the message search window on the open chat, or on
// every chat when everywhere is set or no chat is open. A window opened before
// on the same thing is opened again as it was.
func (m RootModel) openMessageSearch(everywhere bool) (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	here := m.currentHistory()
	open := screens.MessageSearchOpen{
		Here:  project.SearchScope{ChatID: here.ChatID, TopicID: here.TopicID},
		Title: m.chat.HeaderTitle(),
	}
	if everywhere || here.ChatID == 0 {
		open.Everywhere = true
		if m.searchMemory.all == nil {
			m.searchMemory.all = screens.NewMessageSearchModel(open, m.width, m.height, m.keyMap)
		} else {
			// The chat Tab switches to is the one open now.
			m.searchMemory.all.Retarget(open).Reopen()
		}
		m.messageSearch = m.searchMemory.all
		return m, nil
	}
	if m.searchMemory.chat == nil || m.searchMemory.chatFor != here {
		m.searchMemory.chat = screens.NewMessageSearchModel(open, m.width, m.height, m.keyMap)
		m.searchMemory.chatFor = here
	} else {
		m.searchMemory.chat.Reopen()
	}
	m.messageSearch = m.searchMemory.chat
	return m, nil
}

// messageSearchPage is how many results one question asks for.
const messageSearchPage = 30

// messageSearchAnswer carries the owner's answer back to the window that
// asked, which may no longer be the open one.
type messageSearchAnswer struct {
	to  *screens.MessageSearchModel
	res screens.MessageSearchResult
}

// handleMessageSearchRequest asks the owner the open window's question. A
// question still out is cancelled: the window asks one at a time, and the
// answer to the old one would be dropped anyway.
func (m RootModel) handleMessageSearchRequest(req screens.MessageSearchRequest) (RootModel, tea.Cmd) {
	if m.owner == nil || m.messageSearch == nil {
		return m, nil
	}
	if m.cancelSearch != nil {
		m.cancelSearch()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancelSearch = cancel
	owner, to := m.owner, m.messageSearch
	return m, func() tea.Msg {
		page, err := owner.SearchMessages(ctx, req.Scope, req.Text, req.After, messageSearchPage)
		return messageSearchAnswer{to: to, res: screens.MessageSearchResult{Serial: req.Serial, Page: page, Err: err}}
	}
}

// handleMessageSearchAnswer hands the answer to the window that asked, and
// tells a failure as a toast; a cancelled question is no failure.
func (m RootModel) handleMessageSearchAnswer(a messageSearchAnswer) (RootModel, tea.Cmd) {
	a.to.Update(a.res)
	if a.res.Err == nil {
		return m, nil
	}
	toast := errStatus("search", a.res.Err)
	if toast == nil {
		return m, nil
	}
	return m, func() tea.Msg { return toast }
}

// closeMessageSearch closes the window, which stays in memory, and cancels
// its question.
func (m RootModel) closeMessageSearch() RootModel {
	if m.cancelSearch != nil {
		m.cancelSearch()
		m.cancelSearch = nil
	}
	m.messageSearch = nil
	return m
}

// handleMessageSearchJump opens a result where it was found, at the message:
// the window closes, kept in memory, and the history is opened on the message
// and highlights it once it holds it, as a reply's original is (#347). A
// message in a forum carries its topic, General included.
func (m RootModel) handleMessageSearchJump(j screens.MessageSearchJump) (RootModel, tea.Cmd) {
	m = m.closeMessageSearch()
	msg := j.Found.Message
	h := domain.HistoryKey{ChatID: msg.ChatID, TopicID: msg.TopicID}
	if h == m.currentHistory() {
		if m.chat.ScrollToMessage(msg.ID) {
			return m.highlightJump(msg.ID)
		}
		m.jumpToMessage(msg.ID)
		return m, nil
	}
	var cmd tea.Cmd
	if h.TopicID != 0 {
		m.chatList.SetActiveByID(h.ChatID)
		m = m.enterForum(h.ChatID, j.Found.Chat)
		m, cmd = m.openTopic(screens.OpenTopicMsg{ChatID: h.ChatID, TopicID: h.TopicID, Title: j.Found.Topic})
	} else {
		m = m.leaveForum()
		m, cmd = m.openHistory(h, j.Found.Chat)
	}
	m.jumpToMessage(msg.ID)
	return m, cmd
}

// MessageSearch is the open message search window, nil when none is.
func (m RootModel) MessageSearch() *screens.MessageSearchModel { return m.messageSearch }
