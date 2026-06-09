package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// updateUIMsg handles messages that update layout, navigation, overlays, and animations.
func (m RootModel) updateUIMsg(msg tea.Msg) (RootModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.hasDarkBackground = msg.IsDark()
		m.logo.SetDarkBackground(m.hasDarkBackground)
		m.chat.SetDarkBackground(m.hasDarkBackground)
		return m, bgColorPollCmd()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.logo.SetWidth(msg.Width)
		m.statusBar.SetWidth(msg.Width)
		paneH := msg.Height - 1
		innerH := paneH - 2*borderSize
		if m.folderBar != nil && m.folderBar.HasFolders() {
			const sidebarW = 18
			_, chatlistW, chatW := layout.SplitThree(msg.Width, sidebarW, 0.30)
			m.folderBar.SetSize(sidebarW-2*borderSize, innerH)
			m.chatList.SetSize(chatlistW-2*borderSize, innerH)
			m.chat.SetSize(chatW-2*borderSize, innerH)
		} else {
			leftW, rightW := layout.SplitHorizontal(msg.Width, msg.Height, 0.30)
			m.chatList.SetSize(leftW-2*borderSize, innerH)
			m.chat.SetSize(rightW-2*borderSize, innerH)
		}
		return m, m.retransmitOnColsChange()

	case retransmitTickMsg:
		// Only the latest debounce tick performs the retransmit; earlier ones
		// were superseded by a newer width change.
		if msg.gen != m.retransmitGen {
			return m, nil
		}
		return m, m.retransmitChatCmd()

	case FolderFiltersMsg:
		if m.folderBar != nil {
			m.folderBar.SetFolders(msg.Filters)
			if m.width > 0 && m.height > 0 {
				const sidebarW = 18
				paneH := m.height - 1
				innerH := paneH - 2*borderSize
				_, chatlistW, chatW := layout.SplitThree(m.width, sidebarW, 0.30)
				m.folderBar.SetSize(sidebarW-2*borderSize, innerH)
				m.chatList.SetSize(chatlistW-2*borderSize, innerH)
				m.chat.SetSize(chatW-2*borderSize, innerH)
			}
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		return m, m.retransmitOnColsChange()

	case screens.FolderSelectedMsg:
		m.activeFilter = msg.Filter
		m.chatList.SetChats(m.filteredChats())
		m.chatList.SetActiveByID(m.currentChatID)
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		result, cmd := m.focusPane(FocusChatList)
		return result.(RootModel), cmd

	case screens.TransitionToMainMsg:
		m.screen = ScreenMain
		m.statusBar.SetVerbose(m.verbose)
		m.statusBar.SetActivePane("chatlist")
		if m.st != nil {
			m.chatList.SetChats(m.filteredChats())
		}
		if m.folderBar != nil {
			m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
		}
		return m, spinnerTickCmd()

	case screens.CloseSearchMsg:
		m.searchModel = nil
		return m, nil

	case components.JumpToMsgRequest:
		m.contextMenu = nil
		if !m.chat.ScrollToMessage(msg.MsgID) {
			m.statusBar.SetStatus("Not in buffer")
		}
		return m, nil

	case components.ReplyMsgRequest:
		m.contextMenu = nil
		return m, m.activateReply(msg.MsgID)

	case components.EditMsgRequest:
		m.contextMenu = nil
		return m, m.activateEdit(msg.MsgID)

	case components.CloseContextMenuMsg:
		m.contextMenu = nil
		m.chatMenu = nil
		return m, nil

	case components.ToggleMuteRequest, components.ToggleUnreadRequest,
		components.AddToFolderRequest, components.ToggleArchiveRequest:
		if rm, cmd, ok := m.handleChatMenuRequest(msg); ok {
			return rm, cmd
		}
		return m, nil

	case components.ReactMsgRequest:
		m.contextMenu = nil
		if m.st == nil {
			return m, nil
		}
		var chosen string
		for _, sm := range m.st.Messages(m.currentChatID) {
			if sm.ID == msg.MsgID {
				for _, r := range sm.Reactions {
					if r.IsChosen {
						chosen = r.Emoji
						break
					}
				}
				break
			}
		}
		m.reactionTargetID = msg.MsgID
		m.reactionPicker = components.NewReactionPicker(chosen)
		return m, nil

	case components.CloseReactionPickerMsg:
		m.reactionPicker = nil
		return m, nil

	case components.LogoTickMsg:
		m.logo.Tick()
		m.chat.TickLogo()
		return m, logoTickCmd()

	case components.SpinnerTickMsg:
		m.chatList.TickSpinner()
		m.chat.TickSpinner()
		if m.screen == ScreenMain {
			return m, spinnerTickCmd()
		}
		return m, nil

	case components.TypingDotsTickMsg:
		if m.chat.IsTyping() {
			m.chat.TickTypingDots()
			return m, typingDotsTickCmd()
		}
		return m, nil

	case clearTypingMsg:
		if msg.serial == m.typingSerial {
			m.chat.ClearTypingLabel()
		}
		return m, nil

	case screens.AuthRequestMsg, screens.ConnectedMsg, screens.AuthErrorMsg:
		if m.screen == ScreenLogin {
			newLogin, cmd := m.login.Update(msg)
			m.login = newLogin.(screens.LoginModel)
			if _, ok := msg.(screens.AuthRequestMsg); ok {
				m.logo.SetState(components.LogoStateStatic)
			}
			return m, cmd
		}
		return m, nil

	case tea.PasteMsg:
		if m.screen != ScreenMain {
			return m, nil
		}
		if m.searchModel != nil {
			newSearch, cmd := m.searchModel.Update(msg)
			m.searchModel = newSearch
			return m, cmd
		}
		if m.focus == FocusChat {
			newPane, cmd := m.chat.Update(msg)
			m.chat = newPane.(*screens.ChatModel)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}
