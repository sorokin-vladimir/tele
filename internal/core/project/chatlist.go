package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// ChatRow is one rendered chat-list row. It carries what the row draws and
// nothing more; in particular no Peer, because a client has no business holding
// an access hash and commands address chats by id.
type ChatRow struct {
	ID         int64
	Title      string
	IsUser     bool
	Online     bool
	Unread     int
	Mentions   int
	Reactions  int
	UnreadMark bool
	Muted      bool
	Archived   bool
}

// FolderCounts is the folder pane's derived state: how many chats carry unread
// per folder, and whether an Archive entry should exist at all. Both are
// functions of the whole list, which is why a client holding a window cannot
// compute them.
type FolderCounts struct {
	Unread         map[int]int
	ArchivePresent bool
}

// ChatListContents is everything a chatlist subscription currently shows.
type ChatListContents struct {
	Offset  int
	Total   int
	Rows    []ChatRow
	Folders FolderCounts
}

// BuildChatList filters the ordered chat list by the window's folder, slices out
// the window, and derives the folder counts from the whole list. Order comes
// from the reader; this function never sorts.
func BuildChatList(r Reader, w ChatListWindow) ChatListContents {
	all := r.Chats()
	filters := r.FolderFilters()
	filtered := filterChats(all, w.Folder, filters)

	out := ChatListContents{
		Offset:  w.Offset,
		Total:   len(filtered),
		Folders: folderCounts(all, filters),
	}

	start := w.Offset
	if start < 0 {
		start = 0
	}
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + w.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	out.Rows = make([]ChatRow, 0, end-start)
	for _, c := range filtered[start:end] {
		out.Rows = append(out.Rows, Row(c))
	}
	return out
}

// Row is how a chat reaches a client: everything a row or a menu over it shows,
// and no address. Queries answer with it too, so a client handles one shape of
// chat whether it came from a subscription or a one-off answer (#278).
func Row(c domain.Chat) ChatRow {
	return ChatRow{
		ID:         c.ID,
		Title:      c.Title,
		IsUser:     c.Peer.IsUser(),
		Online:     c.Online,
		Unread:     c.UnreadCount,
		Mentions:   c.UnreadMentionsCount,
		Reactions:  c.UnreadReactionsCount,
		UnreadMark: c.UnreadMark,
		Muted:      c.IsMuted,
		Archived:   c.IsArchived,
	}
}

// filterChats applies one folder to the ordered list. It is the former
// ui.filteredChats, moved here because only the holder of the whole list can
// window it.
func filterChats(all []domain.Chat, folder int, filters []domain.FolderFilter) []domain.Chat {
	out := make([]domain.Chat, 0, len(all))
	switch folder {
	case domain.ArchiveFolderID:
		for _, c := range all {
			if c.IsArchived {
				out = append(out, c)
			}
		}
	case 0:
		for _, c := range all {
			if !c.IsArchived {
				out = append(out, c)
			}
		}
	default:
		f, ok := filterByID(filters, folder)
		if !ok {
			return nil
		}
		// FolderFilter.Matches owns Telegram's folder rules, including whether
		// archived chats are excluded.
		for _, c := range all {
			if f.Matches(c) {
				out = append(out, c)
			}
		}
	}
	return out
}

func filterByID(filters []domain.FolderFilter, id int) (domain.FolderFilter, bool) {
	for _, f := range filters {
		if f.ID == id {
			return f, true
		}
	}
	return domain.FolderFilter{}, false
}

// folderCounts is the former ui.computeFolderUnreads plus ui.hasArchivedChats.
// All Chats carries no badge and Archive intentionally shows no count, matching
// the official client.
func folderCounts(all []domain.Chat, filters []domain.FolderFilter) FolderCounts {
	fc := FolderCounts{Unread: make(map[int]int)}
	for _, c := range all {
		if c.IsArchived {
			fc.ArchivePresent = true
			break
		}
	}
	for _, f := range filters {
		if f.ID == 0 || f.ID == domain.ArchiveFolderID {
			continue
		}
		n := 0
		for _, c := range all {
			if f.Matches(c) && c.UnreadCount > 0 {
				n++
			}
		}
		fc.Unread[f.ID] = n
	}
	return fc
}
