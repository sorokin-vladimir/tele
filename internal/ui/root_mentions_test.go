package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// newRootOnGroupChat builds a main-screen RootModel focused on a supergroup, so
// the @mention popup (a group-only feature) is applicable.
func newRootOnGroupChat(t *testing.T, mc *mockTGClient) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	chat := store.Chat{ID: 5, Title: "Group", Peer: store.Peer{ID: 5, Type: store.PeerSuperGroup}}
	st.SetChat(chat)
	m := ui.NewRootModel(mc, st, 50, false).WithScreen(ui.ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	nm, cmd := m.Update(screens.OpenChatMsg{Chat: chat})
	m = nm.(ui.RootModel)
	return deliver(t, m, cmd), st
}

// deliver drains a (possibly batched) cmd into the model, recursing so follow-up
// commands (e.g. fetch → participantsLoadedMsg) are applied too.
func deliver(t *testing.T, m ui.RootModel, cmd tea.Cmd) ui.RootModel {
	t.Helper()
	if cmd == nil {
		return m
	}
	for _, inner := range drainMsgs(cmd()) {
		if inner == nil {
			continue
		}
		nm, next := m.Update(inner)
		m = nm.(ui.RootModel)
		m = deliver(t, m, next)
	}
	return m
}

func typeKey(t *testing.T, m ui.RootModel, r rune) ui.RootModel {
	t.Helper()
	nm, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	m = nm.(ui.RootModel)
	return deliver(t, m, cmd)
}

func TestMentionPopupOpensAndInserts(t *testing.T) {
	mc := &mockTGClient{participants: []store.ChatMember{
		{UserID: 7, Username: "alice", DisplayName: "Alice A"},
		{UserID: 8, Username: "bob", DisplayName: "Bob B"},
	}}
	m, _ := newRootOnGroupChat(t, mc)

	// enter insert mode
	m = typeKey(t, m, 'i')

	// type "@a" -> popup opens and, after participants load, filters to alice
	m = typeKey(t, m, '@')
	m = typeKey(t, m, 'a')

	if !m.MentionPopupOpen() {
		t.Fatal("mention popup should be open after typing @a")
	}

	// select the first candidate
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(ui.RootModel)
	m = deliver(t, m, cmd)

	if m.MentionPopupOpen() {
		t.Fatal("popup should close after selection")
	}
	if v := m.Chat().ComposerValue(); !strings.Contains(v, "@alice") {
		t.Fatalf("composer should contain the inserted mention, got %q", v)
	}
}

func TestOutgoingMentionSentinelCarriesEntities(t *testing.T) {
	mc := &mockTGClient{}
	m, st := newRootOnGroupChat(t, mc)
	peer := store.Peer{ID: 5, Type: store.PeerSuperGroup}
	ents := []store.MessageEntity{{Type: "mention_name", Offset: 0, Length: 5, UserID: 7, AccessHash: 8}}
	nm, _ := m.Update(screens.SendMsgRequest{Peer: peer, Text: "@Ivan hi", Entities: ents})
	_ = nm.(ui.RootModel)

	msgs := st.Messages(5)
	if len(msgs) == 0 {
		t.Fatal("no message stored after send")
	}
	last := msgs[len(msgs)-1]
	if len(last.Entities) != 1 || last.Entities[0].Type != "mention_name" || last.Entities[0].UserID != 7 {
		t.Fatalf("optimistic outgoing bubble must carry the mention entity, got %+v", last.Entities)
	}
}

func TestMentionPopupNotOpenedInPrivateChat(t *testing.T) {
	mc := &mockTGClient{participants: []store.ChatMember{
		{UserID: 7, Username: "alice", DisplayName: "Alice A"},
	}}
	// newRootOnChat opens a 1:1 (PeerUser) chat.
	m, _ := newRootOnChat(t, mc)
	m = typeKey(t, m, 'i')
	m = typeKey(t, m, '@')
	m = typeKey(t, m, 'a')
	if m.MentionPopupOpen() {
		t.Fatal("mention popup must not open in a private chat")
	}
}
