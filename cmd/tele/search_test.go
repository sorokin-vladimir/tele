package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// searchAsk is a message search asked for.
type searchAsk struct {
	scope project.SearchScope
	text  string
	limit int
}

func (f *fakeAccount) SearchMessages(_ context.Context, scope project.SearchScope, text string, limit int) ([]project.FoundMessage, error) {
	f.searched = append(f.searched, searchAsk{scope, text, limit})
	return f.found, nil
}

// With no chat named, a search looks in every chat (#348).
func TestSearch_WithNoChatLooksEverywhere(t *testing.T) {
	acc := newFakeAccount()

	if err := searchMessages(shortCtx(t), held(acc), &bytes.Buffer{}, "hello", searchQuery{limit: 30}); err != nil {
		t.Fatal(err)
	}

	want := []searchAsk{{scope: project.SearchScope{}, text: "hello", limit: 30}}
	if !reflect.DeepEqual(acc.searched, want) {
		t.Errorf("asked %+v, want %+v", acc.searched, want)
	}
}

// A chat named is looked up and searched, and so is a topic of a forum.
func TestSearch_InAChatOrATopic(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{
		{Username: "bob"}:     {ID: 42, Title: "Bob"},
		{Username: "gophers"}: {ID: 300, Title: "Gophers", IsForum: true},
	}

	for _, q := range []searchQuery{{chat: "@bob", limit: 5}, {chat: "@gophers", topic: 17, limit: 5}, {chat: "@gophers", limit: 5}} {
		if err := searchMessages(shortCtx(t), held(acc), &bytes.Buffer{}, "hello", q); err != nil {
			t.Fatal(err)
		}
	}

	want := []project.SearchScope{{ChatID: 42}, {ChatID: 300, TopicID: 17}, {ChatID: 300}}
	var got []project.SearchScope
	for _, s := range acc.searched {
		got = append(got, s.scope)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("searched %+v, want %+v", got, want)
	}
}

// A topic of a chat that is no forum exits 5, and a chat nobody holds too.
func TestSearch_NoSuchTopicOrChatExitsFive(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "bob"}: {ID: 42, Title: "Bob"}}

	for _, q := range []searchQuery{{chat: "@bob", topic: 3, limit: 5}, {chat: "@nobody", limit: 5}} {
		err := searchMessages(shortCtx(t), held(acc), &bytes.Buffer{}, "hello", q)

		var code *exitCode
		if !errors.As(err, &code) || code.code != notFoundCode {
			t.Errorf("%+v: got %v, want exit %d", q, err, notFoundCode)
		}
	}
	if len(acc.searched) != 0 {
		t.Errorf("searched anyway: %+v", acc.searched)
	}
}

// Nothing found prints nothing and is no failure.
func TestSearch_NothingFoundPrintsNothing(t *testing.T) {
	acc := newFakeAccount()
	var out bytes.Buffer

	err := searchMessages(shortCtx(t), held(acc), &out, "hello", searchQuery{limit: 5, asJSON: true})

	if err != nil || out.Len() != 0 {
		t.Errorf("got %v and printed %q", err, out.String())
	}
}

func foundSample() []project.FoundMessage {
	return []project.FoundMessage{
		{Chat: "Gophers", Message: domain.Message{ID: 900, ChatID: 300, TopicID: 17, Date: at(25, 9, 14), SenderID: 42, SenderName: "Bob", Text: "hello there\nand more"}},
		{Chat: "Bob", Message: domain.Message{ID: 512, ChatID: 42, Date: at(24, 21, 58), IsOut: true, Text: "hello"}},
	}
}

// Each message found is a JSON object a line, in the shape tele show prints,
// with the chat it was found in first and its topic in a forum, whatever was
// searched (#348).
func TestSearchJSON(t *testing.T) {
	var out bytes.Buffer

	if err := searchJSON(&out, foundSample(), me); err != nil {
		t.Fatal(err)
	}

	want := "" +
		`{"chat_id":300,"chat":"Gophers","topic_id":17,"id":900,"date":"2026-10-25T09:14:00Z","sender_id":42,"sender":"Bob","out":false,"text":"hello there\nand more"}` + "\n" +
		`{"chat_id":42,"chat":"Bob","id":512,"date":"2026-10-24T21:58:00Z","sender_id":77,"sender":"Ada Lovelace","out":true,"text":"hello"}` + "\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}

// As a person reads it, a message found is a line: its id first, as that is
// what tele show --around takes, then when, where and who, and how it starts.
func TestSearchText(t *testing.T) {
	got := searchText(foundSample(), 20, time.UTC)

	want := "" +
		"900  2026-10-25 09:14  Gophers › topic 17  Bob:  hello there …\n" +
		"512  2026-10-24 21:58  Bob                 You:  hello\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}
