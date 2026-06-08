package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func (m RootModel) handleStoreEvent(msg store.Event) (RootModel, tea.Cmd) {
	if m.st == nil {
		return m, nil
	}
	switch msg.Kind {
	case store.EventNewMessage:
		m.st.AppendMessage(msg.Message)
		// Track unread in the store (single source of truth) before rebuilding the
		// list. Skip messages already covered by the read pointer — they were read
		// elsewhere and arrive via getDifference catch-up. The store value is later
		// overwritten by authoritative GetDialogs server state, so it must not be
		// shadowed by a sticky list badge.
		if msg.Message.ChatID != m.currentChatID && !msg.Message.IsOut {
			if chat, ok := m.st.GetChat(msg.Message.ChatID); ok && msg.Message.ID > chat.ReadInboxMaxID {
				m.st.IncrementChatUnread(msg.Message.ChatID)
			}
		}
		m.chatList.SetChats(m.filteredChats())
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		if msg.Message.ChatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
			return m, tea.Batch(m.markReadCmd(), m.pendingDownloadCmds([]store.Message{msg.Message}))
		}
	case store.EventReadInbox:
		m.st.UpdateChatReadMaxID(msg.ChatID, msg.ReadMaxID)
		if chat, ok := m.st.GetChat(msg.ChatID); ok {
			m.chatList.SetChatUnread(msg.ChatID, chat.UnreadCount)
		}
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
	case store.EventReadOutbox:
		m.st.UpdateChatOutboxReadMaxID(msg.ChatID, msg.ReadMaxID)
		if msg.ChatID == m.currentChatID {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chat.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
			}
		}
	case store.EventReactionsUpdate:
		m.st.UpdateMessageReactions(msg.ChatID, msg.MsgID, msg.Reactions)
		if msg.ChatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
		}
	case store.EventDeleteMessages:
		if msg.ChatID != 0 {
			m.st.RemoveMessages(msg.ChatID, msg.MsgIDs)
		} else {
			for _, chat := range m.st.Chats() {
				m.st.RemoveMessages(chat.ID, msg.MsgIDs)
			}
		}
		if msg.ChatID == 0 || msg.ChatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
		}
		m.chatList.SetChats(m.filteredChats())
	case store.EventUserPresence:
		m.st.UpdateChatOnline(msg.ChatID, msg.Online)
		m.chatList.SetChats(m.filteredChats())
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		if msg.ChatID == m.currentChatID {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chat.SetChat(&chat)
			}
		}
	case store.EventTyping:
		if msg.ChatID != m.currentChatID {
			return m, nil
		}
		label := msg.TypingAction.Label()
		if label == "" {
			m.chat.ClearTypingLabel()
			return m, nil
		}
		alreadyActive := m.chat.IsTyping()
		m.typingSerial++
		serial := m.typingSerial
		m.chat.SetTypingLabel(label)
		var cmds []tea.Cmd
		cmds = append(cmds, tea.Tick(6*time.Second, func(time.Time) tea.Msg { return clearTypingMsg{serial: serial} }))
		if !alreadyActive {
			cmds = append(cmds, typingDotsTickCmd())
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}
