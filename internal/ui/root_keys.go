package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func (m RootModel) handleMainKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.statusBar.SetStatus("")
	if m.reactionPicker != nil {
		newPicker, cmd := m.reactionPicker.Update(msg)
		m.reactionPicker = newPicker
		return m, cmd
	}
	if m.openPicker != nil {
		newPicker, cmd := m.openPicker.Update(msg)
		m.openPicker = newPicker
		return m, cmd
	}
	// While context menu is open, route all keys to it.
	if m.contextMenu != nil {
		newCM, cmd := m.contextMenu.Update(msg)
		m.contextMenu = newCM
		return m, cmd
	}
	if m.chatMenu != nil {
		newCM, cmd := m.chatMenu.Update(msg)
		m.chatMenu = newCM
		return m, cmd
	}

	keyStr := msg.String()
	if m.verbose {
		m.statusBar.SetLastKey(keyStr)
	}

	// While the photo modal is open it owns all keys.
	if m.photoViewer != nil {
		return m.handlePhotoModalKey(keyStr)
	}

	// While the video modal is open it owns all keys.
	if m.videoPlayer != nil {
		return m.handleVideoPlayerKey(keyStr)
	}

	if m.searchModel != nil {
		if keyStr == "ctrl+v" {
			return m, readClipboardCmd()
		}
		newSearch, cmd := m.searchModel.Update(msg)
		m.searchModel = newSearch
		return m, cmd
	}

	// While the file picker is open it owns all keys.
	if m.filePicker != nil {
		if keyStr == "ctrl+v" {
			return m, readClipboardCmd()
		}
		newPicker, cmd := m.filePicker.Update(msg)
		m.filePicker = newPicker
		return m, cmd
	}

	// In insert mode, bypass global bindings and pass key directly to chat/composer
	if m.focus == FocusChat && m.vimState.Mode == keys.ModeInsert {
		// While the mention popup is open it owns navigation/selection keys.
		// ctrl+j/ctrl+k move the cursor without leaving insert mode, where plain
		// j/k must stay literal text.
		if m.mentionPopup != nil {
			switch keys.NormalizeKey(keyStr) {
			case "up", "down", "ctrl+j", "ctrl+k", "enter", "tab", "esc":
				newPopup, cmd := m.mentionPopup.Update(msg)
				m.mentionPopup = newPopup
				return m, cmd
			}
		}
		// Normalize so the toggle fires on the same physical key under non-Latin
		// layouts (e.g. Russian ЙЦУКЕН), like the other bindings.
		if keys.NormalizeKey(keyStr) == "ctrl+t" && m.pendingAttachment != nil {
			return m.toggleSendAs()
		}
		if keyStr == "esc" {
			// esc only leaves insert mode; a staged attachment is kept (drop it
			// explicitly with the cancel key in normal mode).
			m.vimState.Mode = keys.ModeNormal
			m.statusBar.SetMode(keys.ModeNormal)
			newPane, cmd := m.chat.Update(keys.ActionMsg{Action: keys.ActionNormal})
			m.chat = newPane.(*screens.ChatModel)
			return m, cmd
		}
		if keyStr == "ctrl+v" {
			return m, readClipboardCmd()
		}
		newPane, cmd := m.chat.Update(msg)
		m.chat = newPane.(*screens.ChatModel)
		mentionCmd := m.syncMentionPopup()
		return m, tea.Batch(cmd, mentionCmd, m.markReadCmd())
	}

	// Global bindings take priority, unless the focused context explicitly binds
	// the key — a context-specific override (e.g. chatlist "confirm: l") must win
	// over a conflicting global binding (issue #132).
	if !m.matcher.ContextOwns(m.focusedContext(), keyStr) {
		switch m.keyMap.Resolve(keys.ContextGlobal, keyStr) {
		case keys.ActionFocusChatList:
			return m.focusPane(FocusChatList)
		case keys.ActionFocusChat:
			return m.focusPane(FocusChat)
		case keys.ActionFocusPrev:
			return m.focusPrev()
		case keys.ActionFocusNext:
			return m.focusNext()
		case keys.ActionFocusFolders:
			if m.folderBar != nil && m.folderBar.HasFolders() {
				return m.focusPane(FocusFolders)
			}
			return m, nil
		case keys.ActionQuit:
			return m, tea.Quit
		}
	}

	if keyStr == "/" {
		if m.st != nil {
			m.searchModel = screens.NewSearchModel(m.st.Chats(), m.width, m.height, m.keyMap)
		}
		return m, nil
	}

	if m.focus == FocusFolders {
		action, res := m.matcher.Resolve(keys.ContextFolders, keyStr)
		if res == keys.MatchPending {
			return m, nil
		}
		if action != keys.ActionNone {
			newPane, cmd := m.folderBar.Update(keys.ActionMsg{Action: action})
			m.folderBar = newPane.(*screens.FoldersModel)
			return m, cmd
		}
		return m, nil
	}

	if m.focus == FocusChatList {
		action, res := m.matcher.Resolve(keys.ContextChatList, keyStr)
		if res == keys.MatchPending {
			return m, nil
		}
		if action == keys.ActionOpenContextMenu {
			if chat, ok := m.chatList.CursorChat(); ok && m.st != nil {
				m.chatMenu = components.NewChatContextMenu(chat, m.st.FolderFilters(), m.keyMap)
			}
			return m, nil
		}
		if action != keys.ActionNone {
			newPane, cmd := m.chatList.Update(keys.ActionMsg{Action: action})
			m.chatList = newPane.(*screens.ChatListModel)
			return m, cmd
		}
		return m, nil
	}

	// Chat pane: resolve through the matcher (supports chords).
	action, res := m.matcher.Resolve(keys.ContextChat, keyStr)
	if res == keys.MatchPending {
		return m, nil
	}
	// Mode is a consequence of the resolved action.
	switch action {
	case keys.ActionInsert:
		m.vimState.Mode = keys.ModeInsert
	case keys.ActionNormal:
		m.vimState.Mode = keys.ModeNormal
	}
	m.statusBar.SetMode(m.vimState.Mode)

	// Esc in normal mode: close active chat and return to chatlist.
	if action == keys.ActionNormal && m.focus == FocusChat {
		// Persist the draft of the chat being closed before tearing it down (#62).
		draftFlush := m.flushCurrentDraftCmd()
		m.chat.ClearPendingAction()
		m.chat.SetChat(nil)
		m.chat.SetMessages(nil)
		m.currentChatID = 0
		m.chatList.SetActiveByID(0)
		result, cmd := m.focusPane(FocusChatList)
		return result, tea.Batch(draftFlush, cmd)
	}

	// ActionOpenInViewer (o) opens the message's content. A message can hold
	// several openable targets (media plus links): one opens directly, several
	// present a picker. Media opens in the in-app modal (external-player fallback
	// when Kitty+ffmpeg are unavailable); links open in the default browser.
	if action == keys.ActionOpenInViewer && m.focus == FocusChat {
		return m.handleOpen()
	}

	// ActionOpenExternal (O) opens media in the OS default app: photos in the
	// image viewer, videos in the external player.
	if action == keys.ActionOpenExternal && m.focus == FocusChat {
		if photoID := m.chat.SelectedMessagePhotoID(); photoID != 0 {
			return m.openPhotoExternal(photoID)
		}
		if ref, ok := m.chat.SelectedMessageVideo(); ok {
			return m.startDocumentOpen(ref, m.chat.SelectedMessageID(), m.selectedDownloadLabel())
		}
		return m, nil
	}

	if action == keys.ActionDownloadFile && m.focus == FocusChat {
		return m.handleDownloadSelected()
	}

	// ActionCopyMessage (y) copies the focused message's text to the clipboard.
	// Media-only messages (no caption) carry no text and are a no-op.
	if action == keys.ActionCopyMessage && m.focus == FocusChat {
		if text, ok := m.chat.SelectedMessageText(); ok {
			return m, copyToClipboardCmd(text)
		}
		return m, nil
	}

	if action == keys.ActionPlayVoice && m.focus == FocusChat {
		return m.handlePlayVoice()
	}

	if action == keys.ActionOpenContextMenu && m.focus == FocusChat {
		if m.chat != nil {
			msgID := m.chat.SelectedMessageID()
			isOut := m.chat.SelectedMessageIsOut()
			if msgID != 0 {
				replyToMsgID := m.chat.SelectedMessageReplyToMsgID()
				mediaKind, hasMedia := m.chat.SelectedMessageMediaKind()
				_, hasText := m.chat.SelectedMessageText()
				openTargets := m.chat.SelectedMessageOpenTargets()
				m.contextMenu = components.NewContextMenu(msgID, isOut, replyToMsgID, mediaKind, hasMedia, hasText, openTargets, m.keyMap)
			}
		}
		return m, nil
	}

	if action == keys.ActionAttach && m.focus == FocusChat {
		return m.openFilePicker()
	}

	if action == keys.ActionCancelUpload && m.focus == FocusChat {
		return m.handleCancelUpload()
	}

	if action == keys.ActionReply && m.focus == FocusChat {
		if m.chat != nil {
			return m, m.activateReply(m.chat.SelectedMessageID())
		}
		return m, nil
	}

	if action == keys.ActionEdit && m.focus == FocusChat {
		if m.chat != nil && m.chat.SelectedMessageIsOut() {
			return m, m.activateEdit(m.chat.SelectedMessageID())
		}
		return m, nil
	}

	if action == keys.ActionReact && m.focus == FocusChat {
		if m.chat != nil {
			return m.openReactionPicker(m.chat.SelectedMessageID()), nil
		}
		return m, nil
	}

	if action == keys.ActionForward && m.focus == FocusChat {
		if m.chat != nil {
			return m.openForwardPicker(m.chat.SelectedMessageID())
		}
		return m, nil
	}

	if action != keys.ActionNone {
		before := m.chat.SelectedMessageID()
		newPane, cmd := m.chat.Update(keys.ActionMsg{Action: action})
		m.chat = newPane.(*screens.ChatModel)
		var gifCmd tea.Cmd
		if m.chat.SelectedMessageID() != before {
			m, gifCmd = m.reconcileGifAnim()
		}
		return m, tea.Batch(cmd, m.markReadCmd(), gifCmd)
	}

	return m, nil
}
