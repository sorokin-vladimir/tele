package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// maxShowLimit bounds --limit: a command reads a screenful or a page, and a
// script wanting more pages through --before.
const maxShowLimit = 200

// quoteWidth is how much of a quoted message a reply line shows.
const quoteWidth = 60

// newShowCmd is `tele show CHAT`: the newest messages of a chat, or of one
// topic of a forum, oldest first (#181).
func newShowCmd(o *options) *cobra.Command {
	var q showQuery
	cmd := &cobra.Command{
		Use:   "show CHAT",
		Short: "Show the messages of a chat",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := parseChatName(args[0]); err != nil {
				return err
			}
			if q.limit < 1 || q.limit > maxShowLimit {
				return &exitCode{code: usageCode, msg: fmt.Sprintf("--limit is from 1 to %d.", maxShowLimit)}
			}
			return askAccount(cmd.Context(), *o, readyTimeout, func(ctx context.Context, a heldAccount) error {
				return showMessages(ctx, a, cmd.OutOrStdout(), args[0], q)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&q.topic, "topic", 0, "the topic of a forum to show, by id; `tele topics CHAT` lists them")
	f.IntVar(&q.limit, "limit", 20, fmt.Sprintf("how many messages, at most %d", maxShowLimit))
	f.IntVar(&q.before, "before", 0, "show the messages older than this message id")
	f.BoolVar(&q.markRead, "mark-read", false, "mark the chat read up to the last message shown")
	f.BoolVar(&q.asJSON, "json", false, "print a JSON object a message, as docs/cli.md describes")
	return cmd
}

// showQuery is what `tele show` was asked for.
type showQuery struct {
	topic, limit, before int
	markRead, asJSON     bool
}

// showMessages prints the messages of the chat named chat that q asks for. It
// changes nothing unless asked to mark them read, and then marks read up to
// the last one shown and no further, before printing: a failure leaves
// nothing printed.
func showMessages(ctx context.Context, a heldAccount, out io.Writer, chat string, q showQuery) error {
	row, err := resolveChat(ctx, a, chat)
	if err != nil {
		return err
	}
	switch {
	case row.IsForum && q.topic == 0:
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf(
			"%s is a forum: its messages are its topics'. Name one with --topic; `tele topics %s` lists them.", row.Title, chat)}
	case !row.IsForum && q.topic != 0:
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf("%s is not a forum: it has no topics.", row.Title)}
	}
	h := domain.HistoryKey{ChatID: row.ID, TopicID: q.topic}
	msgs, err := a.History(ctx, h, q.before, q.limit)
	if telerr.Of(err) == telerr.PeerNotFound || telerr.Of(err) == telerr.NotFound {
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf("No topic %d in %s.", q.topic, row.Title)}
	}
	if err != nil {
		return err
	}
	if q.markRead && len(msgs) > 0 {
		if err := a.MarkRead(ctx, h, msgs[len(msgs)-1].ID); err != nil {
			return fmt.Errorf("marking %s read: %w", row.Title, err)
		}
	}
	if q.asJSON {
		return showJSON(out, msgs, a.me)
	}
	_, err = io.WriteString(out, showText(msgs, a.me, a.cfg.CLI.NameWidth, time.Local))
	return err
}

// showText is the messages as a person reads them: a line per message with
// its time and sender, the text of every message starting in one column, a
// date line wherever the day changes, and a reply line under a reply. A name
// wider than nameWidth columns is cut short, so one long name does not push
// everybody's text to the right.
func showText(msgs []domain.Message, me project.LoggedInUser, nameWidth int, loc *time.Location) string {
	byID := make(map[int]domain.Message, len(msgs))
	nameW := 0
	for _, m := range msgs {
		byID[m.ID] = m
		nameW = max(nameW, ansi.StringWidth(shownName(m, nameWidth)))
	}
	indent := strings.Repeat(" ", len("15:04 ")+nameW+len(":  "))

	var b strings.Builder
	day := ""
	for _, m := range msgs {
		t := m.Date.In(loc)
		if d := t.Format(time.DateOnly); d != day {
			day = d
			b.WriteString(d + "\n")
		}
		name := shownName(m, nameWidth) + ":"
		fmt.Fprintf(&b, "%s %s%s  ", t.Format("15:04"), name, strings.Repeat(" ", nameW+1-ansi.StringWidth(name)))
		for i, line := range strings.Split(body(m), "\n") {
			if i > 0 {
				b.WriteString(indent)
			}
			b.WriteString(line + "\n")
		}
		if m.ReplyToMsgID != 0 {
			b.WriteString("  ↳ " + quote(m.ReplyToMsgID, byID, nameWidth) + "\n")
		}
	}
	return b.String()
}

