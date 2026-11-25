package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// handleDelta renders one projection delta. It performs no mutation and reads no
// domain state: everything it needs is on the delta, addressed to a subscription
// the client asked for.
func (m RootModel) handleDelta(d project.Delta) (RootModel, tea.Cmd) {
	switch {
	case d.ChatList != nil && d.Sub == m.chatListSub:
		return m.handleChatListDelta(d.ChatList)
	case d.History != nil && d.Sub == m.chatSub:
		return m.handleChatDelta(d.History)
	case d.TopicList != nil && d.Sub == m.topicSub:
		return m.handleTopicListDelta(d.TopicList)
	case d.Account != nil && d.Sub == m.accountSub:
		return m.handleAccountDelta(d.Account.Contents)
	}
	// A delta for a subscription this model no longer holds: the window was
	// replaced (a chat switch) and the reply raced the unsubscribe. Dropping it
	// is correct — the new subscription's Reset carries the truth.
	return m, nil
}

func (m RootModel) handleChatListDelta(d *project.ChatListDelta) (RootModel, tea.Cmd) {
	switch d.Kind {
	case project.ChatListReset:
		m.chatList.SetWindow(d.Offset, d.Total, d.Rows)
		m.chatList.SetActive(m.currentChatID)

	case project.ChatListRow:
		m.chatList.SetRow(d.Row)

	case project.ChatListFolders:
		if m.folderBar == nil {
			break
		}
		cmd := m.setFolders(d.Folders.Filters)
		m.folderBar.SetUnreadCounts(d.Folders.Unread)
		m.folderBar.SetArchivePresent(d.Folders.ArchivePresent)
		// The Archive folder was showing and just became empty (the last archived
		// chat was unarchived): fall back to All Chats so the user is not
		// stranded on an empty list.
		if m.activeFolder == domainArchiveFolderID && !d.Folders.ArchivePresent {
			next, selCmd := m.selectFolder(0)
			return next, tea.Batch(cmd, selCmd)
		}
		return m, cmd
	}
	return m, nil
}

// setFolders puts the account's folders on the folder bar and lays the panes
// out again, since a bar that gains or loses its folders changes the widths
// around it. The folders travel with the chat list (#309).
func (m *RootModel) setFolders(filters []domain.FolderFilter) tea.Cmd {
	m.folderBar.SetFolders(filters)
	if m.width > 0 && m.height > 0 {
		lay := computeLayout(m.width, m.height, m.chat.ComposerHeight(),
			m.folderBar.HasFolders())
		if lay.hasFolders {
			m.folderBar.SetSize(lay.folders.Width, lay.folders.Height)
		}
		m.chatList.SetSize(lay.chatList.Width, lay.chatList.Height)
		m.chat.SetSize(lay.messages.Width, lay.messages.Height+lay.composer.Height)
	}
	return m.retransmitOnColsChange()
}

// handleIncoming follows the reorder: the row flashes so the eye can track a
// chat moving to the top (#39). It says nothing about whether the user should be
// interrupted — that arrives separately, as a Notification.
func (m RootModel) handleIncoming(in core.Incoming) (RootModel, tea.Cmd) {
	m.chatList.HighlightChat(in.ChatID)
	if m.forum != nil && m.forum.ChatID() == in.ChatID && in.TopicID != 0 {
		m.forum.HighlightTopic(in.TopicID)
	}
	m.chatHighlightSerial++
	return m, chatHighlightFadeCmd(m.chatHighlightSerial)
}

// handleNotification renders a decision the owner has already made. There is no
// condition here on purpose: judging it a second time is the defect this
// replaced (#192).
func (m RootModel) handleNotification(n core.Notification) (RootModel, tea.Cmd) {
	return m, m.showInAppNotify(n)
}

// handleFailure renders work the owner could not finish. The owner reports what
// failed in domain terms (#191); saying it is the client's job.
func (m RootModel) handleFailure(f core.Failure) (RootModel, tea.Cmd) {
	text, sev, ok := errText(f.Op, f.Err)
	if !ok {
		// A cancelled operation is not a failure and must not blank out the
		// pane with an empty error banner.
		return m, nil
	}
	if f.Op == core.OpSend {
		// A refused send has nothing to blank out: the message is still on
		// screen, marked, and can be retried. Say it once and move on (#193).
		return m, func() tea.Msg { return StatusErrMsg{Text: text, Sev: sev} }
	}
	return m.handleChatLoadErr(chatLoadErrMsg{chatID: f.ChatID, text: text})
}

// notifyOpenMsg is emitted when a notify toast is clicked: it dismisses the
// toast and opens the target chat (#59 click-to-open).
type notifyOpenMsg struct {
	chatID int64
	// topicID is the forum topic the notification came from, 0 outside a
	// forum; opening it goes into the forum and opens that topic (#275).
	topicID    int
	title      string
	topicTitle string
	serial     int
}

// desktopBannerLife is about how long a desktop notification banner stays on
// screen (macOS shows one for about five seconds).
const desktopBannerLife = 5 * time.Second

// notifyToastLife is how long a notify toast stays. A click on the desktop
// banner only brings the terminal forward, and the toast is what opens the
// chat, so it must still be there after the banner is gone.
const notifyToastLife = desktopBannerLife + 3*time.Second

// showInAppNotify adds a top-right notify toast for a decision the owner made
// and returns its auto-dismiss command. The whole toast is a click target that
// opens the chat.
func (m RootModel) showInAppNotify(n core.Notification) tea.Cmd {
	title := n.Title
	if title == "" {
		title = "New message"
	}
	serial := m.toasts.Add(components.ToastNotify, title+"\n"+n.Body)
	m.toasts.SetClick(serial, notifyOpenMsg{chatID: n.ChatID, topicID: n.TopicID, title: n.ChatTitle, topicTitle: n.TopicTitle, serial: serial})
	return tea.Tick(notifyToastLife, func(time.Time) tea.Msg {
		return ClearStatusErrMsg{Serial: serial}
	})
}
