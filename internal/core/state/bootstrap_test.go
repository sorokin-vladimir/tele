package state_test

import (
	"testing"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddDiscovered_AddsOnlyUnknownChatsAndCommits(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1, Title: "Known", UnreadCount: 3})
	var got []state.Change
	s.OnChange(func(c state.Change) { got = append(got, c) })

	added := s.AddDiscovered([]domain.Chat{
		{ID: 1, Title: "Known", UnreadCount: 0},
		{ID: 2, Title: "New", UnreadCount: 1},
	})

	assert.True(t, added)
	known, _ := st.GetChat(1)
	assert.Equal(t, 3, known.UnreadCount, "a known chat is not overwritten by an older list")
	_, ok := st.GetChat(2)
	assert.True(t, ok)
	require.Len(t, got, 1)
	assert.Equal(t, state.ChangeChatsAdded, got[0].Kind)
}

func TestAddDiscovered_NothingNewCommitsNothing(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	s.OnChange(func(state.Change) { t.Fatal("no change expected") })

	assert.False(t, s.AddDiscovered([]domain.Chat{{ID: 1}}))
}

func TestSetDialogs_WritesEveryChat(t *testing.T) {
	s, st := newState(t)

	s.SetDialogs([]domain.Chat{
		{ID: 1, Title: "A", UnreadCount: 2},
		{ID: 2, Title: "B"},
	})

	c, ok := st.GetChat(1)
	require.True(t, ok)
	assert.Equal(t, 2, c.UnreadCount)
	_, ok = st.GetChat(2)
	assert.True(t, ok)
}

func TestSetFolderFilters_WritesFilters(t *testing.T) {
	s, st := newState(t)

	s.SetFolderFilters([]domain.FolderFilter{{ID: 7, Title: "Work"}})

	got := st.FolderFilters()
	require.Len(t, got, 1)
	assert.Equal(t, 7, got[0].ID)
}
