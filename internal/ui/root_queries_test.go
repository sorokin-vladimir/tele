package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// Overlays that look at data outside the open windows ask the owner for it
// from a command and are filled when the answer lands (#278). These tests run
// the command and feed its message back, which is what the program does.

func runQuery(t *testing.T, m ui.RootModel, cmd tea.Cmd) ui.RootModel {
	t.Helper()
	require.NotNil(t, cmd, "opening the overlay must ask the owner")
	for _, msg := range drainMsgs(cmd()) {
		if msg == nil {
			continue
		}
		nm, _ := m.Update(msg)
		m = nm.(ui.RootModel)
	}
	return m
}

func TestRoot_Search_OpensAtOnceAndFillsFromTheOwner(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice"})
	st.SetChat(domain.Chat{ID: 2, Title: "Old", IsArchived: true})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)

	nm, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = nm.(ui.RootModel)
	require.True(t, m.SearchActive(), "the overlay opens before the owner answers")
	assert.Empty(t, m.Search().Results())

	m = runQuery(t, m, cmd)

	var ids []int64
	for _, r := range m.Search().Results() {
		ids = append(ids, r.ID)
	}
	assert.ElementsMatch(t, []int64{1, 2}, ids, "search looks through archived chats too")
}

func TestRoot_ForwardPicker_FillsFromTheOwner(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})

	nm, cmd := m.Update(components.ForwardMsgRequest{MsgID: 5})
	m = nm.(ui.RootModel)
	require.True(t, m.SearchActive())

	m = runQuery(t, m, cmd)

	assert.Len(t, m.Search().Results(), 2)
}

// The menu's own items come from the row under the cursor; the folders are
// asked for and add "Add to folder" when they land.
func TestRoot_ChatMenu_FoldersArriveFromTheOwner(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "A", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.SetFolderFilters([]domain.FolderFilter{{ID: 7, Title: "Work"}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = toMain(t, nm.(ui.RootModel))

	nm, cmd := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = nm.(ui.RootModel)
	require.True(t, m.ChatMenuOpen())
	assert.NotContains(t, stripSeq(m.View().Content), "Add to folder")

	m = runQuery(t, m, cmd)

	assert.Contains(t, stripSeq(m.View().Content), "Add to folder")
}

// Mute is a fact about a dialog, and the owner is asked whether there is one.
func TestRoot_Profile_WithADialog_GainsTheMuteItem(t *testing.T) {
	m, _ := newRootOnChat(t)

	nm, cmd := m.Update(components.OpenProfileRequest{UserID: 1})
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())
	assert.NotContains(t, stripSeq(m.Profile().View()), "mute", "unknown until the owner answers")

	m = runQuery(t, m, cmd)

	assert.Contains(t, stripSeq(m.Profile().View()), "mute")
}

func TestRoot_Profile_WithNoDialog_HasNoMuteItem(t *testing.T) {
	m, _ := groupWithMessageFrom(t, 9)
	ownerOf(t, m).knownUsers = map[int64]domain.User{9: bob()}

	nm, cmd := m.Update(components.OpenProfileRequest{UserID: 9})
	m = nm.(ui.RootModel)
	m = runQuery(t, m, cmd)

	require.True(t, m.ProfileOpen())
	assert.NotContains(t, stripSeq(m.Profile().View()), "mute")
}
