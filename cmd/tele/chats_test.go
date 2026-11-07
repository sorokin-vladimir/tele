package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// fakeAccount is the owner's surface as a command sees it. Only what a test
// sets up answers: any other call panics on the nil Surface (#181).
type fakeAccount struct {
	core.Surface
	stream chan any
	// subs are the windows subscribed, in order; the first gets SubID 1.
	subs []project.Window
	// chats is what a chat list subscription is answered with, whatever its
	// folder: which chats a folder holds is the owner's business.
	chats   []project.ChatRow
	folders []domain.FolderFilter
	// byName is what each chat name resolves to; named is every name asked.
	byName map[core.ChatName]project.ChatRow
	named  []core.ChatName
	topics map[int64][]project.TopicRow
	// history is what History answers with; historyAsked and markedRead are
	// what was asked of it.
	history      []domain.Message
	historyAsked []historyAsk
	markedRead   []historyAsk
	markErr      error
	// sent is every send queued; outcomes are what AwaitSend answers, in
	// turn, and awaited how long it was asked to wait each time.
	sent     []core.SendRequest
	outcomes []outcome
	awaited  []time.Duration
}

func newFakeAccount() *fakeAccount {
	return &fakeAccount{stream: make(chan any, 16)}
}

func (f *fakeAccount) Stream() <-chan any { return f.stream }

func (f *fakeAccount) Subscribe(w project.Window) project.SubID {
	f.subs = append(f.subs, w)
	id := project.SubID(len(f.subs))
	if _, ok := w.(project.ChatListWindow); ok {
		f.stream <- project.Delta{Sub: id, ChatList: &project.ChatListDelta{
			Kind: project.ChatListReset, Total: len(f.chats), Rows: f.chats,
			Folders: project.FolderCounts{Filters: f.folders},
		}}
	}
	return id
}

func (f *fakeAccount) Unsubscribe(project.SubID) {}

// folderAsked is the folder the command's chat list subscription asked for.
func (f *fakeAccount) folderAsked(t *testing.T) int {
	t.Helper()
	for _, w := range f.subs {
		if cw, ok := w.(project.ChatListWindow); ok {
			return cw.Folder
		}
	}
	t.Fatal("no chat list was subscribed to")
	return 0
}

// say puts the account in phase on the stream, addressed to subscription sub.
func (f *fakeAccount) say(sub project.SubID, phase project.AccountPhase) {
	f.stream <- project.Delta{Sub: sub, Account: &project.AccountDelta{Contents: project.AccountContents{Phase: phase}}}
}

func shortCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)
	return ctx
}

// An account waiting on a login is not logged in: the command does not wait
// for a person to log in, it exits 4 and says where to do it (#181).
func TestAwaitReady_AnAccountWaitingOnALoginExitsFour(t *testing.T) {
	acc := newFakeAccount()
	acc.say(1, project.AccountConnecting)
	acc.say(1, project.AccountLoggingIn)

	_, err := awaitReady(shortCtx(t), acc)

	var code *exitCode
	if !errors.As(err, &code) || code.code != notLoggedInCode {
		t.Fatalf("got %v, want exit %d", err, notLoggedInCode)
	}
	assertSays(t, code.msg, "not logged in", "tele")
}

// Between one owner and the next there is no account to be ready. A
// subscription ends with its owner, so the command subscribes again and waits
// for the next one (#181).
func TestAwaitReady_WaitsThroughAnAccountEnding(t *testing.T) {
	acc := newFakeAccount()
	acc.say(1, project.AccountLoading)
	acc.stream <- core.AccountEnded{Reason: core.EndLoggedOutElsewhere}
	acc.say(2, project.AccountConnecting)
	acc.say(2, project.AccountReady)

	if _, err := awaitReady(shortCtx(t), acc); err != nil {
		t.Fatal(err)
	}
	if len(acc.subs) != 2 {
		t.Errorf("subscribed %d times, want once per owner", len(acc.subs))
	}
}

// A host that goes away leaves nobody to ask: the command exits 3, as when
// there was none to begin with (#181).
func TestAwaitReady_AHostGoneExitsThree(t *testing.T) {
	acc := newFakeAccount()
	acc.say(1, project.AccountConnecting)
	close(acc.stream)

	_, err := awaitReady(shortCtx(t), acc)

	var code *exitCode
	if !errors.As(err, &code) || code.code != noHostCode {
		t.Fatalf("got %v, want exit %d", err, noHostCode)
	}
}

