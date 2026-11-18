package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/transcript"
)

// maxShowLimit bounds --limit: a command reads a screenful or a page, and a
// script wanting more pages through --before.
const maxShowLimit = 200

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
			switch {
			case q.around < 0:
				return &exitCode{code: usageCode, msg: "--around takes a message id."}
			case q.around != 0 && q.before != 0:
				return &exitCode{code: usageCode, msg: "--around and --before do not go together."}
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
	f.IntVar(&q.around, "around", 0, "show this message id with the messages around it, half of --limit either side")
	f.BoolVar(&q.markRead, "mark-read", false, "mark the chat read up to the last message shown")
	f.BoolVar(&q.asJSON, "json", false, "print a JSON object a message, as docs/cli.md describes")
	return cmd
}

// showQuery is what `tele show` was asked for.
type showQuery struct {
	topic, limit, before, around int
	markRead, asJSON             bool
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
	var msgs []domain.Message
	if q.around != 0 {
		msgs, err = a.HistoryAround(ctx, h, q.around, q.limit)
		if core.IsNoMessage(err) {
			return &exitCode{code: notFoundCode, msg: fmt.Sprintf("No message %d in %s.", q.around, row.Title)}
		}
	} else {
		msgs, err = a.History(ctx, h, q.before, q.limit)
	}
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
	_, err = io.WriteString(out, transcript.Text(msgs, transcript.Format{NameWidth: a.cfg.CLI.NameWidth, Location: time.Local}))
	return err
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
		if err := enc.Encode(messageOf(m, me)); err != nil {
			return err
		}
	}
	return nil
}

// messageOf is m as JSON prints it. Your own messages carry your id and name.
func messageOf(m domain.Message, me project.LoggedInUser) messageJSON {
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
	return j
}
