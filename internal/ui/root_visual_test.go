package ui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestVisual_CtrlUThenCtrlDStretchesAndShrinksTheRange(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(30)...)
	m = pressKeys(m, "V")

	nm, _ := m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)
	stretched := len(m.Chat().VisualSelection())
	assert.Greater(t, stretched, 1)

	nm, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)
	assert.Len(t, m.Chat().VisualSelection(), 1)
	assert.Equal(t, keys.ModeVisual, m.VimMode())
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

func TestVisual_ShiftYCopiesWithTimesAndNamesAsTeleShowPrints(t *testing.T) {
	msgs := visualMessages(5)
	msgs[3] = domain.Message{ID: 4, ChatID: 1, IsOut: true, Text: "on my way\nsoon", ReplyToMsgID: 1, Date: visualDate(3)}
	msgs[4] = domain.Message{ID: 5, ChatID: 1, SenderID: 42, SenderName: "Bob", Text: "ok", ReplyToMsgID: 511, Date: visualDate(4)}
	m, _ := visualRoot(t, msgs...)

	m = pressKeys(m, "V", "k", "k")
	_, got := copiedBy(t, m, "Y")

	want := "" +
		"2026-11-18\n" +
		"09:02 Bob:           m3\n" +
		"09:03 Ada Lovelace:  on my way\n" +
		"                     soon\n" +
		"  ↳ Bob: m1\n" +
		"09:04 Bob:           ok\n"
	assert.Equal(t, want, got)
}

func TestVisual_MFromNormalSelectsOnlyTheFocusedMessage(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)

	m = pressKeys(m, "m", "k")
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	_, got := copiedBy(t, m, "y")
	assert.Equal(t, "m3", got)
}

func TestVisual_YOnASetWithGapsCopiesWithNames(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(5)...)

	m = pressKeys(m, "m", "k", "k", "m")
	_, got := copiedBy(t, m, "y")

	want := "" +
		"2026-11-18\n" +
		"09:02 Bob:  m3\n" +
		"09:04 Bob:  m5\n"
	assert.Equal(t, want, got)
}

func TestVisual_VInsideASetAddsARange(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(6)...)

	m = pressKeys(m, "m", "k", "k", "V", "k")
	_, got := copiedBy(t, m, "Y")

	want := "" +
		"2026-11-18\n" +
		"09:02 Bob:  m3\n" +
		"09:03 Bob:  m4\n" +
		"09:05 Bob:  m6\n"
	assert.Equal(t, want, got)
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

func TestVisual_GlobalKeysOtherThanFocusDoNothing(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k")

	presses := map[string]tea.KeyPressMsg{
		"/": {Code: '/', Text: "/"},
		"?": {Code: '?', Text: "?"},
		",": {Code: ',', Text: ","},
		"q": {Code: 'q', Text: "q"},
	}
	for name, press := range presses {
		nm, cmd := m.Update(press)
		m = nm.(ui.RootModel)
		assert.Nil(t, cmd, "%s does nothing", name)
		assert.Equal(t, keys.ModeVisual, m.VimMode(), "%s keeps Visual mode", name)
	}
	m = pressKeys(m, "g", "/")
	assert.Nil(t, m.MessageSearch(), "g / opens no search")
	assert.False(t, m.SearchActive())
	assert.False(t, m.SettingsOpen())
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	assert.Len(t, m.Chat().VisualSelection(), 2)
}

func TestVisual_CtrlCLeavesTheModeWithoutQuitting(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k")

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)

	assert.Nil(t, cmd)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_CtrlCOnTheRussianLayoutLeavesTheMode(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k")

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'с', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)

	assert.Nil(t, cmd)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_QuitReboundToAModifierKeyLeavesTheMode(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"alt+x"}},
	})
	require.Empty(t, warns)
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m.WithKeyMap(km), "V", "k")

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})
	m = nm.(ui.RootModel)

	assert.Nil(t, cmd)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, m.Chat().InVisual())
}

