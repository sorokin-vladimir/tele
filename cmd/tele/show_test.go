package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/config"
	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

var me = project.LoggedInUser{ID: 77, Username: "ada", Name: "Ada Lovelace"}

func (f *fakeAccount) History(_ context.Context, h domain.HistoryKey, before, limit int) ([]domain.Message, error) {
	f.historyAsked = append(f.historyAsked, historyAsk{h, before, limit})
	return f.history, nil
}

func (f *fakeAccount) MarkRead(_ context.Context, h domain.HistoryKey, maxID int) error {
	f.markedRead = append(f.markedRead, historyAsk{h: h, before: maxID})
	return f.markErr
}

// historyAsk is a history asked for, and from where.
type historyAsk struct {
	h             domain.HistoryKey
	before, limit int
}

func held(acc *fakeAccount) heldAccount {
	return heldAccount{accountSurface: acc, me: me, cfg: &config.Config{CLI: config.CLIConfig{NameWidth: 20}}}
}

// cli.name_width is read by each command from its own config as it starts:
// the next `tele show` cuts a sender's name, and the next `tele chats` a
// chat's title, at the width the file names, and at 20 when it names none
// (#181).
func TestTakesHold_cli_name_width(t *testing.T) {
	_, cfgPath := isolatedHome(t)
	const long = "Alexander Konstantinovich Long"
	shown := func() (show, chats string) {
		t.Helper()
		s, err := newSetup(cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		acc := newFakeAccount()
		acc.byName = map[core.ChatName]project.ChatRow{{ID: 42}: {ID: 42, Title: "Group"}}
		acc.history = []domain.Message{{ID: 1, Date: at(25, 9, 0), SenderName: long, Text: "hi"}}
		acc.chats = []project.ChatRow{{ID: 42, Title: long, Unread: 1}}
		a := heldAccount{accountSurface: acc, me: me, cfg: s.store.Current()}
		var showOut, chatsOut bytes.Buffer
		if err := showMessages(shortCtx(t), a, &showOut, "42", showQuery{limit: 1}); err != nil {
			t.Fatal(err)
		}
		if err := listChats(shortCtx(t), a, &chatsOut, chatsQuery{nameWidth: a.cfg.CLI.NameWidth}); err != nil {
			t.Fatal(err)
		}
		return showOut.String(), chatsOut.String()
	}

	show, chats := shown()
	assertSays(t, show, "Alexander Konstanti…:")
	assertSays(t, chats, "Alexander Konstanti…  1 unread")

	body := "telegram:\n  api_id: 1\n  api_hash: x\ncli:\n  name_width: 6\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	show, chats = shown()
	assertSays(t, show, "Alexa…:")
	assertSays(t, chats, "Alexa…  1 unread")
}

// show asks for the history it was named, from before and at most limit, and
// marks nothing read unless asked (#181).
func TestShow_AsksForTheHistoryNamed(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "bob"}: {ID: 42, Title: "Bob"}}
	acc.history = []domain.Message{{ID: 9, Date: at(25, 9, 0), SenderName: "Bob", Text: "hi"}}
	var out bytes.Buffer

	err := showMessages(shortCtx(t), held(acc), &out, "@bob", showQuery{limit: 5, before: 10})

	if err != nil {
		t.Fatal(err)
	}
	want := []historyAsk{{h: domain.HistoryKey{ChatID: 42}, before: 10, limit: 5}}
	if !reflect.DeepEqual(acc.historyAsked, want) {
		t.Errorf("asked %+v, want %+v", acc.historyAsked, want)
	}
	if len(acc.markedRead) != 0 {
		t.Errorf("marked read unasked: %+v", acc.markedRead)
	}
	assertSays(t, out.String(), "Bob:", "hi")
}

// --mark-read marks the chat read up to the last message shown and no
// further, so nothing the script never saw is marked read.
func TestShow_MarkReadStopsAtTheLastMessageShown(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{ID: 42}: {ID: 42, Title: "Bob"}}
	acc.history = []domain.Message{{ID: 7, Date: at(25, 9, 0)}, {ID: 9, Date: at(25, 9, 1)}}
	var out bytes.Buffer

	if err := showMessages(shortCtx(t), held(acc), &out, "42", showQuery{limit: 2, markRead: true}); err != nil {
		t.Fatal(err)
	}

	want := []historyAsk{{h: domain.HistoryKey{ChatID: 42}, before: 9}}
	if !reflect.DeepEqual(acc.markedRead, want) {
		t.Errorf("marked %+v, want %+v", acc.markedRead, want)
	}
}

