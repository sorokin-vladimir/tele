package ui

import (
	"testing"

	tg "github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestMediaBuilderFor_FileBuildsForcedDocument(t *testing.T) {
	att := &pendingAttachment{name: "report.pdf", mime: "application/pdf", sendAs: store.MediaFile}
	build, ok := mediaBuilderFor(att)
	require.True(t, ok)
	media := build(&tg.InputFile{ID: 1})
	doc, ok := media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedDocument", media)
	assert.True(t, doc.ForceFile)
	assert.Equal(t, "application/pdf", doc.MimeType)
	require.Len(t, doc.Attributes, 1)
	fn, ok := doc.Attributes[0].(*tg.DocumentAttributeFilename)
	require.True(t, ok)
	assert.Equal(t, "report.pdf", fn.FileName)
}

func TestMediaBuilderFor_PhotoStillSupported(t *testing.T) {
	att := &pendingAttachment{sendAs: store.MediaPhoto}
	_, ok := mediaBuilderFor(att)
	assert.True(t, ok)
}

func TestMediaBuilderFor_VideoUnsupported(t *testing.T) {
	att := &pendingAttachment{sendAs: store.MediaVideo}
	_, ok := mediaBuilderFor(att)
	assert.False(t, ok, "video send-as is #107, not yet supported")
}

func TestComputeFolderUnreads_NoArchiveBadge_ExcludesArchived(t *testing.T) {
	st := store.NewMemory()
	// Two archived chats (one unread) and one normal unread chat in a group folder.
	st.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerGroup}, IsArchived: true, UnreadCount: 2})
	st.SetChat(store.Chat{ID: 2, Peer: store.Peer{ID: 2, Type: store.PeerGroup}, UnreadCount: 3})
	m := NewRootModel(nil, st, 50, false)
	m.folderBar.SetFolders([]store.FolderFilter{{ID: 7, Title: "Groups", Groups: true}})
	m.folderBar.SetArchivePresent(true)

	counts := m.computeFolderUnreads()
	// Archive carries no unread badge.
	_, hasArchive := counts[store.ArchiveFolderID]
	assert.False(t, hasArchive)
	// The archived chat does not inflate the Groups folder badge.
	assert.Equal(t, 1, counts[7])
}

func TestSyncFolderBar_TogglesArchivePresence(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	m := NewRootModel(nil, st, 50, false)

	m.syncFolderBar()
	for _, f := range m.folderBar.Folders() {
		require.NotEqual(t, store.ArchiveFolderID, f.ID)
	}

	st.SetChatArchived(1, true)
	m.syncFolderBar()
	last := m.folderBar.Folders()
	assert.Equal(t, store.ArchiveFolderID, last[len(last)-1].ID)
}

func TestFilteredChats_ArchiveSplit(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Normal", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.SetChat(store.Chat{ID: 2, Title: "Archived", Peer: store.Peer{ID: 2, Type: store.PeerUser}, IsArchived: true})

	m := NewRootModel(nil, st, 50, false)

	// All Chats (nil filter): archived hidden.
	m.activeFilter = nil
	got := m.filteredChats()
	require.Len(t, got, 1)
	assert.Equal(t, int64(1), got[0].ID)

	// Archive virtual folder: only archived.
	arch := store.FolderFilter{ID: store.ArchiveFolderID, Title: "Archive"}
	m.activeFilter = &arch
	got = m.filteredChats()
	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].ID)
}

func TestFilteredChats_CustomFolderIncludesArchivedExplicitPeer(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Normal", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.SetChat(store.Chat{ID: 2, Title: "Archived", Peer: store.Peer{ID: 2, Type: store.PeerChannel}, IsArchived: true})

	m := NewRootModel(nil, st, 50, false)
	f := store.FolderFilter{ID: 7, Title: "News", IncludePeers: []int64{2}}
	m.activeFilter = &f

	got := m.filteredChats()
	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].ID)
}

func TestFilteredChats_CustomFolderIncludesArchivedCategoryMatch(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(store.Chat{ID: 1, Title: "Normal", Peer: store.Peer{ID: 1, Type: store.PeerUser}})
	st.SetChat(store.Chat{ID: 2, Title: "Archived", Peer: store.Peer{ID: 2, Type: store.PeerGroup}, IsArchived: true})

	m := NewRootModel(nil, st, 50, false)
	f := store.FolderFilter{ID: 7, Title: "Groups", Groups: true}
	m.activeFilter = &f

	got := m.filteredChats()
	require.Len(t, got, 1)
	assert.Equal(t, int64(2), got[0].ID)
}
