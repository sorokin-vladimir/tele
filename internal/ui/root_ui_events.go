package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2/compat"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// setDarkBackground records the terminal/OS theme and propagates it to the
// components whose colors depend on a dark vs light background.
func (m *RootModel) setDarkBackground(isDark bool) {
	m.hasDarkBackground = isDark
	m.logo.SetDarkBackground(isDark)
	m.chat.SetDarkBackground(isDark)
	m.chatList.SetDarkBackground(isDark)
	// compat.AdaptiveColor (context menu / hint / reaction-picker backgrounds)
	// resolves against this package-level flag, which is otherwise detected once
	// at init and never updated. Keep it in sync so those explicit light/dark
	// colors follow a runtime theme change (issue #148).
	compat.HasDarkBackground = isDark
}

// updateUIMsg handles messages that update layout, navigation, overlays, and animations.
func (m RootModel) updateUIMsg(msg tea.Msg) (RootModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.setDarkBackground(msg.IsDark())
		return m, nil

	// Unsolicited OS color-scheme reports (DEC mode 2031) arrive as raw
	// ultraviolet events; flip the theme directly with no further command.
	case uv.DarkColorSchemeEvent:
		m.setDarkBackground(true)
		return m, nil
	case uv.LightColorSchemeEvent:
		m.setDarkBackground(false)
		return m, nil

	// Fallback for terminals without mode 2031: re-read the background color
	// when the window regains focus, so a theme change made while away is
	// reflected (issue #148).
	case tea.FocusMsg:
		return m, requestBGColorCmd()
	case tea.BlurMsg:
		return m, nil

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
		// were superseded by a newer width change. Request a reset so reconcile
		// deletes the stale-width placements and re-transmits at the new width.
		if msg.gen != m.retransmitGen {
			return m, nil
		}
		m.requestKittyReset()
		return m, nil

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
			m.syncFolderBar()
		}
		return m, m.retransmitOnColsChange()

	case screens.FolderSelectedMsg:
		m.activeFilter = msg.Filter
		m.chatList.SetChats(m.filteredChats())
		m.chatList.SetActiveByID(m.currentChatID)
		if m.folderBar != nil {
			m.syncFolderBar()
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
			m.syncFolderBar()
		}
		// The spinner loop is (re)started by ensureAnimationTicks when an actual
		// spinner is active (e.g. chats still loading); no unconditional start.
		return m, nil

	case screens.CloseSearchMsg:
		m.searchModel = nil
		return m, nil

	case components.JumpToMsgRequest:
		m.contextMenu = nil
		if !m.chat.ScrollToMessage(msg.MsgID) {
			m.statusBar.SetStatus("Not in buffer")
			return m, nil
		}
		m.chat.HighlightMessage(msg.MsgID)
		m.msgHighlightSerial++
		return m, msgHighlightFadeCmd(m.msgHighlightSerial)

	case msgHighlightFadeMsg:
		// Ignore ticks from a superseded highlight.
		if msg.serial != m.msgHighlightSerial {
			return m, nil
		}
		if m.chat.StepHighlight() {
			return m, msgHighlightFadeCmd(m.msgHighlightSerial)
		}
		return m, nil

	case chatHighlightFadeMsg:
		if msg.serial != m.chatHighlightSerial {
			return m, nil
		}
		if m.chatList.StepChatHighlight() {
			return m, chatHighlightFadeCmd(m.chatHighlightSerial)
		}
		return m, nil

	case components.ReplyMsgRequest:
		m.contextMenu = nil
		return m, m.activateReply(msg.MsgID)

	case components.ForwardMsgRequest:
		return m.openForwardPicker(msg.MsgID)

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
		return m.openReactionPicker(msg.MsgID), nil

	case components.CloseReactionPickerMsg:
		m.reactionPicker = nil
		return m, nil

	case components.LogoTickMsg:
		m.logo.Tick()
		m.chat.TickLogo()
		if m.logoShouldTick() {
			return m, logoTickCmd()
		}
		m.logoTicking = false
		return m, nil

	case components.SpinnerTickMsg:
		m.chatList.TickSpinner()
		m.chat.TickSpinner()
		m.statusBar.TickDownloadSpinner()
		m.updateGifLoadingSpinner()
		m.updateVideoSpinner()
		if m.spinnerShouldTick() {
			return m, spinnerTickCmd()
		}
		m.spinnerTicking = false
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
