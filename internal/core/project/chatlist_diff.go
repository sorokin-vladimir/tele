package project

import "reflect"

// ChatListDeltaKind names the three things that can happen to a chatlist
// subscription. There is no "row moved": anything that moves a row is a Reset.
type ChatListDeltaKind int

const (
	// ChatListReset replaces the window's whole contents. Emitted when the
	// order, the membership, the offset or the total changed — and as the first
	// delta of every subscription, which is what makes a resubscribe a resync.
	ChatListReset ChatListDeltaKind = iota
	// ChatListRow replaces one row in place. It carries no index: the client
	// finds the row by ID, which is unambiguous because a Row is only emitted
	// while the ID sequence is unchanged.
	ChatListRow
	// ChatListFolders carries the folder pane's derived counts, which depend on
	// the whole list and therefore change independently of the window.
	ChatListFolders
)

type ChatListDelta struct {
	Kind    ChatListDeltaKind
	Offset  int
	Total   int
	Rows    []ChatRow
	Row     ChatRow
	Folders FolderCounts
}

// DiffChatList turns a pair of successive contents into the deltas that carry
// the difference. Emitting nothing for an unchanged window is the point:
// presence updates stream continuously for every online contact, and one for a
// chat nowhere near the screen must not cost a redraw.
func DiffChatList(prev, next ChatListContents) []ChatListDelta {
	var out []ChatListDelta

	if !sameRowIDs(prev.Rows, next.Rows) || prev.Offset != next.Offset || prev.Total != next.Total {
		out = append(out, ChatListDelta{
			Kind:   ChatListReset,
			Offset: next.Offset,
			Total:  next.Total,
			Rows:   next.Rows,
		})
	} else {
		for i, row := range next.Rows {
			if row != prev.Rows[i] {
				out = append(out, ChatListDelta{Kind: ChatListRow, Row: row})
			}
		}
	}

	if !sameFolderCounts(prev.Folders, next.Folders) {
		out = append(out, ChatListDelta{Kind: ChatListFolders, Folders: next.Folders})
	}
	return out
}

func sameRowIDs(a, b []ChatRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].TopicID != b[i].TopicID {
			return false
		}
	}
	return true
}

// sameFolderCounts treats a nil and an empty map or list as the same folder
// state, so a subscription's first build does not report a spurious folder
// change.
func sameFolderCounts(a, b FolderCounts) bool {
	if a.ArchivePresent != b.ArchivePresent || len(a.Unread) != len(b.Unread) {
		return false
	}
	if len(a.Filters) != len(b.Filters) || (len(a.Filters) > 0 && !reflect.DeepEqual(a.Filters, b.Filters)) {
		return false
	}
	for id, n := range a.Unread {
		if b.Unread[id] != n {
			return false
		}
	}
	return true
}
