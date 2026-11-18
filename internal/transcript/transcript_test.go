package transcript_test

import (
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/transcript"
)

func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

// Every message's text starts in the same column, after the widest name
// shown; a date line comes first and wherever the day changes, and a reply
// says what it answers under it. Your own messages are yours (#181).
func TestText_AlignsTheTextAfterTheNames(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, Date: at(24, 21, 58), SenderID: 42, SenderName: "Bob", Text: "see you tomorrow"},
		{ID: 2, Date: at(25, 9, 12), IsOut: true, Text: "morning!", ReplyToMsgID: 1},
		{ID: 3, Date: at(25, 9, 15), SenderID: 43, SenderName: "Alexandra K.", Text: "first line\nsecond line"},
		{ID: 4, Date: at(25, 9, 16), SenderID: 42, SenderName: "Bob", Text: "an answer", ReplyToMsgID: 900},
	}

	got := transcript.Text(msgs, transcript.Format{NameWidth: 20, Location: time.UTC})

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
func TestText_CutsANameWiderThanTheColumn(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, Date: at(25, 9, 0), SenderName: "Alexander Konstantinovich", Text: "hi"},
		{ID: 2, Date: at(25, 9, 1), SenderName: "Bob", Text: "hello"},
	}

	got := transcript.Text(msgs, transcript.Format{NameWidth: 10, Location: time.UTC})

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
func TestText_NamesTheMedia(t *testing.T) {
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

		got := transcript.Text(msgs, transcript.Format{NameWidth: 20, Location: time.UTC})

		want := "2026-10-25\n09:00 Bob:  " + c.want + "\n"
		if got != want {
			t.Errorf("printed %q, want %q", got, want)
		}
	}
}

func TestText_NamesYourOwnMessagesAsSelf(t *testing.T) {
	msgs := []domain.Message{
		{ID: 1, Date: at(25, 9, 0), SenderName: "Bob", Text: "hi"},
		{ID: 2, Date: at(25, 9, 1), IsOut: true, Text: "hello", ReplyToMsgID: 1},
	}

	got := transcript.Text(msgs, transcript.Format{NameWidth: 20, Location: time.UTC, Self: "Ada Lovelace"})

	want := "" +
		"2026-10-25\n" +
		"09:00 Bob:           hi\n" +
		"09:01 Ada Lovelace:  hello\n" +
		"  ↳ Bob: hi\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

func TestText_QuotesAQuotableMessageNotShown(t *testing.T) {
	quotable := []domain.Message{{ID: 1, Date: at(25, 8, 0), SenderName: "Bob", Text: "first line\nsecond line"}}
	msgs := []domain.Message{{ID: 2, Date: at(25, 9, 0), SenderName: "Eve", Text: "yes", ReplyToMsgID: 1}}

	got := transcript.Text(msgs, transcript.Format{NameWidth: 20, Location: time.UTC, Quotable: quotable, OmitUnquoted: true})

	want := "" +
		"2026-10-25\n" +
		"09:00 Eve:  yes\n" +
		"  ↳ Bob: first line\n"
	if got != want {
		t.Errorf("printed\n%s\nwant\n%s", got, want)
	}
}

func TestText_OmitUnquotedDropsTheLineOfAReplyToAMessageNotLoaded(t *testing.T) {
	msgs := []domain.Message{{ID: 2, Date: at(25, 9, 0), SenderName: "Eve", Text: "yes", ReplyToMsgID: 511}}

	got := transcript.Text(msgs, transcript.Format{NameWidth: 20, Location: time.UTC, OmitUnquoted: true})

	want := "2026-10-25\n09:00 Eve:  yes\n"
	if got != want {
		t.Errorf("printed %q, want %q", got, want)
	}
}