// A tele that does not let commands reach the account says so on the stream,
// where the command is waiting; the command exits 7 and names the setting
// (#181).
func TestAwaitReady_CommandsTurnedOffExitSeven(t *testing.T) {
	acc := newFakeAccount()
	acc.stream <- core.HostFailure{Err: &telerr.Error{Kind: telerr.CommandsOff}}

	_, err := awaitReady(shortCtx(t), acc)

	var code *exitCode
	if !errors.As(err, &code) || code.code != commandsOffCode {
		t.Fatalf("got %v, want exit %d", err, commandsOffCode)
	}
	assertSays(t, code.msg, "cli.account_commands")
}

// A call refused the same way exits 7 too, whichever call it was.
func TestCommandsOff_ARefusedCallExitsSeven(t *testing.T) {
	err := commandsOff(fmt.Errorf("resolving: %w", &telerr.Error{Kind: telerr.CommandsOff}))

	var code *exitCode
	if !errors.As(err, &code) || code.code != commandsOffCode {
		t.Fatalf("got %v, want exit %d", err, commandsOffCode)
	}
	other := errors.New("something else")
	if commandsOff(other) != other {
		t.Error("another error was taken for a refusal")
	}
}

// A connection that ended with anything but a log out will not come back by
// itself: the command says what it ended with instead of waiting it out. A log
// out is waited through, since the next account follows it (#181).
func TestAwaitReady_AConnectionThatEndedIsSaid(t *testing.T) {
	acc := newFakeAccount()
	acc.stream <- project.Delta{Sub: 1, Account: &project.AccountDelta{Contents: project.AccountContents{
		Phase: project.AccountConnecting,
		Ended: &telerr.Error{Kind: telerr.AppKeyBlocked},
	}}}

	_, err := awaitReady(shortCtx(t), acc)

	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want what the connection ended with", err)
	}
	assertSays(t, err.Error(), "ended", string(telerr.AppKeyBlocked))
}

func TestAwaitReady_ALogOutIsWaitedThrough(t *testing.T) {
	acc := newFakeAccount()
	acc.stream <- project.Delta{Sub: 1, Account: &project.AccountDelta{Contents: project.AccountContents{
		Phase: project.AccountReady,
		Ended: &telerr.Error{Kind: telerr.Unauthorized, LogOut: telerr.LogOutElsewhere},
	}}}
	acc.stream <- core.AccountEnded{Reason: core.EndLoggedOutElsewhere}
	acc.say(2, project.AccountReady)

	if _, err := awaitReady(shortCtx(t), acc); err != nil {
		t.Fatal(err)
	}
	if len(acc.subs) != 2 {
		t.Errorf("answered from the account that was logged out of")
	}
}

// `tele chats` prints the main list in its order, a line per chat with its id,
// title and badges. The main list is the one a window shows first, without
// the archived chats (#181).
func TestChats_ListsTheMainListWithBadges(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 42, Title: "Alice", IsUser: true, Unread: 3, Mentions: 1},
		{ID: 7, Title: "Work", Muted: true, Reactions: 2},
		{ID: 300, Title: "Gophers", IsForum: true, Unread: 1, UnreadMark: true},
		{ID: 5, Title: "Bob"},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{}); err != nil {
		t.Fatal(err)
	}

	if got := acc.folderAsked(t); got != 0 {
		t.Errorf("asked for folder %d, want the main list", got)
	}
	want := "" +
		"42   Alice    3 unread, 1 mention\n" +
		"7    Work     2 reactions, muted\n" +
		"300  Gophers  forum, 1 topic unread, marked unread\n" +
		"5    Bob\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// A title wider than the name width is cut short, as a sender's name is in
// `tele show`, so one long title does not push everybody's badges to the
// right. JSON keeps it whole (#181).
func TestChats_CutsALongTitle(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 1, Title: "The Very Long Name of a Group", Unread: 2},
		{ID: 2, Title: "Bob", Muted: true},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{nameWidth: 10}); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"1  The Very…  2 unread\n" +
		"2  Bob        muted\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// A chat's @username has its own column between the id and the title, so the
// name a command takes is on screen. It is never cut, since a cut one cannot
// be pasted, and a chat with none leaves the column blank (#328).
func TestChats_ShowsTheUsername(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 42, Title: "Bob", Username: "bob", Unread: 3},
		{ID: 100500, Title: "Julia"},
		{ID: 7, Title: "Gophers", Username: "golang_gophers_chat", Muted: true},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{nameWidth: 10}); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"42      @bob                  Bob      3 unread\n" +
		"100500                        Julia\n" +
		"7       @golang_gophers_chat  Gophers  muted\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// --limit keeps the first chats of what the other flags leave: the first two
