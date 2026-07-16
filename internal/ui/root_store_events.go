package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
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
			if msg.Message.Mentioned && m.st.ApplyUnreadMention(msg.Message.ChatID, msg.Message.ID, true) {
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
		// In-app notification: a fresh message in an inactive, unmuted chat pops a
		// top-right toast (#59), gated identically to OS notifications.
		var notifyCmd tea.Cmd
		if incomingOther && store.Notifiable(m.st, msg.Message.ChatID, m.currentChatID, msg.Message.Date, time.Now()) {
			notifyCmd = m.showInAppNotify(msg.Message)
		}
		if msg.Message.ChatID == m.currentChatID {
			m.chat.SetMessages(m.st.Messages(m.currentChatID))
			cmds := []tea.Cmd{m.markReadCmd(), m.pendingDownloadCmds([]store.Message{msg.Message})}
			if m.focus == FocusChat && msg.Message.Mentioned {
				cmds = append(cmds, m.readMentionsCmd(msg.Message.ChatID))
			}
			return m, tea.Batch(cmds...)
		}
		return m, tea.Batch(highlightCmd, notifyCmd)
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
		// (Telegram edit_hide), e.g. a reaction bump: not a real content edit, so
		// the message must not be flipped to "edited" (issue #118). But in 1:1
		// chats an incoming reaction is delivered ONLY as this hidden edit (it
		// carries the message's new reactions), not as a separate
		// UpdateMessageReactions — so apply those reactions here, otherwise peer
		// reactions never show live and only appear after a chat refresh (#160).
		if msg.Message.EditDate == nil {
			m.st.UpdateMessageReactions(msg.Message.ChatID, msg.Message.ID, msg.Message.Reactions)
			if msg.Message.ChatID == m.currentChatID {
				m.chat.SetMessagesKeepScroll(m.st.Messages(m.currentChatID))
				if m.focus == FocusChat && msg.Message.HasUnreadReactions {
					return m, m.readReactionsCmd(msg.Message.ChatID)
				}
				return m, nil
			}
			if msg.Message.HasUnreadReactions && m.st.ApplyUnreadReaction(msg.Message.ChatID, msg.Message.ID, true) {
				m.chatList.SetChats(m.filteredChats())
			}
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
			if m.focus == FocusChat && msg.ReactionsUnread {
				return m, m.readReactionsCmd(msg.ChatID)
			}
			return m, nil
		}
		if msg.ReactionsUnread && m.st.ApplyUnreadReaction(msg.ChatID, msg.MsgID, true) {
			m.chatList.SetChats(m.filteredChats())
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

// notifyOpenMsg is emitted when a notify toast is clicked: it dismisses the
// toast and opens the target chat (#59 click-to-open).
type notifyOpenMsg struct {
	chat   store.Chat
	serial int
}

// showInAppNotify adds a top-right notify toast for an incoming message in an
// inactive chat and returns its auto-dismiss command. The whole toast is a
// click target that opens the chat. Respects the notification-preview setting.
func (m RootModel) showInAppNotify(msg store.Message) tea.Cmd {
	chat, _ := m.st.GetChat(msg.ChatID)
	title := "New message"
	if chat.Title != "" {
		title = chat.Title
	}
	body := "New message"
	if m.cfg == nil || m.cfg.UI.NotificationPreview {
		body = truncatePreview(msg.Text, 100)
	}
	serial := m.toasts.Add(components.ToastNotify, title+"\n"+body)
	m.toasts.SetClick(serial, notifyOpenMsg{chat: chat, serial: serial})
	return tea.Tick(durationFor(components.SeverityInfo), func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}

// truncatePreview shortens s to at most n runes, appending an ellipsis when cut.
func truncatePreview(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
