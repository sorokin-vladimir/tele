package ui

import (
	tea "charm.land/bubbletea/v2"

	vmedia "github.com/sorokin-vladimir/tele/internal/media"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// prependOlder merges an older history chunk in front of the existing messages,
// dropping any chunk entries whose IDs already appear in existing. Duplicate
// in-flight loads (issue #120) or overlapping server pages would otherwise seed
// duplicate messages into the store that the message list rebuilds from.
func prependOlder(older, existing []store.Message) []store.Message {
	if len(existing) == 0 {
		return older
	}
	seen := make(map[int]struct{}, len(existing))
	for _, m := range existing {
		seen[m.ID] = struct{}{}
	}
	combined := make([]store.Message, 0, len(older)+len(existing))
	for _, m := range older {
		if _, dup := seen[m.ID]; dup {
			continue
		}
		combined = append(combined, m)
	}
	return append(combined, existing...)
}

// updateNetworkMsg handles messages that involve async data loading (history, media, read state).
func (m RootModel) updateNetworkMsg(msg tea.Msg) (RootModel, tea.Cmd) {
	switch msg := msg.(type) {
	case screens.OpenChatMsg:
		m.searchModel = nil
		if msg.Chat.ID == m.currentChatID {
			result, cmd := m.focusPane(FocusChat)
			return result.(RootModel), cmd
		}
		// Persist the chat we are leaving as a Telegram draft before switching
		// (#62). Captured here while currentChatID still points at the old chat.
		draftFlush := m.flushCurrentDraftCmd()
		m.currentChatID = msg.Chat.ID
		m.stopGifAnim()
		// Drop decoded GIF frames from the previous chat; they are large (up to
		// gifMaxFrames RGBA images each) and otherwise accumulate for the whole
		// session. They re-decode on demand if a GIF is selected again.
		clear(m.gifFrames)
		m.chatList.SetActiveByID(msg.Chat.ID)
		if m.onChatOpen != nil {
			m.onChatOpen(msg.Chat.ID)
		}
		// Seed the incoming chat's composer from its server-known draft, unless a
		// newer local draft is already cached for it (SeedDraft does not clobber).
		if m.st != nil {
			if c, ok := m.st.GetChat(msg.Chat.ID); ok {
				m.chat.SeedDraft(msg.Chat.Peer.ID, c.Draft)
			}
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
		// Drop the previous chat's placements; reconcile (after this update)
		// transmits the now-visible images.
		m.requestKittyReset()
		if m.tgClient != nil {
			m.chat.SetLoading(true)
			ctx := m.ctx
			client := m.tgClient
			peer := msg.Chat.Peer
			chatID := msg.Chat.ID
			limit := m.historyLimit
			historyCmd := func() tea.Msg {
				msgs, err := client.GetHistory(ctx, peer, 0, limit)
				if err != nil {
					return chatLoadErrMsg{chatID: chatID, text: "load history failed: " + err.Error()}
				}
				return ChatHistoryMsg{ChatID: chatID, Messages: msgs}
			}
			return m, tea.Batch(draftFlush, historyCmd)
		}
		return m, draftFlush

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
		// A chat may open with a GIF already selected (newest message) and its
		// thumbnail already cached from a prior visit; start its animation here
		// since no key event will.
		nm, gifCmd := m.ensureGifAnimForSelection()
		return nm, tea.Batch(nm.markReadCmd(), nm.pendingDownloadCmds(msg.Messages), gifCmd)

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
		// One in-flight "load older" fetch per chat. Without this guard, rapid
		// scroll-up fires several identical fetches whose duplicate chunks stack
		// into a repeating date-range ring (issue #120).
		if m.loadingOlderChat == msg.ChatID {
			return m, nil
		}
		chat, ok := m.st.GetChat(msg.ChatID)
		if !ok {
			return m, nil
		}
		m.loadingOlderChat = msg.ChatID
		ctx := m.ctx
		client := m.tgClient
		peer := chat.Peer
		offsetID := msg.OffsetID
		limit := m.historyLimit
		chatID := msg.ChatID
		return m, func() tea.Msg {
			msgs, err := client.GetHistory(ctx, peer, offsetID, limit)
			return historyChunkMsg{chatID: chatID, messages: msgs, err: err}
		}

	case historyChunkMsg:
		// Clear the in-flight guard for this chat regardless of outcome (error,
		// empty, or success) so subsequent scroll-up can load again.
		if msg.chatID == m.loadingOlderChat {
			m.loadingOlderChat = 0
		}
		if msg.err != nil {
			return m.handleStatusErr(StatusErrMsg{
				Text: "load history failed: " + msg.err.Error(),
				Sev:  components.SeverityWarning,
			})
		}
		if m.st != nil && msg.chatID == m.currentChatID && len(msg.messages) > 0 {
			existing := m.st.Messages(msg.chatID)
			combined := prependOlder(msg.messages, existing)
			m.st.SetMessages(msg.chatID, combined)
			m.chat.PrependMessages(msg.messages) // dedups + preserves viewport position
			return m, m.pendingDownloadCmds(msg.messages)
		}
		return m, nil

	case PhotoReadyMsg:
		// SetImage owns the cache write: it must measure whether the viewport was
		// at the bottom *before* the image grows the bubble's height. Adding to the
		// shared cache here first would defeat that snapshot (the height would have
		// already grown), so the newest message could scroll out of view. SetImage
		// writes to this same shared cache, so the image still lands in m.imageCache.
		m.chat.SetImage(msg.PhotoID, msg.Image)
		// Transmit is left to reconcile (after this update): the image is only
		// placed on the terminal if it is currently visible. If this thumbnail
		// belongs to the selected GIF, start its animation now (the default
		// newest-message selection fires no key event to trigger it).
		return m.ensureGifAnimForSelection()

	case kittyTransmittedMsg:
		// Placement is now on the terminal; advertise it so the next render emits
		// the placeholder grid over an existing placement. The bubble already
		// reserved the image's full footprint (rendered as a placeholder box), so
		// the image swaps in at the same height — no re-anchor needed.
		m.kittyStore.MarkTransmitted(msg.photoID, msg.cols)
		return m, nil

	case FullPhotoReadyMsg:
		m.fullImageCache.Add(msg.PhotoID, msg.Image)
		return m, nil

	case components.OpenInViewerRequest:
		// In-app modal. Photo modals are deferred, so a photo request is a no-op;
		// videos open in the modal (external-player fallback without Kitty+ffmpeg).
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			if useInAppVideoPlayer(m.imageMode, vmedia.HasFFmpeg()) {
				dur, sender := m.selectedVideoInfo()
				return m.openVideoModal(ref, m.chat.SelectedMessageID(), dur, sender)
			}
			return m.startDocumentOpen(ref, m.chat.SelectedMessageID(), m.selectedDownloadLabel())
		}
		return m, nil

	case components.OpenExternalRequest:
		if photoID := m.chat.SelectedMessagePhotoID(); photoID != 0 {
			return m.openPhotoExternal(photoID)
		}
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			return m.startDocumentOpen(ref, m.chat.SelectedMessageID(), m.selectedDownloadLabel())
		}
		return m, nil

	case components.DownloadFileRequest:
		return m.handleDownloadSelected()

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
