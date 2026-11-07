package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (f *fakeAccount) Send(_ context.Context, req core.SendRequest) error {
	f.sent = append(f.sent, req)
	return nil
}

func (f *fakeAccount) AwaitSend(_ context.Context, ref string, within time.Duration) (core.SendOutcome, error) {
	f.awaited = append(f.awaited, within)
	if len(f.outcomes) == 0 {
		return core.SendOutcome{}, &telerr.Error{Kind: telerr.NotFound}
	}
	next := f.outcomes[0]
	f.outcomes = f.outcomes[1:]
	return next.outcome, next.err
}

// outcome is one answer AwaitSend gives.
type outcome struct {
	outcome core.SendOutcome
	err     error
}

func sentAs(msgID int) outcome {
	return outcome{outcome: core.SendOutcome{Sent: &domain.SentSend{
		Ref: "r", ChatID: 42, MsgIDs: []int{msgID}, At: at(25, 9, 30),
	}}}
}

func sendingTo(row project.ChatRow, outcomes ...outcome) *fakeAccount {
	acc := newFakeAccount()
	acc.byName = map[core.ChatName]project.ChatRow{{Username: "bob"}: row}
	acc.outcomes = outcomes
	return acc
}

var bob = project.ChatRow{ID: 42, Title: "Bob", Kind: project.ChatKindUser}

func exitOf(t *testing.T, err error) int {
	t.Helper()
	var code *exitCode
	if !errors.As(err, &code) {
		t.Fatalf("got %v, want an exit code", err)
	}
	return code.code
}

// The text is the words given, joined by spaces; with none it is what comes
// on stdin. Nothing to send is a mistake in the command line (#181).
func TestComposeText(t *testing.T) {
	got, err := composeText([]string{"see", "you", "at", "9"}, strings.NewReader("ignored"))
	if err != nil || got != "see you at 9" {
		t.Errorf("got %q, %v", got, err)
	}
	got, err = composeText(nil, strings.NewReader("first line\nsecond line\n"))
	if err != nil || got != "first line\nsecond line" {
		t.Errorf("got %q, %v", got, err)
	}
	_, err = composeText(nil, strings.NewReader("  \n"))
	if exitOf(t, err) != usageCode {
		t.Errorf("an empty text exited %d", exitOf(t, err))
	}
}

// send queues the text, its markup read as the composer reads it, waits for it
// to go and prints the message it became (#181).
func TestSend_QueuesWaitsAndPrintsTheMessage(t *testing.T) {
	acc := sendingTo(bob, sentAs(512))
	var out bytes.Buffer

	err := sendMessage(shortCtx(t), acc, &out, "@bob", "see you **at 9**", sendQuery{timeout: 30 * time.Second})

	if err != nil {
		t.Fatal(err)
	}
	if len(acc.sent) != 1 {
		t.Fatalf("sent %d times", len(acc.sent))
	}
	req := acc.sent[0]
	if req.ChatID != 42 || req.Text != "see you at 9" || len(req.Entities) != 1 || req.Ref == "" {
		t.Errorf("sent %+v", req)
	}
	if w := acc.awaited[0]; w <= 29*time.Second || w > 30*time.Second {
		t.Errorf("waited %v, want the 30s asked for", w)
	}
	if out.String() != "512\n" {
		t.Errorf("printed %q", out.String())
	}
}

// --plain sends the text as it is, markup characters and all.
func TestSend_PlainSendsTheTextAsItIs(t *testing.T) {
	acc := sendingTo(bob, sentAs(1))

	if err := sendMessage(shortCtx(t), acc, &bytes.Buffer{}, "@bob", "2**3**4", sendQuery{plain: true, timeout: time.Second}); err != nil {
		t.Fatal(err)
	}

	if req := acc.sent[0]; req.Text != "2**3**4" || len(req.Entities) != 0 {
		t.Errorf("sent %+v", req)
	}
}

// --ref names the send, so that the same command run again does not send it
// twice; the owner keeps it from going twice.
func TestSend_TheRefGivenNamesTheSend(t *testing.T) {
	acc := sendingTo(bob, sentAs(1))

	if err := sendMessage(shortCtx(t), acc, &bytes.Buffer{}, "@bob", "hi", sendQuery{ref: "nightly-report", timeout: time.Second}); err != nil {
		t.Fatal(err)
	}

	if acc.sent[0].Ref != "nightly-report" {
		t.Errorf("sent as %q", acc.sent[0].Ref)
	}
}

