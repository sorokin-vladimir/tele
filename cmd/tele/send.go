package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/markup"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// notSentCode is what send exits with when the message will not go, and
// trying again will not change that (#181).
const notSentCode = 6

// laterCode is what send exits with when it gave up waiting while the message
// was still queued: it goes on its own, and may have by the time a script
// looks (#181).
const laterCode = 75

// lostHostPause is how long send waits before asking again about a send it
// lost the answer to, while a service comes back.
const lostHostPause = 500 * time.Millisecond

// newSendCmd is `tele send CHAT [TEXT...]`: a message to a chat, waited for
// until it is one (#181).
func newSendCmd(o *options) *cobra.Command {
	var q sendQuery
	cmd := &cobra.Command{
		Use:   "send CHAT [TEXT...]",
		Short: "Send a message to a chat",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := parseChatName(args[0]); err != nil {
				return err
			}
			if q.timeout <= 0 {
				return &exitCode{code: usageCode, msg: "--timeout is more than nothing, such as 30s or 2m."}
			}
			text, err := composeText(args[1:], cmd.InOrStdin())
			if err != nil {
				return err
			}
			within := readyTimeout
			if !q.noWait {
				// The wait is send's own, on top of reaching the account.
				within = q.timeout + readyTimeout
			}
			return askAccount(cmd.Context(), *o, within, func(ctx context.Context, a heldAccount) error {
				return sendMessage(ctx, a, cmd.OutOrStdout(), args[0], text, q)
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&q.topic, "topic", 0, "the topic of a forum to send to, by id; General when left out")
	f.BoolVar(&q.plain, "plain", false, "send the text as it is, with no markup read in it")
	f.DurationVar(&q.timeout, "timeout", 30*time.Second, "how long to wait for the message to go")
	f.BoolVar(&q.noWait, "no-wait", false, "return once the message is queued, without waiting for it to go")
	f.StringVar(&q.ref, "ref", "", "name the send, so that running the same command again does not send it twice")
	f.BoolVar(&q.asJSON, "json", false, "print the answer as JSON, as docs/cli.md describes")
	return cmd
}

// sendQuery is what `tele send` was asked for.
type sendQuery struct {
	topic   int
	plain   bool
	timeout time.Duration
	noWait  bool
	ref     string
	asJSON  bool
}

// composeText is the message: the words given joined by spaces, or with none
// what comes on stdin, trimmed as the composer trims it. Nothing to send is a
// mistake in the command line.
func composeText(args []string, in io.Reader) (string, error) {
	text := strings.Join(args, " ")
	if len(args) == 0 {
		raw, err := io.ReadAll(in)
		if err != nil {
			return "", fmt.Errorf("reading the message from stdin: %w", err)
		}
		text = string(raw)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", &exitCode{code: usageCode, msg: "There is nothing to send: give the text, or pipe it in."}
	}
	return text, nil
}

// sendMessage queues text to the chat named chat and, unless told not to,
// waits until it is a message or will not be one. The send is on disk once
// Send returns, so whatever happens after - a timeout, the host going - leaves
// it queued, which is exit 75 and never a failure.
func sendMessage(ctx context.Context, acc accountSurface, out io.Writer, chat, text string, q sendQuery) error {
	row, err := resolveChat(ctx, acc, chat)
	if err != nil {
		return err
	}
	topic := q.topic
	switch {
	case !row.IsForum && topic != 0:
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf("%s is not a forum: it has no topics.", row.Title)}
	case row.IsForum && topic == 0:
		// As Telegram's own apps do.
		topic = domain.GeneralTopicID
	}
	req := core.SendRequest{Ref: q.ref, ChatID: row.ID, TopicID: topic, Text: text}
	if req.Ref == "" {
		req.Ref = core.NewRef()
	}
	if !q.plain {
		req.Text, req.Entities = markup.Parse(text)
	}
	if err := acc.Send(ctx, req); err != nil {
		return err
	}

	if q.noWait {
		got, err := acc.AwaitSend(ctx, req.Ref, 0)
		if err != nil {
			return err
		}
		return printQueued(out, req.Ref, got, q.asJSON)
	}

	deadline := time.Now().Add(q.timeout)
	for {
		got, err := acc.AwaitSend(ctx, req.Ref, time.Until(deadline))
		switch {
		case err == nil:
			return sendAnswer(out, row.Title, req.Ref, got, q)
		case telerr.Of(err) == telerr.CommandsOff:
			return err
		case telerr.Of(err) == telerr.NotFound:
			return fmt.Errorf("the message to %s was discarded before it went", row.Title)
		}
		// The answer was lost, most likely with the tele holding the account:
		// a service comes back, and is asked again while there is time.
		lost := &exitCode{code: laterCode, msg: fmt.Sprintf(
			"Lost the tele holding the account while waiting (%v): the message is queued and goes once it is back. "+
				"--ref %s waits for it again without sending it twice.", err, req.Ref)}
		left := time.Until(deadline)
		if left <= 0 || ctx.Err() != nil {
			return lost
		}
		select {
		case <-time.After(min(lostHostPause, left)):
		case <-ctx.Done():
		}
		if time.Until(deadline) <= 0 || ctx.Err() != nil {
			return lost
		}
	}
}

// sendAnswer says what became of a send that was waited for: the message it
// is, exit 6 for one that will not go, exit 75 for one still queued.
func sendAnswer(out io.Writer, title, ref string, got core.SendOutcome, q sendQuery) error {
	if s := got.Sent; s != nil {
		return printSent(out, *s, q.asJSON)
	}
	e := got.Entry
	if e.State == domain.OutboxFailed {
		why := string(e.ErrReason)
		if why == "" {
			why = string(e.ErrKind)
		}
		if e.ErrDetail != "" {
			why += " (" + e.ErrDetail + ")"
		}
		return &exitCode{code: notSentCode, msg: fmt.Sprintf(
			"The message to %s was not sent: %s. It stays in the chat, for you to retry or discard in a window.", title, why)}
	}
	waiting := "still queued"
	if e.NextAttemptAt.After(time.Now()) {
		waiting = "queued, and Telegram asked to wait until " + e.NextAttemptAt.Local().Format(time.TimeOnly)
	}
	return &exitCode{code: laterCode, msg: fmt.Sprintf(
		"The message to %s is %s after %s: it goes on its own. --ref %s waits for it again without sending it twice.",
		title, waiting, q.timeout, ref)}
}

// sentJSON is a message `tele send --json` printed, a contract kept within a
// major version (docs/cli.md).
type sentJSON struct {
	ChatID    int64     `json:"chat_id"`
	TopicID   int       `json:"topic_id,omitempty"`
	MessageID int       `json:"message_id,omitempty"`
	Date      time.Time `json:"date"`
}

// printSent prints the message a send became: its id alone, for a script to
// read, or as JSON.
func printSent(out io.Writer, s domain.SentSend, asJSON bool) error {
	id := 0
	if len(s.MsgIDs) > 0 {
		id = s.MsgIDs[0]
	}
	if asJSON {
		return json.NewEncoder(out).Encode(sentJSON{ChatID: s.ChatID, TopicID: s.TopicID, MessageID: id, Date: s.At})
	}
	if id == 0 {
		// Telegram took it and named no message.
		_, err := fmt.Fprintln(out, "sent")
		return err
	}
	_, err := fmt.Fprintln(out, id)
	return err
}

// queuedJSON is a send `tele send --no-wait --json` printed.
type queuedJSON struct {
	Ref   string `json:"ref"`
	State string `json:"state"`
}

// printQueued says where a send not waited for stands: queued, or already
// sent when its ref had gone before.
func printQueued(out io.Writer, ref string, got core.SendOutcome, asJSON bool) error {
	state := "sent"
	if got.Entry != nil {
		state = string(got.Entry.State)
	}
	if asJSON {
		return json.NewEncoder(out).Encode(queuedJSON{Ref: ref, State: state})
	}
	_, err := fmt.Fprintf(out, "%s %s\n", ref, state)
	return err
}
