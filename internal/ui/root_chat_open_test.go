package ui

import (
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// The open chat is read from its projection, not from the store: the selected
// message is always one the window holds, and the chat's own state arrives on
// the window's Reset (#278).

func countCalls(o *ownerStub, name string) int {
	n := 0
	for _, c := range o.calls {
		if c.name == name {
			n++
		}
	}
	return n
}

// openTestChat opens chat 1 the way a user does and delivers contents as the
// subscription's opening Reset, running every command both steps return.
func openTestChat(t *testing.T, contents project.ChatContents, held ...domain.Chat) (*ownerStub, RootModel) {
	t.Helper()
	st := store.NewMemory()
	for _, c := range held {
		st.SetChat(c)
	}
	o := newOwnerStub(st)
	m := newRootInternal(st, 50).WithOwner(o).WithScreen(ScreenMain)
	nm, cmd := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Group"})
	m = nm.(RootModel)
	runCmdTree(cmd)
	contents.ChatID = 1
	m, cmd = m.handleChatDelta(&project.ChatDelta{Kind: project.ChatReset, Contents: contents})
	runCmdTree(cmd)
	return o, m
}

// The store is empty here: the message exists only in the window, which is
// where the client finds it now.
func TestActivateEdit_FindsTheMessageInTheWindow(t *testing.T) {
	_, m := openTestChat(t, project.ChatContents{
		IsGroup:  true,
		Messages: []domain.Message{{ID: 5, ChatID: 1, IsOut: true, Text: "typo", Date: time.Unix(1, 0)}},
	})

	m.activateEdit(5)

	if m.chat.EditMsgID() != 5 {
		t.Fatalf("edit target = %d, want 5", m.chat.EditMsgID())
	}
	if got := m.chat.ComposerValue(); got != "typo" {
		t.Fatalf("composer = %q, want the message text", got)
	}
}

// Mentions are read once, when the chat is opened. A later Reset - a window
// that moved, a retry after a failed load - is not an opening.
func TestChatReset_OnlyTheFirstAfterOpeningReadsMentions(t *testing.T) {
	o, m := openTestChat(t, project.ChatContents{IsGroup: true, UnreadMentions: 2})
	if n := countCalls(o, "ReadMentions"); n != 1 {
		t.Fatalf("ReadMentions after opening = %d, want 1", n)
	}

	_, cmd := m.handleChatDelta(&project.ChatDelta{
		Kind:     project.ChatReset,
		Contents: project.ChatContents{ChatID: 1, IsGroup: true, UnreadMentions: 2},
	})
	runCmdTree(cmd)

	if n := countCalls(o, "ReadMentions"); n != 1 {
		t.Fatalf("ReadMentions after a second Reset = %d, want still 1", n)
	}
}

func TestChatReset_NoMentionsNoRequest(t *testing.T) {
	o, _ := openTestChat(t, project.ChatContents{IsGroup: true})
	if n := countCalls(o, "ReadMentions"); n != 0 {
		t.Fatalf("ReadMentions = %d, want none for a chat with nothing to read", n)
	}
}

// Opening a chat with unread reactions reads them once. The opening Reset is
// what does it; the open path used to send a second request of its own.
func TestOpenChat_ReadsReactionsOnce(t *testing.T) {
	o, _ := openTestChat(t, project.ChatContents{UnreadReactions: 3},
		domain.Chat{ID: 1, Title: "Ada", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, UnreadReactionsCount: 3})
	if n := countCalls(o, "ReadReactions"); n != 1 {
		t.Fatalf("ReadReactions = %d, want exactly 1", n)
	}
}

// The draft the composer is compared against is the one the projection
// carries, so leaving a chat whose draft is unchanged sends nothing.
func TestFlushDraft_ComparesAgainstTheProjectionsDraft(t *testing.T) {
	_, m := openTestChat(t, project.ChatContents{Draft: "wip"})

	m.chat.SetComposerValue("wip")
	if cmd := m.flushCurrentDraftCmd(); cmd != nil {
		t.Fatal("an unchanged draft must not be saved again")
	}

	m.chat.SetComposerValue("wip, more")
	if cmd := m.flushCurrentDraftCmd(); cmd == nil {
		t.Fatal("a changed draft must be saved")
	}
}

// A draft synced from another device moves what counts as unchanged.
func TestFlushDraft_FollowsADraftSyncedFromElsewhere(t *testing.T) {
	_, m := openTestChat(t, project.ChatContents{Draft: "wip"})
	m, _ = m.handleChatDelta(&project.ChatDelta{Kind: project.ChatDraft, Draft: "from phone"})

	m.chat.SetComposerValue("from phone")
	if cmd := m.flushCurrentDraftCmd(); cmd != nil {
		t.Fatal("the synced draft is the server's; saving it back is a no-op round trip")
	}
}