func TestVisual_QuitReboundToABareKeyDoesNothing(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"x"}},
	})
	require.Empty(t, warns)
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m.WithKeyMap(km), "V", "k")

	nm, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = nm.(ui.RootModel)

	assert.Nil(t, cmd)
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	assert.Len(t, m.Chat().VisualSelection(), 2)
}

func TestVisual_CtrlCAfterAChordPrefixLeavesTheMode(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(3)...)
	m = pressKeys(m, "V", "k", "g")

	nm, _ := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = nm.(ui.RootModel)
	m = pressKeys(m, "/")

	assert.Equal(t, keys.ModeNormal, m.VimMode())
	require.NotNil(t, m.MessageSearch())
	assert.False(t, m.MessageSearch().Opened().Everywhere, "the g before ctrl+c is dropped")
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
	_, got := copiedBy(t, m, "Y")
	want := "" +
		"2026-11-18\n" +
		"09:03 Bob:  m4\n" +
		"09:05 Bob:  m6\n"
	assert.Equal(t, want, got)
}

func TestVisual_TheSelectionLivesThroughAWindowReload(t *testing.T) {
	m, st := visualRoot(t, visualMessages(60)...)
	require.Equal(t, 60, m.Chat().SelectedMessageID())
	m = pressKeys(m, "m")
	for range 49 {
		m = pressKeys(m, "k")
	}
	require.Equal(t, 11, m.Chat().SelectedMessageID())
	m = pressKeys(m, "m")

	nm, _ := applyEvent(t, m, st, store.Event{Kind: store.EventDeleteMessages, ChatID: 1, MsgIDs: []int{60}})
	m = nm.(ui.RootModel)
	assert.Equal(t, keys.ModeVisual, m.VimMode(), "the window reload after the delete keeps the mode")
	assert.Equal(t, 11, m.Chat().SelectedMessageID())
	var ids []int
	for _, msg := range m.Chat().VisualSelection() {
		ids = append(ids, msg.ID)
	}
	assert.Equal(t, []int{11}, ids)
}

func TestVisual_HintsOfARangeOfferCopyWithNames(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(4)...)
	m = pressKeys(m, "V", "k")

	bar := ansi.Strip(m.View().Content)
	assert.Contains(t, bar, "VISUAL")
	assert.Contains(t, bar, hintLine("y copy", "Y copy with names", "m mark", "V range", "esc exit"))
}

func TestVisual_HintsOfASetLeaveOutCopyWithNames(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(4)...)
	m = pressKeys(m, "m", "k", "k", "m")

	bar := ansi.Strip(m.View().Content)
	assert.Contains(t, bar, hintLine("y copy", "m mark", "V range", "esc exit"))
	assert.NotContains(t, bar, "with names")
}

func TestVisual_TheMenuOfARangeOffersBothCopies(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(4)...)
	m = pressKeys(m, "V", "k", " ")

	require.True(t, m.ContextMenuOpen())
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "CopY with names")
	assert.NotContains(t, view, "Reply")

	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()
	m, cmd := key(m, "Y")
	require.NotNil(t, cmd)
	nm, copyCmd := m.Update(cmd())
	m = nm.(ui.RootModel)
	require.NotNil(t, copyCmd)
	drainMsgs(copyCmd())
	assert.Equal(t, "2026-11-18\n09:02 Bob:  m3\n09:03 Bob:  m4\n", got)
	assert.False(t, m.ContextMenuOpen())
	assert.Equal(t, keys.ModeNormal, m.VimMode())
}

func TestVisual_TheMenuOfASetOffersOnlyCopy(t *testing.T) {
	m, _ := visualRoot(t, visualMessages(4)...)
	m = pressKeys(m, "m", "k", "k", "m", " ")

	require.True(t, m.ContextMenuOpen())
	view := ansi.Strip(m.View().Content)
	assert.Contains(t, view, "Copy")
	assert.NotContains(t, view, "with names")
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
	_, got := copiedBy(t, m, "Y")
	assert.Equal(t, "2026-11-18\n09:01 Bob:  [photo] the view\n09:01 Bob:  [photo]\n", got)

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

func hintLine(hints ...string) string {
	return strings.Join(hints, " \u00b7 ")
}
