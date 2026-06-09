package store_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
)

var (
	dmChat      = store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}, IsContact: true}
	botChat     = store.Chat{ID: 2, Peer: store.Peer{ID: 2, Type: store.PeerUser}, IsBot: true}
	groupChat   = store.Chat{ID: 3, Peer: store.Peer{ID: 3, Type: store.PeerGroup}}
	channelChat = store.Chat{ID: 4, Peer: store.Peer{ID: 4, Type: store.PeerChannel}}
	mutedDM     = store.Chat{ID: 5, Peer: store.Peer{ID: 5, Type: store.PeerUser}, IsContact: true, IsMuted: true}
	unreadDM    = store.Chat{ID: 6, Peer: store.Peer{ID: 6, Type: store.PeerUser}, IsContact: true, UnreadCount: 3}
)

func TestFolderFilter_ExcludePeer(t *testing.T) {
	f := store.FolderFilter{Contacts: true, ExcludePeers: []int64{1}}
	assert.False(t, f.Matches(dmChat), "excluded peer must not match even if category matches")
}

func TestFolderFilter_IncludePeer_BypassesCategory(t *testing.T) {
	f := store.FolderFilter{Groups: true, IncludePeers: []int64{1}}
	assert.True(t, f.Matches(dmChat), "explicitly included peer matches regardless of category flags")
}

func TestFolderFilter_PinnedPeer_BypassesCategory(t *testing.T) {
	f := store.FolderFilter{Groups: true, PinnedPeers: []int64{1}}
	assert.True(t, f.Matches(dmChat))
}

func TestFolderFilter_IncludePeer_BypassesExclusionFlags(t *testing.T) {
	f := store.FolderFilter{IncludePeers: []int64{5}, ExcludeMuted: true}
	assert.True(t, f.Matches(mutedDM), "explicitly included peer bypasses ExcludeMuted")
}

func TestFolderFilter_Contacts(t *testing.T) {
	f := store.FolderFilter{Contacts: true}
	assert.True(t, f.Matches(dmChat))
	assert.False(t, f.Matches(groupChat))
}

func TestFolderFilter_NonContacts(t *testing.T) {
	nonContact := store.Chat{ID: 7, Peer: store.Peer{ID: 7, Type: store.PeerUser}}
	f := store.FolderFilter{NonContacts: true}
	assert.True(t, f.Matches(nonContact))
	assert.False(t, f.Matches(dmChat), "contact is not a non-contact")
	assert.False(t, f.Matches(botChat), "bot is not a non-contact")
}

func TestFolderFilter_Groups(t *testing.T) {
	f := store.FolderFilter{Groups: true}
	assert.True(t, f.Matches(groupChat))
	assert.False(t, f.Matches(channelChat))
	assert.False(t, f.Matches(dmChat))
}

func TestFolderFilter_Broadcasts(t *testing.T) {
	f := store.FolderFilter{Broadcasts: true}
	assert.True(t, f.Matches(channelChat))
	assert.False(t, f.Matches(groupChat))
}

func TestFolderFilter_Bots(t *testing.T) {
	f := store.FolderFilter{Bots: true}
	assert.True(t, f.Matches(botChat))
	assert.False(t, f.Matches(dmChat))
}

func TestFolderFilter_ExcludeMuted(t *testing.T) {
	f := store.FolderFilter{Contacts: true, ExcludeMuted: true}
	assert.True(t, f.Matches(dmChat))
	assert.False(t, f.Matches(mutedDM))
}

func TestFolderFilter_ExcludeRead(t *testing.T) {
	f := store.FolderFilter{Contacts: true, ExcludeRead: true}
	assert.True(t, f.Matches(unreadDM))
	assert.False(t, f.Matches(dmChat), "read chat (UnreadCount==0) must be excluded")
}

func TestFolderFilter_SuperGroupMatchesGroups(t *testing.T) {
	superGroup := store.Chat{ID: 8, Peer: store.Peer{ID: 8, Type: store.PeerSuperGroup}}
	f := store.FolderFilter{Groups: true}
	assert.True(t, f.Matches(superGroup), "supergroup must match Groups flag")
}

func TestFolderFilter_SuperGroupDoesNotMatchBroadcasts(t *testing.T) {
	superGroup := store.Chat{ID: 8, Peer: store.Peer{ID: 8, Type: store.PeerSuperGroup}}
	f := store.FolderFilter{Broadcasts: true}
	assert.False(t, f.Matches(superGroup), "supergroup must not match Broadcasts flag")
}

func TestFolderFilter_NoFlagsNoMatches(t *testing.T) {
	f := store.FolderFilter{ID: 1, Title: "Empty"}
	assert.False(t, f.Matches(dmChat))
	assert.False(t, f.Matches(groupChat))
}

func TestMatches_ExcludeArchived(t *testing.T) {
	f := store.FolderFilter{Groups: true, ExcludeArchived: true}
	group := store.Chat{ID: 1, Peer: store.Peer{Type: store.PeerGroup}}
	assert.True(t, f.Matches(group))

	archived := store.Chat{ID: 2, Peer: store.Peer{Type: store.PeerGroup}, IsArchived: true}
	assert.False(t, f.Matches(archived))
}
