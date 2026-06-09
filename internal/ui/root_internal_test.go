package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/store"
)

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