// Marking read that fails leaves nothing printed: a script reads either the
// whole answer or nothing (docs/cli.md).
func TestShow_AFailedMarkReadPrintsNothing(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{ID: 42}: {ID: 42, Title: "Bob"}}
	acc.history = []domain.Message{{ID: 9, Date: at(25, 9, 0), Text: "hi"}}
	acc.markErr = errors.New("refused")
	var out bytes.Buffer

	err := showMessages(shortCtx(t), held(acc), &out, "42", showQuery{limit: 2, markRead: true})

	if err == nil || out.Len() != 0 {
		t.Errorf("got %v and printed %q", err, out.String())
	}
}

// A forum has no history of its own: show without --topic exits 5 and points
// at the topics. A topic of a chat that is no forum exits 5 too (#181).
func TestShow_AForumNeedsATopicAndOnlyAForumHasOne(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{
		{Username: "gophers"}: {ID: 300, Title: "Gophers", IsForum: true},
		{Username: "bob"}:     {ID: 42, Title: "Bob"},
	}
	for _, c := range []struct {
		chat  string
		q     showQuery
		wants []string
	}{
		{"@gophers", showQuery{limit: 5}, []string{"forum", "tele topics @gophers"}},
		{"@bob", showQuery{limit: 5, topic: 3}, []string{"Bob", "not a forum"}},
	} {
		err := showMessages(shortCtx(t), held(acc), &bytes.Buffer{}, c.chat, c.q)

		var code *exitCode
		if !errors.As(err, &code) || code.code != notFoundCode {
			t.Fatalf("%s: got %v, want exit %d", c.chat, err, notFoundCode)
		}
		assertSays(t, code.msg, c.wants...)
	}
	if len(acc.historyAsked) != 0 {
		t.Errorf("asked for a history anyway: %+v", acc.historyAsked)
	}
}

// A topic of a forum is asked for by its id.
func TestShow_ATopicOfAForum(t *testing.T) {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "gophers"}: {ID: 300, Title: "Gophers", IsForum: true}}

	if err := showMessages(shortCtx(t), held(acc), &bytes.Buffer{}, "@gophers", showQuery{limit: 5, topic: 17}); err != nil {
		t.Fatal(err)
	}

	if len(acc.historyAsked) != 1 || acc.historyAsked[0].h != (domain.HistoryKey{ChatID: 300, TopicID: 17}) {
		t.Errorf("asked %+v", acc.historyAsked)
	}
}

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

// Every message's text starts in the same column, after the widest name
// shown; a date line comes first and wherever the day changes, and a reply
// says what it answers under it. Your own messages are yours (#181).
func TestShowText_AlignsTheTextAfterTheNames(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, Date: at(24, 21, 58), SenderID: 42, SenderName: "Bob", Text: "see you tomorrow"},
		{ID: 2, Date: at(25, 9, 12), IsOut: true, Text: "morning!", ReplyToMsgID: 1},
		{ID: 3, Date: at(25, 9, 15), SenderID: 43, SenderName: "Alexandra K.", Text: "first line\nsecond line"},
		{ID: 4, Date: at(25, 9, 16), SenderID: 42, SenderName: "Bob", Text: "an answer", ReplyToMsgID: 900},
	}

	got := showText(msgs, me, 20, time.UTC)

	want := "" +
		"2026-10-24\n" +
		"21:58 Bob:           see you tomorrow\n" +
		"2026-10-25\n" +
		"09:12 You:           morning!\n" +
		"  ↳ Bob: see you tomorrow\n" +
		"09:15 Alexandra K.:  first line\n" +
		"                     second line\n" +
		"09:16 Bob:           an answer\n" +
		"  ↳ reply to 900\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

