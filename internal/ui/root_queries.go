package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// Owner queries are asked from a command and answered by a message, never
// called from Update: in v2 each is a round trip to another process, and the
// interface must not freeze while one is out. The overlay that asked opens at
// once and is filled when the answer lands (#278).

// searchChatsMsg carries the chat list to an open search or forward overlay.
type searchChatsMsg struct {
	chats []project.ChatRow
	err   error
}

// chatMenuFoldersMsg carries the folders to the chat menu opened over chatID.
type chatMenuFoldersMsg struct {
	chatID  int64
	folders []domain.FolderFilter
	err     error
}

// profileDialogMsg says whether the client has a dialog with the person an
// open profile is about, which is what decides its mute item.
type profileDialogMsg struct {
	userID int64
	row    project.ChatRow
	found  bool
	err    error
}

// openSearch opens the search overlay and asks for the chats it looks through.
func (m RootModel) openSearch() (RootModel, tea.Cmd) {
	if m.owner == nil {
		return m, nil
	}
	m.searchModel = screens.NewSearchModel(nil, m.width, m.height, m.keyMap)
	return m, m.searchChatsCmd()
}

// openForwardPicker opens the chat picker in forward mode for msgID and asks
// for the chats it offers.
func (m RootModel) openForwardPicker(msgID int) (RootModel, tea.Cmd) {
	if m.owner == nil || msgID == 0 {
		return m, nil
	}
	m.contextMenu = nil
	m.searchModel = screens.NewForwardPicker(nil, msgID, m.width, m.height, m.keyMap)
	return m, m.searchChatsCmd()
}

func (m RootModel) searchChatsCmd() tea.Cmd {
	ctx, owner := m.ctx, m.owner
	return func() tea.Msg {
		chats, err := owner.Chats(ctx)
		return searchChatsMsg{chats: chats, err: err}
	}
}

// handleSearchChats fills the overlay that asked. One that was closed in the
// meantime has nothing to fill; a search and a forward picker ask the same
// question, so an answer never lands in the wrong kind of overlay.
func (m RootModel) handleSearchChats(msg searchChatsMsg) (RootModel, tea.Cmd) {
	if msg.err != nil {
		return m, func() tea.Msg { return errStatus("chats", msg.err) }
	}
	if m.searchModel != nil {
		m.searchModel.SetChats(msg.chats)
	}
	return m, nil
}

// openChatMenu opens the menu over the chat-list row under the cursor. The row
// carries everything the main items need; the folders are asked for.
func (m RootModel) openChatMenu() (RootModel, tea.Cmd) {
	row, ok := m.chatList.CursorChat()
	if !ok || m.owner == nil {
		return m, nil
	}
	m.chatMenu = components.NewChatContextMenu(row, nil, m.keyMap)
	ctx, owner, chatID := m.ctx, m.owner, row.ID
	return m, func() tea.Msg {
		folders, err := owner.FolderFilters(ctx)
		return chatMenuFoldersMsg{chatID: chatID, folders: folders, err: err}
	}
}

// handleChatMenuFolders fills the menu that asked, and only that one.
func (m RootModel) handleChatMenuFolders(msg chatMenuFoldersMsg) (RootModel, tea.Cmd) {
	if msg.err != nil {
		return m, func() tea.Msg { return errStatus("folders", msg.err) }
	}
	if m.chatMenu != nil && m.chatMenu.ChatID() == msg.chatID {
		m.chatMenu.SetFolders(msg.folders)
	}
	return m, nil
}

// profileDialogCmd asks whether the client has a dialog with userID.
func profileDialogCmd(ctx context.Context, owner Owner, userID int64) tea.Cmd {
	return func() tea.Msg {
		row, found, err := owner.Chat(ctx, userID)
		return profileDialogMsg{userID: userID, row: row, found: found, err: err}
	}
}

// handleProfileDialog gives an open profile its mute item when there is a
// dialog to mute. A failed answer leaves the profile without one, which is
// what it opened with: the item is offered only when it is known to apply.
func (m RootModel) handleProfileDialog(msg profileDialogMsg) (RootModel, tea.Cmd) {
	if msg.err != nil || m.profile == nil || m.profile.UserID() != msg.userID {
		return m, nil
	}
	if msg.found && msg.row.IsUser {
		m.profile.SetDialog(true, msg.row.Muted)
	}
	return m, nil
}
