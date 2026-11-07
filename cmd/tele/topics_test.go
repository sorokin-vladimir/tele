package main

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (f *fakeAccount) ResolveChat(_ context.Context, name core.ChatName) (project.ChatRow, error) {
	f.named = append(f.named, name)
	row, ok := f.byName[name]
	if !ok {
		return project.ChatRow{}, &telerr.Error{Kind: telerr.PeerNotFound}
	}
	return row, nil
}

func (f *fakeAccount) Topics(_ context.Context, chatID int64) ([]project.TopicRow, error) {
	return f.topics[chatID], nil
}

// A chat is named by its id, @username or me, and by nothing else: a title
// changes, and two chats can share one (#181).
func TestParseChatName(t *testing.T) {
	for arg, want := range map[string]core.ChatName{
		"42":      {ID: 42},
		"@gonews": {Username: "gonews"},
		"me":      {Me: true},
	} {
		got, err := parseChatName(arg)
		if err != nil || got != want {
			t.Errorf("%q named %+v, %v; want %+v", arg, got, err, want)
		}
	}
	for _, arg := range []string{"Go News", "@", "-5", "0", "gonews", ""} {
		_, err := parseChatName(arg)
		var code *exitCode
		if !errors.As(err, &code) || code.code != usageCode {
			t.Errorf("%q: got %v, want exit %d", arg, err, usageCode)
		}
	}
}

// A chat nobody holds exits 5 and says which name named nothing (#181).
func TestResolve_AChatNobodyHoldsExitsFive(t *testing.T) {
	acc := newFakeAccount()

	_, err := resolveChat(shortCtx(t), acc, "@nobody_here")

	var code *exitCode
	if !errors.As(err, &code) || code.code != notFoundCode {
		t.Fatalf("got %v, want exit %d", err, notFoundCode)
	}
	assertSays(t, code.msg, "@nobody_here")
}

// `tele topics CHAT` lists a forum's topics in the order a window shows them,
// a line each with its id, title and badges (#181).
func TestTopics_ListsAForumsTopics(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "gophers"}: {ID: 300, Title: "Gophers", IsForum: true}}
	acc.topics = map[int64][]project.TopicRow{300: {
		{ID: 1, Title: "General", Pinned: true},
		{ID: 17, Title: "Releases", Unread: 4, Mentions: 1, Muted: true},
		{ID: 23, Title: "Old", Closed: true, Reactions: 1},
	}}
	var out bytes.Buffer

	if err := listTopics(shortCtx(t), acc, &out, "@gophers", topicsQuery{}); err != nil {
		t.Fatal(err)
	}

	want := "" +
		"1   General   pinned\n" +
		"17  Releases  4 unread, 1 mention, muted\n" +
		"23  Old       1 reaction, closed\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// A topic's title is cut at the name width too.
func TestTopics_CutsALongTitle(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{ID: 300}: {ID: 300, Title: "Gophers", IsForum: true}}
	acc.topics = map[int64][]project.TopicRow{300: {{ID: 17, Title: "Releases and everything around them", Unread: 4}}}
	var out bytes.Buffer

	if err := listTopics(shortCtx(t), acc, &out, "300", topicsQuery{nameWidth: 8}); err != nil {
		t.Fatal(err)
	}

	if out.String() != "17  Release…  4 unread\n" {
		t.Errorf("printed %q", out.String())
	}
}

// --limit keeps the first topics of the list.
func TestTopics_LimitKeepsTheFirst(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{ID: 300}: {ID: 300, Title: "Gophers", IsForum: true}}
	acc.topics = map[int64][]project.TopicRow{300: {{ID: 1, Title: "General"}, {ID: 17, Title: "Releases"}, {ID: 23, Title: "Old"}}}
	var out bytes.Buffer

	if err := listTopics(shortCtx(t), acc, &out, "300", topicsQuery{limit: 2}); err != nil {
		t.Fatal(err)
	}

	if out.String() != "1   General\n17  Releases\n" {
		t.Errorf("printed %q", out.String())
	}
}

func TestTopics_PrintsJSONLines(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{ID: 300}: {ID: 300, Title: "Gophers", IsForum: true}}
	acc.topics = map[int64][]project.TopicRow{300: {{ID: 17, Title: "Releases", Unread: 4, Muted: true, Closed: true}}}
	var out bytes.Buffer

	if err := listTopics(shortCtx(t), acc, &out, "300", topicsQuery{asJSON: true}); err != nil {
		t.Fatal(err)
	}

	want := `{"id":17,"title":"Releases","unread":4,"mentions":0,"reactions":0,"muted":true,"pinned":false,"closed":true}` + "\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// A chat that is not a forum has no topics to list: it exits 5 rather than
// printing nothing (#181).
func TestTopics_AChatThatIsNotAForumExitsFive(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "bob"}: {ID: 5, Title: "Bob"}}
	var out bytes.Buffer

	err := listTopics(shortCtx(t), acc, &out, "@bob", topicsQuery{})

	var code *exitCode
	if !errors.As(err, &code) || code.code != notFoundCode {
		t.Fatalf("got %v, want exit %d", err, notFoundCode)
	}
	assertSays(t, code.msg, "Bob", "not a forum")
}