// As JSON the message is one object: where it went, which it is and when.
func TestSend_PrintsJSON(t *testing.T) {
	acc := sendingTo(bob, sentAs(512))
	var out bytes.Buffer

	if err := sendMessage(shortCtx(t), acc, &out, "@bob", "hi", sendQuery{asJSON: true, timeout: time.Second}); err != nil {
		t.Fatal(err)
	}

	want := `{"chat_id":42,"message_id":512,"date":"2026-10-25T09:30:00Z"}` + "\n"
	if out.String() != want {
		t.Errorf("printed %s, want %s", out.String(), want)
	}
}

// A forum with no topic named is written to in General, as Telegram's own
// apps do; a topic of a chat that is no forum is no topic (#181).
func TestSend_AForumWithNoTopicIsWrittenToInGeneral(t *testing.T) {
	forum := sendingTo(project.ChatRow{ID: 300, Title: "Gophers", IsForum: true}, sentAs(1))
	if err := sendMessage(shortCtx(t), forum, &bytes.Buffer{}, "@bob", "hi", sendQuery{timeout: time.Second}); err != nil {
		t.Fatal(err)
	}
	if forum.sent[0].TopicID != domain.GeneralTopicID {
		t.Errorf("sent to topic %d", forum.sent[0].TopicID)
	}

	chat := sendingTo(bob)
	err := sendMessage(shortCtx(t), chat, &bytes.Buffer{}, "@bob", "hi", sendQuery{topic: 3, timeout: time.Second})
	if exitOf(t, err) != notFoundCode {
		t.Errorf("exited %d", exitOf(t, err))
	}
	if len(chat.sent) != 0 {
		t.Error("sent anyway")
	}
}

// A send that will not go exits 6 and says why; it stays in the chat for a
// person to retry or discard (#181).
func TestSend_AFailureExitsSix(t *testing.T) {
	failed := outcome{outcome: core.SendOutcome{Entry: &domain.OutboxEntry{
		Ref: "r", State: domain.OutboxFailed, ErrKind: telerr.Forbidden, ErrDetail: "CHAT_WRITE_FORBIDDEN",
	}}}
	acc := sendingTo(bob, failed)

	err := sendMessage(shortCtx(t), acc, &bytes.Buffer{}, "@bob", "hi", sendQuery{timeout: time.Second})

	if exitOf(t, err) != notSentCode {
		t.Errorf("exited %d", exitOf(t, err))
	}
	assertSays(t, err.Error(), "forbidden", "retry or discard")
}

// Still queued when the time is up exits 75: it will go on its own, and the
// same --ref will not send it twice (#181).
func TestSend_StillQueuedExitsSeventyFive(t *testing.T) {
	queued := outcome{outcome: core.SendOutcome{Entry: &domain.OutboxEntry{Ref: "r", State: domain.OutboxQueued}}}
	acc := sendingTo(bob, queued)

	err := sendMessage(shortCtx(t), acc, &bytes.Buffer{}, "@bob", "hi", sendQuery{ref: "r", timeout: time.Second})

	if exitOf(t, err) != laterCode {
		t.Errorf("exited %d", exitOf(t, err))
	}
	assertSays(t, err.Error(), "queued", "r")
}

// Losing the tele holding the account while waiting leaves the send queued,
// since it is on disk by then: 75, not a failure.
func TestSend_LosingTheHostWhileWaitingExitsSeventyFive(t *testing.T) {
	lost := outcome{err: &telerr.Error{Kind: telerr.Network, Detail: "the tele holding the account went away"}}
	acc := sendingTo(bob, lost)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := sendMessage(ctx, acc, &bytes.Buffer{}, "@bob", "hi", sendQuery{timeout: 200 * time.Millisecond})

	if exitOf(t, err) != laterCode {
		t.Errorf("exited %d", exitOf(t, err))
	}
}

// --no-wait returns once the send is queued, saying its ref and where it
// stands.
func TestSend_NoWaitSaysTheRefAndWhereItStands(t *testing.T) {
	queued := outcome{outcome: core.SendOutcome{Entry: &domain.OutboxEntry{Ref: "r", State: domain.OutboxQueued}}}
	acc := sendingTo(bob, queued)
	var out bytes.Buffer

	if err := sendMessage(shortCtx(t), acc, &out, "@bob", "hi", sendQuery{ref: "r", noWait: true, asJSON: true}); err != nil {
		t.Fatal(err)
	}

	if acc.awaited[0] != 0 {
		t.Errorf("waited %v", acc.awaited[0])
	}
	if out.String() != `{"ref":"r","state":"queued"}`+"\n" {
		t.Errorf("printed %q", out.String())
	}
}
