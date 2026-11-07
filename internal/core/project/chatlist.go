package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// ChatRow is one rendered chat-list row. It carries what the row draws and
// nothing more; in particular no Peer, because a client has no business holding
// an access hash and commands address chats by id.
type ChatRow struct {
	ID    int64
	Title string
	// Username is the chat's public username, without the @; empty when it
	// has none.
	Username   string
	Kind       ChatKind
	IsUser     bool
	Online     bool
	Unread     int
	Mentions   int
	Reactions  int
	UnreadMark bool
	Muted      bool
	Archived   bool
	// IsForum marks a chat opened through its topic list (#275). Its Unread is
	// how many of its topics have unread, as Telegram Desktop badges a forum.
	IsForum bool
}

// ChatKind is what a chat is to the person: whom or what they talk to there.
// A supergroup is a group; how Telegram stores one is not the person's
// business (#181).
type ChatKind string

const (
	ChatKindUser    ChatKind = "user"
	ChatKindBot     ChatKind = "bot"
	ChatKindGroup   ChatKind = "group"
	ChatKindChannel ChatKind = "channel"
)

// kindOf is the kind of chat c is.
func kindOf(c domain.Chat) ChatKind {
	switch {
	case c.Peer.IsChannel():
		return ChatKindChannel
	case c.Peer.IsGroup():
		return ChatKindGroup
	case c.IsBot:
		return ChatKindBot
	default:
		return ChatKindUser
	}
}

// FolderCounts is the folder pane's state: which folders there are, how many
// chats carry unread per folder, and whether an Archive entry should exist at
// all. The counts are functions of the whole list, which is why a client
// holding a window cannot compute them; the folders themselves are account
// state a client attaching later must still see, so they travel here rather
// than as a message of their own (#309).
type FolderCounts struct {
	Filters        []domain.FolderFilter
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
		Folders: folderCounts(r, all, filters),
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
		out.Rows = append(out.Rows, Row(c, r.Topics(c.ID)))
	}
	return out
}

// Row is how a chat reaches a client: everything a row or a menu over it shows,
// and no address. Queries answer with it too, so a client handles one shape of
// chat whether it came from a subscription or a one-off answer (#278).
//
// A forum is counted by its topics, which are passed alongside it; for any
// other chat they are ignored.
func Row(c domain.Chat, topics []domain.Topic) ChatRow {
	row := ChatRow{
		ID:         c.ID,
		Title:      c.Title,
		Username:   c.Username,
		Kind:       kindOf(c),
		IsUser:     c.Peer.IsUser(),
		Online:     c.Online,
		Unread:     c.UnreadCount,
		Mentions:   c.UnreadMentionsCount,
		Reactions:  c.UnreadReactionsCount,
		UnreadMark: c.UnreadMark,
		Muted:      c.IsMuted,
		Archived:   c.IsArchived,
		IsForum:    c.IsForum,
	}
	if c.IsForum {
		row.Unread, row.Mentions, row.Reactions = 0, 0, 0
		for _, t := range topics {
			if t.UnreadCount > 0 {
				row.Unread++
			}
			row.Mentions += t.UnreadMentionsCount
			row.Reactions += t.UnreadReactionsCount
		}
	}
	return row
}

// hasUnread reports whether a chat counts as unread for a folder: a forum when
// any of its topics is, any other chat by its own count.
func hasUnread(r Reader, c domain.Chat) bool {
	if !c.IsForum {
		return c.UnreadCount > 0
	}
	for _, t := range r.Topics(c.ID) {
		if t.UnreadCount > 0 {
			return true
		}
	}
	return false
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
func folderCounts(r Reader, all []domain.Chat, filters []domain.FolderFilter) FolderCounts {
	fc := FolderCounts{Filters: filters, Unread: make(map[int]int)}
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
			if f.Matches(c) && hasUnread(r, c) {
				n++
			}
		}
		fc.Unread[f.ID] = n
	}
	return fc
}
