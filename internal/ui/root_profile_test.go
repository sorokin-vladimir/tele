package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func pressProfileKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'P', Text: "P"} }

// rootOnChatList puts the cursor on one chat-list row. Reaching the main screen
// subscribes the list; its first delta is what fills it.
func rootOnChatList(t *testing.T, chat domain.Chat) ui.RootModel {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(chat)
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain).WithFocus(ui.FocusChatList)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return toMain(t, nm.(ui.RootModel))
}

func stripSeq(s string) string { return xansi.Strip(s) }

// bob is a person met in a group: the store holds no dialog for him, so the
// owner is the only thing that can say who he is.
func bob() domain.User {
	return domain.User{ID: 9, FirstName: "Bob", Username: "bob"}
}

// groupWithMessageFrom opens a group chat holding one incoming message from
// userID, with the message selected.
func groupWithMessageFrom(t *testing.T, userID int64) (ui.RootModel, store.Store) {
	t.Helper()
	m, st := newRootOnGroupChat(t, nil)
	st.AppendMessage(domain.Message{
		ID: 3, ChatID: 5, SenderID: userID, SenderName: "Bob", Text: "hi", Date: time.Now(),
	})
	nm, _ := applyHistory(t, m, st, 5)
	m = nm.(ui.RootModel)
	m.View() // lay out the list so the message becomes the selection
	return m, st
}

// --- entry points ---

func TestProfileKey_InChat_OpensOnTheMessageAuthor(t *testing.T) {
	m, _ := groupWithMessageFrom(t, 9)
	ownerOf(t, m).knownUsers = map[int64]domain.User{9: bob()}

	nm, _ := m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen(), "P on a message must open its author's profile")
	assert.Equal(t, int64(9), m.Profile().UserID())
}

func TestProfileKey_InChat_AsksTheOwnerToCompleteIt(t *testing.T) {
	m, _ := groupWithMessageFrom(t, 9)
	o := ownerOf(t, m)
	o.knownUsers = map[int64]domain.User{9: bob()}
	full := bob()
	full.Bio = "writes go"
	o.fullUsers = map[int64]domain.User{9: full}

	nm, cmd := m.Update(pressProfileKey())
	m = deliver(t, nm.(ui.RootModel), cmd)
	assert.Equal(t, []int64{9}, o.gotUsers, "opening a profile asks for the rest of it")
	require.True(t, m.ProfileOpen())
	assert.Contains(t, stripSeq(m.Profile().View()), "writes go")
}

func TestProfileKey_InPrivateChat_FallsBackToThePeer(t *testing.T) {
	// An outgoing message names no other person, so the key falls back to the
	// person on the other side of the chat rather than doing nothing.
	m, st := newRootOnChat(t)
	st.AppendMessage(domain.Message{ID: 4, ChatID: 1, IsOut: true, Text: "mine", Date: time.Now()})
	nm, _ := applyHistory(t, m, st, 1)
	m = nm.(ui.RootModel)
	m.View()

	nm, _ = m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())
	assert.Equal(t, int64(1), m.Profile().UserID())
}

func TestProfileKey_InChatList_OpensOnAPrivateRow(t *testing.T) {
	m := rootOnChatList(t, domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})

	nm, _ := m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())
	assert.Equal(t, int64(1), m.Profile().UserID())
}

func TestProfileKey_InChatList_DoesNothingOnAGroupRow(t *testing.T) {
	m := rootOnChatList(t, domain.Chat{ID: 5, Title: "Group", Peer: domain.Peer{ID: 5, Type: domain.PeerSuperGroup}})

	nm, _ := m.Update(pressProfileKey())
	m = nm.(ui.RootModel)
	assert.False(t, m.ProfileOpen(), "a group has no profile: it is not a person")
}

func TestOpenProfileRequest_FromTheMessageMenu_ClosesTheMenu(t *testing.T) {
	m, _ := groupWithMessageFrom(t, 9)
	ownerOf(t, m).knownUsers = map[int64]domain.User{9: bob()}

	nm, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = nm.(ui.RootModel)
	require.True(t, m.ContextMenuOpen())

	nm, _ = m.Update(components.OpenProfileRequest{UserID: 9})
	m = nm.(ui.RootModel)
	assert.True(t, m.ProfileOpen())
	assert.False(t, m.ContextMenuOpen(), "two overlays must not compete for the same keys")
}

