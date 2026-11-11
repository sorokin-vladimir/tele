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
)

// maxSearchLimit bounds --limit: Telegram answers a search a page at a time,
// and paging through more is not there yet (#351).
const maxSearchLimit = 100

// newSearchCmd is `tele search TEXT`: the messages that hold a text, in one
// chat, one topic of a forum, or every chat of the main list, newest first.
// The answer is Telegram's, over the whole history (#348).
func newSearchCmd(o *options) *cobra.Command {
	var q searchQuery
	cmd := &cobra.Command{
		Use:   "search TEXT",
		Short: "Search messages in a chat or in every chat",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := strings.TrimSpace(args[0])
			switch {
			case text == "":
				return &exitCode{code: usageCode, msg: "Say what to search for."}
			case q.limit < 1 || q.limit > maxSearchLimit:
				return &exitCode{code: usageCode, msg: fmt.Sprintf("--limit is from 1 to %d.", maxSearchLimit)}
			case q.topic != 0 && q.chat == "":
				return &exitCode{code: usageCode, msg: "--topic needs --chat: a topic is one of a forum's."}
			}
			if q.chat != "" {
				if _, err := parseChatName(q.chat); err != nil {
					return err
				}
			}
			return askAccount(cmd.Context(), *o, readyTimeout, func(ctx context.Context, a heldAccount) error {
				return searchMessages(ctx, a, cmd.OutOrStdout(), text, q)
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&q.chat, "chat", "", "search this chat only, by id, @username or me; every chat of the main list when left out")
	f.IntVar(&q.topic, "topic", 0, "search one topic of the forum --chat names, by id")
	f.IntVar(&q.limit, "limit", 20, fmt.Sprintf("how many messages, at most %d", maxSearchLimit))
	f.BoolVar(&q.asJSON, "json", false, "print a JSON object a message, as docs/cli.md describes")
	return cmd
}

// searchQuery is what `tele search` was asked for.
type searchQuery struct {
	chat         string
	topic, limit int
	asJSON       bool
}

// searchMessages prints the messages holding text within the scope q names,
// newest first. Nothing found prints nothing.
func searchMessages(ctx context.Context, a heldAccount, out io.Writer, text string, q searchQuery) error {
	var scope project.SearchScope
	if q.chat != "" {
		row, err := resolveChat(ctx, a, q.chat)
		if err != nil {
			return err
		}
		if !row.IsForum && q.topic != 0 {
			return &exitCode{code: notFoundCode, msg: fmt.Sprintf("%s is not a forum: it has no topics.", row.Title)}
		}
		scope = project.SearchScope{ChatID: row.ID, TopicID: q.topic}
	}
	found, err := a.SearchMessages(ctx, scope, text, q.limit)
	if err != nil {
		return err
	}
	if q.asJSON {
		return searchJSON(out, found, a.me)
	}
	_, err = io.WriteString(out, searchText(found, a.cfg.CLI.NameWidth, time.Local))
	return err
}

// foundJSON is a message found as `tele search --json` prints it: the chat it
// was found in, then the message in the shape `tele show --json` prints, a
// contract kept within a major version (docs/cli.md).
type foundJSON struct {
	ChatID  int64  `json:"chat_id"`
	Chat    string `json:"chat"`
	TopicID int    `json:"topic_id,omitempty"`
	messageJSON
}

// searchJSON writes found to out as JSON Lines, an object a message.
func searchJSON(out io.Writer, found []project.FoundMessage, me project.LoggedInUser) error {
	enc := json.NewEncoder(out)
	for _, f := range found {
		m := f.Message
		if err := enc.Encode(foundJSON{ChatID: m.ChatID, Chat: f.Chat, TopicID: m.TopicID, messageJSON: messageOf(m, me)}); err != nil {
			return err
		}
	}
	return nil
}

// searchText is the messages found as a person reads them, a line each: the
// id first, as `tele show --around` takes it, then when, the chat and topic,
// who, and how the message starts. Names wider than nameWidth are cut short,
// and every column starts where the widest entry above it leaves room.
func searchText(found []project.FoundMessage, nameWidth int, loc *time.Location) string {
	type line struct{ id, when, where, who, says string }
	lines := make([]line, 0, len(found))
	var idW, whereW, whoW int
	for _, f := range found {
		m := f.Message
		where := cutName(f.Chat, nameWidth)
		if m.TopicID != 0 {
			where += fmt.Sprintf(" › topic %d", m.TopicID)
		}
		first, rest, more := strings.Cut(body(m), "\n")
		if more && strings.TrimSpace(rest) != "" {
			first += " …"
		}
		l := line{
			id:    fmt.Sprint(m.ID),
			when:  m.Date.In(loc).Format("2006-01-02 15:04"),
			where: where,
			who:   shownName(m, nameWidth) + ":",
			says:  first,
		}
		idW = max(idW, len(l.id))
		whereW = max(whereW, ansi.StringWidth(l.where))
		whoW = max(whoW, ansi.StringWidth(l.who))
		lines = append(lines, l)
	}
	pad := func(s string, w int) string { return s + strings.Repeat(" ", w-ansi.StringWidth(s)) }
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n", pad(l.id, idW), l.when, pad(l.where, whereW), pad(l.who, whoW), l.says)
	}
	return b.String()
}
