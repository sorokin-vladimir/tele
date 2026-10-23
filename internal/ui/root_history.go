package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// updateNetworkMsg handles messages that involve async data loading (history, media, read state).
func (m RootModel) updateNetworkMsg(msg tea.Msg) (RootModel, tea.Cmd) {
	switch msg := msg.(type) {
	case screens.OpenChatMsg:
		m.searchModel = nil
		if msg.IsForum {
			// A forum opens through its topic list: it has no history of its
			// own. A notification names the topic it came from, and opens that.
			m.chatList.SetActiveByID(msg.ChatID)
			m = m.enterForum(msg.ChatID, msg.Title)
			if msg.TopicID != 0 {
				return m.openTopic(screens.OpenTopicMsg{ChatID: msg.ChatID, TopicID: msg.TopicID, Title: msg.TopicTitle})
			}
			result, cmd := m.focusPane(FocusChatList)
			return result.(RootModel), cmd
		}
		// A chat that is not a forum is never the forum on screen, so opening
		// it is leaving that forum (#307).
		m = m.leaveForum()
		return m.openHistory(domain.HistoryKey{ChatID: msg.ChatID}, msg.Title)

	case screens.OpenTopicMsg:
		m.searchModel = nil
		return m.openTopic(msg)

	case screens.LeaveForumMsg:
		return m.leaveForum(), nil

	case screens.LoadMoreMsg:
		// Reaching the top of the window asks the owner to widen it. Whether the
		// extra messages come from the store or from Telegram is the owner's
		// business, and one fetch per subscription is in flight at a time (#120).
		if msg.ChatID != m.currentChatID {
			return m, nil
		}
		m.widenChatWindow()
		return m, nil

	case PhotoReadyMsg:
		// SetImage owns the cache write: it must measure whether the viewport was
		// at the bottom *before* the image grows the bubble's height. Adding to the
		// shared cache here first would defeat that snapshot (the height would have
		// already grown), so the newest message could scroll out of view. SetImage
		// writes to this same shared cache, so the image still lands in m.imageCache.
		m.chat.SetImage(msg.PhotoID, msg.Image)
		// A modal opened before the preview was cached is showing its spinner:
		// hand it the image now, resizing the box to it and (in sixel mode)
		// re-encoding at the new geometry.
		if pv := m.photoViewer; pv != nil && pv.photoID == msg.PhotoID && pv.img == nil {
			pv.img = msg.Image
			b := msg.Image.Bounds()
			pv.cols, pv.rows = m.modalImageBox(b.Dx(), b.Dy(), 0)
			if m.imageMode == media.ModeSixel {
				m.dropSixel()
				return m, m.encodeSixelCmd(photoPlayerKey, msg.Image, pv.cols, pv.rows)
			}
		}
		// Transmit is left to reconcile (after this update): the image is only
		// placed on the terminal if it is currently visible. If this thumbnail
		// belongs to the selected GIF, start its animation now (the default
		// newest-message selection fires no key event to trigger it).
		return m.ensureGifAnimForSelection()

	case kittyEncodedMsg:
		// The encode succeeded. Write the placement to the terminal first, then
		// mark the image ready (kittyTransmittedMsg), so the placeholder grid is
		// only painted once the placement exists. A failed encode never reaches
		// here, so the image is never falsely marked ready (#95).
		seq := msg.seq
		photoID, cols := msg.photoID, msg.cols
		return m, tea.Sequence(
			func() tea.Msg { return tea.Raw(seq)() },
			func() tea.Msg { return kittyTransmittedMsg{photoID: photoID, cols: cols} },
		)

	case kittyTransmittedMsg:
		// Placement is now on the terminal; advertise it so the next render emits
		// the placeholder grid over an existing placement. The bubble already
		// reserved the image's full footprint (rendered as a placeholder box), so
		// the image swaps in at the same height — no re-anchor needed.
		m.kittyStore.MarkTransmitted(msg.photoID, msg.cols)
		return m, nil

	case sixelEncodedMsg:
		// A sixel is painted by the terminal, not referenced by cells: store the
		// sequence and paint it after the frame that first shows the modal's
		// reserved blank cells.
		return m.handleSixelEncoded(msg)

	case sixelPaintMsg:
		return m.handleSixelPaint(msg)

	case inlineSixelTickMsg:
		// Inline previews: paint the images the last frame left dirty, after the
		// frame that rewrote their cells.
		return m.handleInlineSixelTick()

	case inlineSixelEncodedMsg:
		return m.handleInlineSixelEncoded(msg)

	case inlineSixelClearedMsg:
		return m.handleInlineSixelCleared(msg)

	case FullPhotoReadyMsg:
		m.fullImageCache.Add(msg.PhotoID, msg.Image)
		return m.handleFullPhotoReady(msg)

	case components.OpenInViewerRequest:
		// The menu "Open" item mirrors the o key: open the sole target directly or
		// present the picker when a message has several (media plus links).
		model, cmd := m.handleOpen()
		return model.(RootModel), cmd

	case components.OpenExternalRequest:
		if photoID := m.chat.SelectedMessagePhotoID(); photoID != 0 {
			return m.openPhotoExternal(photoID)
		}
		if _, ok := m.chat.SelectedMessageVideo(); ok {
			return m.startDocumentOpen(m.chat.SelectedMessageID(), m.selectedDownloadLabel())
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