// with unread, here, and not the first two of the list (#181).
func TestChats_LimitKeepsTheFirstOfWhatIsLeft(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 1, Title: "Read"},
		{ID: 2, Title: "One", Unread: 1},
		{ID: 3, Title: "Two", Unread: 2},
		{ID: 4, Title: "Three", Unread: 3},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{unread: true, limit: 2}); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"2  One  1 unread\n" +
		"3  Two  2 unread\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// As JSON a chat is an object a line, in the list's order. Every field is there
// whatever its value, except a username the chat does not have (#181).
func TestChats_PrintsJSONLines(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 42, Title: "Alice", Username: "alice", Kind: project.ChatKindUser, IsUser: true, Unread: 3, Mentions: 1},
		{ID: 300, Title: "Gophers", Kind: project.ChatKindGroup, IsForum: true, Unread: 1, Reactions: 2, UnreadMark: true, Muted: true},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{asJSON: true}); err != nil {
		t.Fatal(err)
	}

	want := "" +
		`{"id":42,"title":"Alice","username":"alice","kind":"user","unread":3,"mentions":1,"reactions":0,"unread_mark":false,"muted":false,"archived":false,"forum":false}` + "\n" +
		`{"id":300,"title":"Gophers","kind":"group","unread":1,"mentions":0,"reactions":2,"unread_mark":true,"muted":true,"archived":false,"forum":true}` + "\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// --archived lists the Archive, and --folder one of the account's folders,
// each as a window shows it (#181).
func TestChats_ListsTheFolderAskedFor(t *testing.T) {
	for _, c := range []struct {
		q      chatsQuery
		folder int
	}{
		{chatsQuery{folder: domain.ArchiveFolderID}, domain.ArchiveFolderID},
		{chatsQuery{folder: 3}, 3},
	} {
		acc := newFakeAccount()
		acc.folders = []domain.FolderFilter{{ID: 3, Title: "Work"}}
		acc.chats = []project.ChatRow{{ID: 7, Title: "Team"}}
		var out bytes.Buffer

		if err := listChats(shortCtx(t), acc, &out, c.q); err != nil {
			t.Fatal(err)
		}

		if got := acc.folderAsked(t); got != c.folder {
			t.Errorf("asked for folder %d, want %d", got, c.folder)
		}
		if out.String() != "7  Team\n" {
			t.Errorf("printed %q", out.String())
		}
	}
}

// A folder the account does not have is said to be missing, with the ones it
// has, rather than listed as empty (#181).
func TestChats_AFolderThatIsNotThereIsSaid(t *testing.T) {
	acc := newFakeAccount()
	acc.folders = []domain.FolderFilter{{ID: 0, Title: "All Chats"}, {ID: 3, Title: "Work"}, {ID: 4, Title: "Family"}}
	var out bytes.Buffer

	err := listChats(shortCtx(t), acc, &out, chatsQuery{folder: 9})

	var code *exitCode
	if !errors.As(err, &code) || code.code != notFoundCode {
		t.Fatalf("got %v, want exit %d", err, notFoundCode)
	}
	assertSays(t, err.Error(), "9", "3 Work", "4 Family")
	if out.Len() != 0 {
		t.Errorf("printed %q", out.String())
	}
}

// --unread keeps the chats with something unread, a chat marked unread among
// them; a forum counts when any of its topics has unread (#181).
func TestChats_UnreadKeepsTheChatsWithUnread(t *testing.T) {
	acc := newFakeAccount()
	acc.chats = []project.ChatRow{
		{ID: 1, Title: "Read"},
		{ID: 2, Title: "Unread", Unread: 2},
		{ID: 3, Title: "Marked", UnreadMark: true},
		{ID: 4, Title: "Forum", IsForum: true, Unread: 1},
		{ID: 5, Title: "Quiet forum", IsForum: true},
	}
	var out bytes.Buffer

	if err := listChats(shortCtx(t), acc, &out, chatsQuery{unread: true, asJSON: true}); err != nil {
		t.Fatal(err)
	}

	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		ids = append(ids, line[:strings.Index(line, ",")])
	}
	want := []string{`{"id":2`, `{"id":3`, `{"id":4`}
	if strings.Join(ids, " ") != strings.Join(want, " ") {
		t.Errorf("listed %v, want %v", ids, want)
	}
}
