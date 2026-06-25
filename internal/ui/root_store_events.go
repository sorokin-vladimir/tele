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
		unreadChanged := false
		incomingOther := msg.Message.ChatID != m.currentChatID && !msg.Message.IsOut
		if incomingOther {
			if chat, ok := m.st.GetChat(msg.Message.ChatID); ok && msg.Message.ID > chat.ReadInboxMaxID {
				m.st.IncrementChatUnread(msg.Message.ChatID)
				unreadChanged = true
			}
		}
		m.chatList.SetChats(m.filteredChats())
		// Folder unread counts only depend on per-chat unread; recompute solely
		// when this message actually bumped a chat's unread count.
		if m.folderBar != nil && unreadChanged {
			m.syncFolderBar()
		}
		// Flash the row of a non-open chat that just bumped to the top so the eye
		// can follow the reorder (issue #39, second section).
		var highlightCmd tea.Cmd
		if incomingOther {
			m.chatList.HighlightChat(msg.Message.ChatID)
			m.chatHighlightSerial++
			highlightCmd = chatHighlightFadeCmd(m.chatHighlightSerial)
		}
		if msg.Message.ChatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
			return m, tea.Batch(m.markReadCmd(), m.pendingDownloadCmds([]store.Message{msg.Message}))
		}
		return m, highlightCmd
	case store.EventReadInbox:
		if m.st.UpdateChatReadMaxID(msg.ChatID, msg.ReadMaxID) {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chatList.SetChatUnread(msg.ChatID, chat.UnreadCount)
			}
			if m.folderBar != nil {
				m.syncFolderBar()
			}
		}
	case store.EventReadOutbox:
		m.st.UpdateChatOutboxReadMaxID(msg.ChatID, msg.ReadMaxID)
		if msg.ChatID == m.currentChatID {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chat.SetOutboxReadMaxID(chat.ReadOutboxMaxID)
			}
		}
	case store.EventEditMessage:
		// A message was edited on another client. Update the stored text/edit
		// date and re-render the open chat in place (no history reload). Keep
		// scroll position, matching the reactions-update path.
		//
		// A nil EditDate means the converter dropped it as a hidden edit
		// (Telegram edit_hide), e.g. a reaction bump: not a real content edit.
		// Ignore it so the message is not flipped to "edited"; the reaction
		// itself arrives via EventReactionsUpdate (issue #118).
		if msg.Message.EditDate == nil {
			return m, nil
		}
		editDate := *msg.Message.EditDate
		m.st.UpdateMessageText(msg.Message.ChatID, msg.Message.ID, msg.Message.Text, editDate)
		if msg.Message.ChatID == m.currentChatID {
			m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
		}
	case store.EventDraftMessage:
		// Draft changed on another device (or cleared server-side on send). Keep
		// the store as the source of truth (#62). Reflect it live only when this
		// chat is open and the user is not actively typing — otherwise we would
		// clobber an in-progress local edit. For other chats, seed the session
		// cache without overwriting a newer unsent local draft.
		m.st.SetChatDraft(msg.ChatID, msg.Draft)
		if msg.ChatID == m.currentChatID {
			if !m.chat.ComposerFocused() {
				m.chat.SetComposerValue(msg.Draft)
			}
		} else {
			m.chat.SeedDraft(msg.ChatID, msg.Draft)
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
			// Non-channel delete: no peer context. Resolve each ID to its chat
			// via the store index instead of scanning every chat (issue #72).
			m.st.RemoveMessagesByID(msg.MsgIDs)
		}
		if msg.ChatID == 0 || msg.ChatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
		}
		m.chatList.SetChats(m.filteredChats())
	case store.EventUserPresence:
		// Presence cannot change unread counts, so the folder bar is never
		// touched here. Skip all UI work when the online state did not flip —
		// presence updates stream continuously for every online contact.
		if !m.st.UpdateChatOnline(msg.ChatID, msg.Online) {
			return m, nil
		}
		m.chatList.SetChats(m.filteredChats())
		if msg.ChatID == m.currentChatID {
			if chat, ok := m.st.GetChat(msg.ChatID); ok {
				m.chat.SetChat(&chat)
			}
		}
	case store.EventMuteUpdate:
		// Mute toggled on another device. Sync the store's single source of
		// truth, then refresh the list (mute marker) and folder bar (ExcludeMuted
		// folders depend on it). Skip all UI work when nothing actually changed.
		chat, ok := m.st.GetChat(msg.ChatID)
		if !ok || chat.IsMuted == msg.Muted {
			return m, nil
		}
		m.st.SetChatMuted(msg.ChatID, msg.Muted)
		m.chatList.SetChats(m.filteredChats())
		if m.folderBar != nil {
			m.syncFolderBar()
		}
		if msg.ChatID == m.currentChatID {
			if c, ok := m.st.GetChat(msg.ChatID); ok {
				m.chat.SetChat(&c)
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
