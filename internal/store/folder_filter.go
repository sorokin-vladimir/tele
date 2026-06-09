package store

// ArchiveFolderID is the sentinel filter ID for the Archive virtual
// folder. Real Telegram filter IDs are positive; 0 is the "All Chats"
// sentinel, so -1 is unambiguous.
const ArchiveFolderID = -1

func (f FolderFilter) Matches(chat Chat) bool {
	// Excluded peers never match
	for _, id := range f.ExcludePeers {
		if id == chat.ID {
			return false
		}
	}

	// Explicitly included/pinned peers always match (bypass category and exclusion flags)
	for _, id := range f.IncludePeers {
		if id == chat.ID {
			return true
		}
	}
	for _, id := range f.PinnedPeers {
		if id == chat.ID {
			return true
		}
	}

	// Must match at least one category flag
	categoryMatched := (f.Contacts && chat.IsContact && !chat.IsBot) ||
		(f.NonContacts && chat.Peer.IsUser() && !chat.IsContact && !chat.IsBot) ||
		(f.Groups && chat.Peer.IsGroup()) ||
		(f.Broadcasts && chat.Peer.IsChannel()) ||
		(f.Bots && chat.IsBot)
	if !categoryMatched {
		return false
	}

	// Apply exclusion flags (category matches only; explicit peers bypass these above)
	if f.ExcludeRead && chat.UnreadCount == 0 {
		return false
	}
	if f.ExcludeMuted && chat.IsMuted {
		return false
	}
	if f.ExcludeArchived && chat.IsArchived {
		return false
	}
	return true
}