func TestOpenProfile_ForAnUnknownPerson_OpensNothing(t *testing.T) {
	m, _ := newRootOnChat(t)
	nm, _ := m.Update(components.OpenProfileRequest{UserID: 4242})
	m = nm.(ui.RootModel)
	assert.False(t, m.ProfileOpen())
	assert.Contains(t, m.StatusText(), "No profile")
}

// --- completion ---

func TestProfileLoaded_ForADifferentPerson_IsIgnored(t *testing.T) {
	m, _ := newRootOnChat(t)
	nm, _ := m.Update(components.OpenProfileRequest{UserID: 1})
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())

	stale := domain.User{ID: 9, FirstName: "Bob", Bio: "wrong person"}
	nm, _ = m.Update(ui.ProfileLoadedMsgForTest(9, stale))
	m = nm.(ui.RootModel)
	assert.NotContains(t, stripSeq(m.Profile().View()), "wrong person")
}

// --- actions ---

func TestProfileOpenChat_WithADialog_OpensIt(t *testing.T) {
	m, _ := newRootOnChat(t)
	nm, cmd := m.Update(components.ProfileOpenChatRequest{UserID: 1})
	m = nm.(ui.RootModel)
	assert.False(t, m.ProfileOpen(), "opening a chat takes you elsewhere")
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(1), req.ChatID)
	assert.Equal(t, "Alice", req.Title)
	assert.Zero(t, req.Peer.ID, "the owner holds this chat, so it needs no peer")
}

func TestProfileOpenChat_WithNoDialog_CarriesThePeer(t *testing.T) {
	// A person never messaged has no dialog and no history: nothing but a send
	// can address them, so the chat opens on a peer and lands composable.
	m, _ := groupWithMessageFrom(t, 9)
	ownerOf(t, m).knownUsers = map[int64]domain.User{9: bob()}

	_, cmd := m.Update(components.ProfileOpenChatRequest{UserID: 9})
	require.NotNil(t, cmd)
	req, ok := cmd().(screens.OpenChatMsg)
	require.True(t, ok)
	assert.Equal(t, int64(9), req.ChatID)
	assert.Equal(t, "Bob", req.Title)
	assert.Equal(t, int64(9), req.Peer.ID)
	assert.True(t, req.Peer.IsUser())
}

func TestProfileMute_GoesThroughTheSameOwnerCommandAsTheChatMenu(t *testing.T) {
	m, st := newRootOnChat(t)
	_, cmd := m.Update(components.ProfileMuteRequest{UserID: 1, Muted: true})
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	chat, ok := st.GetChat(1)
	require.True(t, ok)
	assert.True(t, chat.IsMuted, "the profile and the chat menu cannot disagree about muting")
}

func TestProfileCopyUsername_NamesWhatItCopied(t *testing.T) {
	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()

	m, _ := newRootOnChat(t)
	nm, cmd := m.Update(components.ProfileCopyUsernameRequest{Username: "@alice"})
	m = nm.(ui.RootModel)
	assert.False(t, m.ProfileOpen())
	require.NotNil(t, cmd)

	for _, msg := range drainMsgs(cmd()) {
		nm, _ = m.Update(msg)
		m = nm.(ui.RootModel)
	}
	assert.Equal(t, "@alice", got)
	assert.Contains(t, m.StatusText(), "@alice", "in an overlay of several strings, say which one went")
}

func TestProfile_OwnsEveryKeyWhileOpen(t *testing.T) {
	m, _ := newRootOnChat(t)
	nm, _ := m.Update(components.OpenProfileRequest{UserID: 1})
	m = nm.(ui.RootModel)
	require.True(t, m.ProfileOpen())

	// "i" would drop the chat pane into insert mode if the profile were not
	// exclusive.
	nm, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = nm.(ui.RootModel)
	assert.True(t, m.ProfileOpen())

	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = nm.(ui.RootModel)
	require.NotNil(t, cmd)
	nm, _ = m.Update(cmd())
	assert.False(t, nm.(ui.RootModel).ProfileOpen())
}
