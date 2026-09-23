package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// newRootOnGroupChat builds a main-screen RootModel focused on a supergroup, so
// the @mention popup (a group-only feature) is applicable.
func newRootOnGroupChat(t *testing.T, participants []domain.ChatMember) (ui.RootModel, store.Store) {
	t.Helper()
	st := store.NewMemory()
	chat := domain.Chat{ID: 5, Title: "Group", Peer: domain.Peer{ID: 5, Type: domain.PeerSuperGroup}}
	st.SetChat(chat)
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	// Mention candidates come from the owner's query (#198), so that is where the
	// test's list goes.
	ownerOf(t, m).participants = participants
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	nm, cmd := m.Update(screens.OpenChatMsg{ChatID: chat.ID, Title: chat.Title})
	m = nm.(ui.RootModel)
	// The subscription's opening Reset carries the header, which is what says
	// the chat is a group and so has members to mention (#278).
	return drainOwner(t, deliver(t, m, cmd)), st
}

// Until the header lands nothing says the chat is a group, and a person has
// no members: the popup stays shut rather than asking a user for them.
func TestMentionPopupStaysShutBeforeTheHeaderLands(t *testing.T) {
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 5, Title: "Group", Peer: domain.Peer{ID: 5, Type: domain.PeerSuperGroup}})
	m := newRoot(st, 50, false).WithScreen(ui.ScreenMain)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = nm.(ui.RootModel)
	nm, _ = m.Update(screens.OpenChatMsg{ChatID: 5, Title: "Group"})
	m = nm.(ui.RootModel)

	m = typeKey(t, m, 'i')
	m = typeKey(t, m, '@')

	if m.MentionPopupOpen() {
		t.Fatal("mention popup must stay shut until the header says the chat is a group")
	}
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
	m, _ := newRootOnGroupChat(t, []domain.ChatMember{
		{UserID: 7, Username: "alice", DisplayName: "Alice A"},
		{UserID: 8, Username: "bob", DisplayName: "Bob B"},
	})

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

// The mention entity has to survive the trip to the queue: it names a user by
// ID and access hash, which cannot be recovered from the text afterwards.
func TestOutgoingMentionSubmissionCarriesEntities(t *testing.T) {
	m, _ := newRootOnGroupChat(t, nil)
	owner := m.Owner().(*testOwner)
	ents := []domain.MessageEntity{{Type: "mention_name", Offset: 0, Length: 5, UserID: 7, AccessHash: 8}}

	_, cmd := m.Update(screens.SendMsgRequest{Text: "@Ivan hi", Entities: ents})
	if cmd == nil {
		t.Fatal("send produced no command")
	}
	cmd()

	if len(owner.sent) != 1 {
		t.Fatalf("expected one submission, got %d", len(owner.sent))
	}
	got := owner.sent[0].Entities
	if len(got) != 1 || got[0].Type != "mention_name" || got[0].UserID != 7 {
		t.Fatalf("the submission must carry the mention entity, got %+v", got)
	}
}

func TestMentionPopupNotOpenedInPrivateChat(t *testing.T) {
	// newRootOnChat opens a 1:1 (PeerUser) chat.
	m, _ := newRootOnChat(t)
	m = typeKey(t, m, 'i')
	m = typeKey(t, m, '@')
	m = typeKey(t, m, 'a')
	if m.MentionPopupOpen() {
		t.Fatal("mention popup must not open in a private chat")
	}
}
