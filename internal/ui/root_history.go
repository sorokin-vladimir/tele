package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// updateNetworkMsg handles messages that involve async data loading (history, media, read state).
func (m RootModel) updateNetworkMsg(msg tea.Msg) (RootModel, tea.Cmd) {
	switch msg := msg.(type) {
	case screens.OpenChatMsg:
		m.searchModel = nil
		if msg.Chat.ID == m.currentChatID {
			result, cmd := m.focusPane(FocusChat)
			return result.(RootModel), cmd
		}
		m.currentChatID = msg.Chat.ID
		m.chatList.SetActiveByID(msg.Chat.ID)
		if m.onChatOpen != nil {
			m.onChatOpen(msg.Chat.ID)
		}
		m.chat.ClearPendingAction()
		m.chat.SetChat(&msg.Chat)
		if m.st != nil {
			m.chat.SetMessages(m.st.Messages(msg.Chat.ID))
		}
		m.chat.SetKnownImages(m.imageCache)
		m.focus = FocusChat
		m.chatList.SetFocused(false)
		m.chat.SetFocused(true)
		m.statusBar.SetActivePane("chat")
		retransmit := m.retransmitChatCmd()
		if m.tgClient != nil {
			m.chat.SetLoading(true)
			ctx := m.ctx
			client := m.tgClient
			peer := msg.Chat.Peer
			chatID := msg.Chat.ID
			limit := m.historyLimit
			return m, tea.Batch(retransmit, func() tea.Msg {
				msgs, err := client.GetHistory(ctx, peer, 0, limit)
				if err != nil {
					return chatLoadErrMsg{chatID: chatID, text: "load history failed: " + err.Error()}
				}
				return ChatHistoryMsg{ChatID: chatID, Messages: msgs}
			})
		}
		return m, retransmit

	case ChatHistoryMsg:
		if m.st != nil {
			m.st.SetMessages(msg.ChatID, msg.Messages)
			if msg.ChatID == m.currentChatID {
				if chat, ok := m.st.GetChat(msg.ChatID); ok {
					m.chat.SetInboxReadMaxID(chat.ReadInboxMaxID)
				}
				m.chat.SetMessages(m.st.Messages(msg.ChatID))
				m.chat.SetLoading(false)
				m.chat.SetLoadError("")
				if chat, ok := m.st.GetChat(msg.ChatID); ok && chat.UnreadCount > 0 {
					m.chat.ScrollToFirstUnread(chat.ReadInboxMaxID)
				}
			}
		}
		return m, tea.Batch(m.markReadCmd(), m.pendingDownloadCmds(msg.Messages))

	case markReadDoneMsg:
		if m.st != nil {
			m.st.UpdateChatReadMaxID(msg.chatID, msg.maxID)
			if chat, ok := m.st.GetChat(msg.chatID); ok {
				m.chatList.SetChatUnread(msg.chatID, chat.UnreadCount)
			}
		}
		return m, nil

	case screens.LoadMoreMsg:
		if m.st == nil || m.tgClient == nil {
			return m, nil
		}
		chat, ok := m.st.GetChat(msg.ChatID)
		if !ok {
			return m, nil
		}
		ctx := m.ctx
		client := m.tgClient
		peer := chat.Peer
		offsetID := msg.OffsetID
		limit := m.historyLimit
		chatID := msg.ChatID
		return m, func() tea.Msg {
			msgs, err := client.GetHistory(ctx, peer, offsetID, limit)
			if err != nil {
				return StatusErrMsg{Text: "load history failed: " + err.Error(), Sev: components.SeverityWarning}
			}
			return historyChunkMsg{chatID: chatID, messages: msgs}
		}

	case historyChunkMsg:
		if m.st != nil && msg.chatID == m.currentChatID && len(msg.messages) > 0 {
			existing := m.st.Messages(msg.chatID)
			combined := append(msg.messages, existing...)
			m.st.SetMessages(msg.chatID, combined)
			m.chat.PrependMessages(msg.messages) // preserves viewport position
			return m, m.pendingDownloadCmds(msg.messages)
		}
		return m, nil

	case PhotoReadyMsg:
		m.imageCache[msg.PhotoID] = msg.Image
		m.chat.SetImage(msg.PhotoID, msg.Image)
		return m, m.transmitPhotoCmd(msg.PhotoID, msg.Image)

	case kittyTransmittedMsg:
		// Placement is now on the terminal; advertise it so the next render emits
		// the placeholder grid over an existing placement. The bubble already
		// reserved the image's full footprint (rendered as a placeholder box), so
		// the image swaps in at the same height — no re-anchor needed.
		m.kittyStore.MarkTransmitted(msg.photoID, msg.cols)
		return m, nil

	case FullPhotoReadyMsg:
		m.fullImageCache[msg.PhotoID] = msg.Image
		return m, nil

	case components.OpenInViewerRequest:
		if msg.PhotoID != 0 {
			img := m.fullImageCache[msg.PhotoID]
			if img == nil {
				img = m.imageCache[msg.PhotoID]
			}
			if img != nil {
				go openInViewer(img, m.tmpDir)
			}
			return m, nil
		}
		// No photo on the request → a video message; open the full file in a player.
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			return m, openDocumentCmd(m.ctx, m.tgClient, m.currentPeer(), m.chat.SelectedMessageID(), ref, m.tmpDir)
		}
		return m, nil

	case components.PlayVoiceRequest:
		return m.handlePlayVoice()

	case voicePlayReadyMsg:
		if m.voicePlayer != nil {
			if err := m.voicePlayer.Play(msg.docID, msg.data); err == nil {
				return m, voiceTickCmd()
			}
		}
		return m, nil

	case voiceTickMsg:
		if m.voicePlayer == nil {
			return m, nil
		}
		docID, progress, pos, active := m.voicePlayer.State()
		if active {
			m.chat.SetVoicePlayback(docID, progress, pos)
			return m, voiceTickCmd()
		}
		m.chat.SetVoicePlayback(0, 0, 0)
		return m, nil
	}
	return m, nil
}
