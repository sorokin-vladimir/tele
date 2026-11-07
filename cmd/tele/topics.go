package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/core/project"
)

// newTopicsCmd is `tele topics CHAT`: a forum's topics, a line each (#181).
func newTopicsCmd(o *options) *cobra.Command {
	var q topicsQuery
	cmd := &cobra.Command{
		Use:   "topics CHAT",
		Short: "List the topics of a forum",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := parseChatName(args[0]); err != nil {
				return err
			}
			if err := listLimit(cmd, q.limit); err != nil {
				return err
			}
			return askAccount(cmd.Context(), *o, readyTimeout, func(ctx context.Context, a heldAccount) error {
				q.nameWidth = a.cfg.CLI.NameWidth
				return listTopics(ctx, a, cmd.OutOrStdout(), args[0], q)
			})
		},
	}
	cmd.Flags().BoolVar(&q.asJSON, "json", false, "print a JSON object a topic, as docs/cli.md describes")
	cmd.Flags().IntVar(&q.limit, "limit", 0, "how many topics at most, the first of the list; all when left out")
	return cmd
}

// topicsQuery is what `tele topics` was asked for.
type topicsQuery struct {
	asJSON bool
	// limit is how many topics to print at most; 0 prints them all.
	limit int
	// nameWidth is the columns a title gets before it is cut short.
	nameWidth int
}

// listTopics prints the topics of the forum chat names, in the order a window
// lists them: as text, or as JSON Lines. A chat that is not a forum has none,
// and exits 5.
func listTopics(ctx context.Context, acc accountSurface, out io.Writer, chat string, q topicsQuery) error {
	forum, err := resolveChat(ctx, acc, chat)
	if err != nil {
		return err
	}
	if !forum.IsForum {
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf("%s is not a forum: it has no topics.", forum.Title)}
	}
	topics, err := acc.Topics(ctx, forum.ID)
	if err != nil {
		return err
	}
	topics = firstOf(topics, q.limit)
	if q.asJSON {
		return topicsJSON(out, topics)
	}
	lines := make([]columnLine, 0, len(topics))
	for _, t := range topics {
		lines = append(lines, columnLine{id: fmt.Sprint(t.ID), title: t.Title, badges: topicBadges(t)})
	}
	_, err = io.WriteString(out, columnsText(lines, q.nameWidth))
	return err
}

// topicBadges says what the topic list would badge a topic with.
func topicBadges(t project.TopicRow) string {
	var bs []string
	if t.Unread > 0 {
		bs = append(bs, fmt.Sprintf("%d unread", t.Unread))
	}
	if t.Mentions > 0 {
		bs = append(bs, count(t.Mentions, "mention"))
	}
	if t.Reactions > 0 {
		bs = append(bs, count(t.Reactions, "reaction"))
	}
	if t.Muted {
		bs = append(bs, "muted")
	}
	if t.Pinned {
		bs = append(bs, "pinned")
	}
	if t.Closed {
		bs = append(bs, "closed")
	}
	return strings.Join(bs, ", ")
}

// topicJSON is a topic as `tele topics --json` prints it, a contract kept
// within a major version (docs/cli.md).
type topicJSON struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Unread    int    `json:"unread"`
	Mentions  int    `json:"mentions"`
	Reactions int    `json:"reactions"`
	Muted     bool   `json:"muted"`
	Pinned    bool   `json:"pinned"`
	Closed    bool   `json:"closed"`
}

// topicsJSON writes topics to out as JSON Lines, an object a topic.
func topicsJSON(out io.Writer, topics []project.TopicRow) error {
	enc := json.NewEncoder(out)
	for _, t := range topics {
		err := enc.Encode(topicJSON{
			ID: t.ID, Title: t.Title, Unread: t.Unread, Mentions: t.Mentions, Reactions: t.Reactions,
			Muted: t.Muted, Pinned: t.Pinned, Closed: t.Closed,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
