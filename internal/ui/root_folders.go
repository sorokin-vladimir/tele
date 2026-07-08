package ui

import "github.com/sorokin-vladimir/tele/internal/store"

func (m RootModel) filteredChats() []store.Chat {
	if m.st == nil {
		return nil
	}
	all := m.st.Chats()

	// Archive virtual folder: only archived chats.
	if m.activeFilter != nil && m.activeFilter.ID == store.ArchiveFolderID {
		out := make([]store.Chat, 0)
		for _, c := range all {
			if c.IsArchived {
				out = append(out, c)
			}
		}
		return out
	}

	// All Chats: every non-archived chat.
	if m.activeFilter == nil {
		out := make([]store.Chat, 0, len(all))
		for _, c := range all {
			if !c.IsArchived {
				out = append(out, c)
			}
		}
		return out
	}

	// Custom filter: FolderFilter.Matches owns Telegram folder rules, including
	// whether archived chats should be excluded.
	out := make([]store.Chat, 0, len(all))
	for _, c := range all {
		if m.activeFilter.Matches(c) {
			out = append(out, c)
		}
	}
	return out
}

func (m RootModel) computeFolderUnreads() map[int]int {
	counts := make(map[int]int)
	if m.st == nil || m.folderBar == nil {
		return counts
	}
	chats := m.st.Chats()
	for _, f := range m.folderBar.Folders() {
		// All Chats has no badge; Archive intentionally shows no unread
		// count (mirrors the official client).
		if f.ID == 0 || f.ID == store.ArchiveFolderID {
			continue
		}
		chatsWithUnread := 0
		for _, c := range chats {
			if !c.IsArchived && f.Matches(c) && c.UnreadCount > 0 {
				chatsWithUnread++
			}
		}
		counts[f.ID] = chatsWithUnread
	}
	return counts
}

// syncFolderBar refreshes the folder pane's unread badges and toggles the
// Archive entry's presence based on whether any archived chat exists.
func (m RootModel) syncFolderBar() {
	if m.folderBar == nil || m.st == nil {
		return
	}
	m.folderBar.SetUnreadCounts(m.computeFolderUnreads())
	m.folderBar.SetArchivePresent(m.hasArchivedChats())
}

func (m RootModel) hasArchivedChats() bool {
	if m.st == nil {
		return false
	}
	for _, c := range m.st.Chats() {
		if c.IsArchived {
			return true
		}
	}
	return false
}
