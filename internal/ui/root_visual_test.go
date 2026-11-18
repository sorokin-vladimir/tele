package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func visualDate(minute int) time.Time {
	return time.Date(2026, 11, 18, 9, minute, 0, 0, time.Local)
}

func visualMessages(n int) []domain.Message {
	out := make([]domain.Message, n)
	for i := range out {
		out[i] = domain.Message{ID: i + 1, ChatID: 1, SenderID: 42, SenderName: "Bob", Text: "m" + string(rune('1'+i)), Date: visualDate(i)}
	}
	return out
}

func visualRoot(t *testing.T, msgs ...domain.Message) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, ReadInboxMaxID: len(msgs) + 100})
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	st.SetMessages(1, msgs)
	cfg := testConfig(50)
	cfg.CLI.NameWidth = 20
	m := ui.NewRootModel(false).WithConfig(cfg).WithOwner(newTestOwner(st, 50)).WithScreen(ui.ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	ready := project.AccountContents{
		Phase:        project.AccountReady,
		HistoryPage:  50,
		LoggedInUser: project.LoggedInUser{ID: 77, Name: "Ada Lovelace"},
	}
	nm, _ = m.Update(accountIs(m, ready))
	m = openChat(t, nm.(ui.RootModel), 1, "Alice")
	nm, _ = m.Update(accountIs(m, project.AccountContents{HistoryPage: 50, LoggedInUser: ready.LoggedInUser}))
	m = nm.(ui.RootModel)
	m.View()
	return m, st
}

func pressKeys(m ui.RootModel, texts ...string) ui.RootModel {
	for _, text := range texts {
		m, _ = key(m, text)
	}
	return m
}

func leaveWithEsc(m ui.RootModel) ui.RootModel {
	nm, _ := m.Update(pressEsc())
	return nm.(ui.RootModel)
}

func copiedBy(t *testing.T, m ui.RootModel, text string) (ui.RootModel, string) {
	t.Helper()
	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()
	m, cmd := key(m, text)
	require.NotNil(t, cmd, "%s copies", text)
	drainMsgs(cmd())
	return m, got
}

func TestVisual_VThenKThenYCopiesTheRangeOldestFirst(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(4)...)

	m = pressKeys(m, "V")
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	m = pressKeys(m, "k", "k")
	m, got := copiedBy(t, m, "y")

	assert.Equal(t, "m2\n\nm3\n\nm4", got)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_LowercaseVIsLeftFree(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "v")
	assert.Equal(t, keys.ModeNormal, m.VimMode())
}

func TestVisual_YCopiesTheTextAsTelegramSentIt(t *testing.T) {
	msgs := visualMessages(3)
	msgs[1].Text = "first line\nsecond line"
	msgs[2].Text = ""
	msgs[2].Media = &domain.MediaRef{Kind: domain.MediaPhoto}
	msgs[2].Photo = &domain.PhotoRef{ID: 5}
	m, _ := visualRoot(t, msgs...)

	m = pressKeys(m, "V", "k", "k")
	_, got := copiedBy(t, m, "y")

	assert.Equal(t, "m1\n\nfirst line\nsecond line", got)
}

func TestVisual_MFromNormalSelectsOnlyTheFocusedMessage(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)

	m = pressKeys(m, "m", "k")
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	_, got := copiedBy(t, m, "y")
	assert.Equal(t, "m3", got)
}

func TestVisual_VInsideASetAddsARange(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(6)...)

	m = pressKeys(m, "m", "k", "k", "V", "k")
	_, got := copiedBy(t, m, "y")

	assert.Equal(t, "m3\n\nm4\n\nm6", got)
}

func TestVisual_EscLeavesAndClearsTheSelectionAndKeepsTheChatOpen(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)

	m = pressKeys(m, "V", "k")
	m = leaveWithEsc(m)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
	assert.Equal(t, int64(1), m.CurrentChatID())

	_, got := copiedBy(t, m, "y")
	assert.Equal(t, "m2", got, "y copies the focused message again")
}

func TestVisual_KeysOutsideTheModeDoNothing(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k")

	for _, k := range []string{"r", "e", "d", "x", "i", "a", "f", "t", "o", "P"} {
		var cmd tea.Cmd
		m, cmd = key(m, k)
		assert.Nil(t, cmd, "%s does nothing", k)
		assert.Equal(t, keys.ModeVisual, m.VimMode(), "%s keeps Visual mode", k)
	}
	assert.Equal(t, 0, m.Chat().ReplyToMsgID())
	assert.False(t, m.Chat().ComposerFocused())
	assert.Len(t, m.Chat().VisualSelection(), 2)
}

func TestVisual_MovingFocusOutOfTheChatLeavesTheMode(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k", "h")

	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_SwitchingChatsLeavesTheMode(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k")

	nm, _ := m.Update(screens.OpenChatMsg{ChatID: 2, Title: "Bob"})
	m = nm.(ui.RootModel)

	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_DeletedMessagesLeaveTheRestSelected(t *testing.T) {
	m, st := visualRoot(t, visualMessages(6)...)
	m = pressKeys(m, "m", "k", "k", "m", "k", "m")

	nm, _ := applyEvent(t, m, st, store.Event{Kind: store.EventDeleteMessages, ChatID: 1, MsgIDs: []int{3}})
	m = nm.(ui.RootModel)
	nm, _ = applyEvent(t, m, st, store.Event{Kind: store.EventDeleteMessages, ChatID: 1, MsgIDs: []int{1}})
	m = nm.(ui.RootModel)

	assert.Equal(t, keys.ModeVisual, m.VimMode())
	assert.Equal(t, 4, m.Chat().SelectedMessageID())
	_, got := copiedBy(t, m, "y")
	assert.Equal(t, "m4\n\nm6", got)
}

func TestVisual_GAndGGStretchTheRangeToTheEnds(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(5)...)
	m = pressKeys(m, "k", "k", "V", "G")
	assert.Len(t, m.Chat().VisualSelection(), 3)

	m = pressKeys(m, "g", "g")
	assert.Len(t, m.Chat().VisualSelection(), 3)
	assert.Equal(t, 1, m.Chat().SelectedMessageID())
}

func TestVisual_AnAlbumIsCopiedAsOneItem(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, ChatID: 1, SenderID: 42, SenderName: "Bob", Text: "look", Date: visualDate(0)},
		{ID: 2, ChatID: 1, SenderID: 42, SenderName: "Bob", GroupedID: 7, Text: "the view", Photo: &domain.PhotoRef{ID: 21}, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Date: visualDate(1)},
		{ID: 3, ChatID: 1, SenderID: 42, SenderName: "Bob", GroupedID: 7, Photo: &domain.PhotoRef{ID: 22}, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Date: visualDate(1)},
	}
	m, _ := visualRoot(t, msgs...)

	m = pressKeys(m, "V")
	_, got := copiedBy(t, m, "y")
	assert.Equal(t, "the view", got)

	m = pressKeys(m, "V", "k")
	_, got = copiedBy(t, m, "y")
	assert.Equal(t, "look\n\nthe view", got)
}

func TestVisual_StatusBarShowsVisualUntilItEnds(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	assert.Contains(t, m.View().Content, "NORMAL")

	m = pressKeys(m, "V")
	assert.Contains(t, m.View().Content, "VISUAL")

	m = leaveWithEsc(m)
	assert.Contains(t, m.View().Content, "NORMAL")
}