// A name wider than the column is cut short, so one long name does not push
// everybody's text to the right.
func TestShowText_CutsANameWiderThanTheColumn(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, Date: at(25, 9, 0), SenderName: "Alexander Konstantinovich", Text: "hi"},
		{ID: 2, Date: at(25, 9, 1), SenderName: "Bob", Text: "hello"},
	}

	got := showText(msgs, me, 10, time.UTC)

	want := "" +
		"2026-10-25\n" +
		"09:00 Alexander…:  hi\n" +
		"09:01 Bob:         hello\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

// Media is named, never fetched: a label for its kind and what it says about
// itself, then the caption (#181).
func TestShowText_NamesTheMedia(t *testing.T) {
	cases := []struct {
		media *domain.MediaRef
		text  string
		want  string
	}{
		{&domain.MediaRef{Kind: domain.MediaPhoto}, "the view", "[photo] the view"},
		{&domain.MediaRef{Kind: domain.MediaVideo, Duration: 31}, "", "[video 0:31]"},
		{&domain.MediaRef{Kind: domain.MediaVideoNote, Duration: 9}, "", "[round video 0:09]"},
		{&domain.MediaRef{Kind: domain.MediaVoice, Duration: 72}, "", "[voice 1:12]"},
		{&domain.MediaRef{Kind: domain.MediaAudio, Duration: 185, Performer: "Nina", Title: "Song"}, "", "[audio Nina – Song 3:05]"},
		{&domain.MediaRef{Kind: domain.MediaAudio, Duration: 3725, FileName: "talk.mp3"}, "", "[audio talk.mp3 1:02:05]"},
		{&domain.MediaRef{Kind: domain.MediaSticker, Emoji: "👍"}, "", "[sticker 👍]"},
		{&domain.MediaRef{Kind: domain.MediaGIF}, "", "[GIF]"},
		{&domain.MediaRef{Kind: domain.MediaFile, FileName: "report.pdf"}, "", "[file report.pdf]"},
		{&domain.MediaRef{Kind: domain.MediaLocation}, "", "[location]"},
		{&domain.MediaRef{Kind: domain.MediaOther}, "", "[media]"},
	}
	for _, c := range cases {
		msgs := []domain.Message{{ID: 1, Date: at(25, 9, 0), SenderName: "Bob", Media: c.media, Text: c.text}}

		got := showText(msgs, me, 20, time.UTC)

		want := "2026-10-25\n09:00 Bob:  " + c.want + "\n"
		if got != want {
			t.Errorf("printed %q, want %q", got, want)
		}
	}
}

// As JSON a message is an object a line, oldest first. The sender is named by
// id and by name on every message, your own included; what a message does not
// have is left out, and a file is described, never pointed at (#181).
func TestShowJSON(t *testing.T) {
	msgs := []domain.Message{
		{ID: 512, Date: at(25, 9, 12), IsOut: true, Text: "morning!", ReplyToMsgID: 511},
		{ID: 513, Date: at(25, 9, 13), SenderID: 42, SenderName: "Bob", Media: &domain.MediaRef{Kind: domain.MediaVoice, Duration: 12}},
		{ID: 514, Date: at(25, 9, 14), SenderID: 42, SenderName: "Bob", Text: "here", Media: &domain.MediaRef{Kind: domain.MediaFile, FileName: "report.pdf", Size: 2048}},
	}
	var out bytes.Buffer

	if err := showJSON(&out, msgs, me); err != nil {
		t.Fatal(err)
	}

	want := "" +
		`{"id":512,"date":"2026-10-25T09:12:00Z","sender_id":77,"sender":"Ada Lovelace","out":true,"text":"morning!","reply_to":511}` + "\n" +
		`{"id":513,"date":"2026-10-25T09:13:00Z","sender_id":42,"sender":"Bob","out":false,"media":{"kind":"voice","duration":12}}` + "\n" +
		`{"id":514,"date":"2026-10-25T09:14:00Z","sender_id":42,"sender":"Bob","out":false,"text":"here","media":{"kind":"file","file_name":"report.pdf","size":2048}}` + "\n"
	if out.String() != want {
		t.Errorf("printed\n%s\nwant\n%s", out.String(), want)
	}
}