// shownName is who sent m as a line names them, cut to width columns.
func shownName(m domain.Message, width int) string {
	name := m.SenderName
	switch {
	case m.IsOut:
		name = "You"
	case name == "":
		name = "?"
	}
	return cutName(name, width)
}

// quote is what a reply line says of the message replied to: who wrote it and
// how it starts, when it is among those shown.
func quote(id int, byID map[int]domain.Message, nameWidth int) string {
	orig, ok := byID[id]
	if !ok {
		return fmt.Sprintf("reply to %d", id)
	}
	first, _, _ := strings.Cut(body(orig), "\n")
	return shownName(orig, nameWidth) + ": " + ansi.Truncate(first, quoteWidth, "…")
}

// body is what a message says: its media named, then its text.
func body(m domain.Message) string {
	if m.Media == nil {
		return m.Text
	}
	label := "[" + mediaLabel(*m.Media) + "]"
	if m.Text == "" {
		return label
	}
	return label + " " + m.Text
}

// mediaLabel names a piece of media and what it says about itself, without
// fetching it.
func mediaLabel(r domain.MediaRef) string {
	withTime := func(label string) string {
		return strings.TrimSpace(label + " " + clock(r.Duration))
	}
	switch r.Kind {
	case domain.MediaPhoto:
		return "photo"
	case domain.MediaVideo:
		return withTime("video")
	case domain.MediaVideoNote:
		return withTime("round video")
	case domain.MediaVoice:
		return withTime("voice")
	case domain.MediaAudio:
		name := r.FileName
		if r.Title != "" {
			name = strings.TrimPrefix(r.Performer+" – "+r.Title, " – ")
		}
		return withTime(strings.TrimSpace("audio " + name))
	case domain.MediaSticker:
		return strings.TrimSpace("sticker " + r.Emoji)
	case domain.MediaGIF:
		return "GIF"
	case domain.MediaFile:
		return strings.TrimSpace("file " + r.FileName)
	case domain.MediaLocation:
		return "location"
	default:
		return "media"
	}
}

// clock is a duration in seconds as a player shows it, m:ss or h:mm:ss; empty
// for none.
func clock(seconds int) string {
	if seconds <= 0 {
		return ""
	}
	h, m, s := seconds/3600, seconds/60%60, seconds%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// messageJSON is a message as `tele show --json` prints it, a contract kept
// within a major version (docs/cli.md). What a message does not have is left
// out; a file is described and never pointed at.
type messageJSON struct {
	ID       int        `json:"id"`
	Date     time.Time  `json:"date"`
	SenderID int64      `json:"sender_id,omitempty"`
	Sender   string     `json:"sender,omitempty"`
	Out      bool       `json:"out"`
	Text     string     `json:"text,omitempty"`
	ReplyTo  int        `json:"reply_to,omitempty"`
	Media    *mediaJSON `json:"media,omitempty"`
}

type mediaJSON struct {
	Kind      string `json:"kind"`
	Duration  int    `json:"duration,omitempty"`
	FileName  string `json:"file_name,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Title     string `json:"title,omitempty"`
	Performer string `json:"performer,omitempty"`
	Emoji     string `json:"emoji,omitempty"`
}

// mediaKinds names each kind of media as JSON does.
var mediaKinds = map[domain.MediaKind]string{
	domain.MediaPhoto:     "photo",
	domain.MediaVideo:     "video",
	domain.MediaVideoNote: "round_video",
	domain.MediaVoice:     "voice",
	domain.MediaAudio:     "audio",
	domain.MediaSticker:   "sticker",
	domain.MediaGIF:       "gif",
	domain.MediaFile:      "file",
	domain.MediaLocation:  "location",
	domain.MediaOther:     "other",
}

// showJSON writes msgs to out as JSON Lines, an object a message. Your own
// messages carry your id and name, as anybody else's carry theirs.
func showJSON(out io.Writer, msgs []domain.Message, me project.LoggedInUser) error {
	enc := json.NewEncoder(out)
	for _, m := range msgs {
		j := messageJSON{
			ID: m.ID, Date: m.Date, SenderID: m.SenderID, Sender: m.SenderName,
			Out: m.IsOut, Text: m.Text, ReplyTo: m.ReplyToMsgID,
		}
		if m.IsOut {
			j.SenderID, j.Sender = me.ID, me.Name
		}
		if r := m.Media; r != nil {
			j.Media = &mediaJSON{
				Kind: mediaKinds[r.Kind], Duration: r.Duration, FileName: r.FileName, Size: r.Size,
				Title: r.Title, Performer: r.Performer, Emoji: r.Emoji,
			}
		}
		if err := enc.Encode(j); err != nil {
			return err
		}
	}
	return nil
}
