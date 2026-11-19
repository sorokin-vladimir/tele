package state

import "github.com/sorokin-vladimir/tele/internal/domain"

// SetDialogs writes the authoritative dialog list. Each SetChat rebases that
// chat's unread baseline and drops session tracking (#189), so this is the
// point where server state wins over anything counted locally.
//
// Bulk loads do not commit a Change. They replace the whole list rather than
// describing a difference, and the client is told about them by the explicit
// signal the caller sends once the load finishes. Publishing a synthetic
// per-change value here would mean inventing a kind that describes nothing.
func (s *State) SetDialogs(chats []domain.Chat) {
	for _, c := range chats {
		s.st.SetChat(c)
	}
}

// AddDiscovered stores the chats of a reload that the store still has no row
// for, and reports whether there were any. Unlike SetDialogs it commits, because
// it runs mid-session when clients are already showing the chat list, and it
// leaves known chats alone: for those the stored counters have moved with the
// update stream since the reload was asked for, and a dialog list read earlier
// would put them back.
func (s *State) AddDiscovered(chats []domain.Chat) bool {
	added := false
	for _, c := range chats {
		if _, known := s.st.GetChat(c.ID); known {
			continue
		}
		s.st.SetChat(c)
		added = true
	}
	if added {
		s.commit(Change{Kind: ChangeChatsAdded})
	}
	return added
}

// SetFolderFilters writes the account's folder filters. Like SetDialogs, a bulk
// load that commits no Change.
func (s *State) SetFolderFilters(f []domain.FolderFilter) {
	s.st.SetFolderFilters(f)
}

// RememberAddress keeps how to reach a chat the account has no dialog with. It
// commits no Change: an address is never drawn, so no projection depends on it.
func (s *State) RememberAddress(p domain.Peer) {
	s.st.RememberAddress(p)
}
