package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// newChatsCmd is `tele chats`: the chats of the account, a line each (#181).
func newChatsCmd(o *options) *cobra.Command {
	var q chatsQuery
	var archived bool
	cmd := &cobra.Command{
		Use:   "chats",
		Short: "List the chats",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := listLimit(cmd, q.limit); err != nil {
				return err
			}
			if archived {
				q.folder = domain.ArchiveFolderID
			}
			return askAccount(cmd.Context(), *o, readyTimeout, func(ctx context.Context, a heldAccount) error {
				q.nameWidth = a.cfg.CLI.NameWidth
				return listChats(ctx, a, cmd.OutOrStdout(), q)
			})
		},
	}
	f := cmd.Flags()
	f.BoolVar(&q.asJSON, "json", false, "print a JSON object a chat, as docs/cli.md describes")
	f.BoolVar(&q.unread, "unread", false, "only the chats with something unread")
	f.BoolVar(&archived, "archived", false, "the archived chats instead of the main list")
	f.IntVar(&q.folder, "folder", 0, "the chats of the folder with this id instead of the main list")
	f.IntVar(&q.limit, "limit", 0, "how many chats at most, the first of the list; all when left out")
	cmd.MarkFlagsMutuallyExclusive("archived", "folder")
	return cmd
}

// chatsQuery is what `tele chats` was asked for.
type chatsQuery struct {
	// folder is the list to print: 0 for the main list,
	// domain.ArchiveFolderID for the archive, or a folder's id.
	folder int
	unread bool
	asJSON bool
	// limit is how many chats to print at most, of what the other flags
	// leave; 0 prints them all.
	limit int
	// nameWidth is the columns a title gets before it is cut short.
	nameWidth int
}

// firstOf is the first limit of rows, every one for a limit of 0.
func firstOf[T any](rows []T, limit int) []T {
	if limit > 0 && len(rows) > limit {
		return rows[:limit]
	}
	return rows
}

// listLimit refuses a --limit below 1 as a mistake in the command line. Left
// out, it lists everything.
func listLimit(cmd *cobra.Command, n int) error {
	if cmd.Flags().Changed("limit") && n < 1 {
		return &exitCode{code: usageCode, msg: "--limit is 1 or more; leave it out for all of them."}
	}
	return nil
}

// listChats prints the chats of the list q names, in its order: as text, or
// as JSON Lines. The list is the one a window would show, folder rules and
// all, since only the owner holds what they are judged on.
func listChats(ctx context.Context, acc accountSurface, out io.Writer, q chatsQuery) error {
	d, err := firstDelta(ctx, acc, project.ChatListWindow{Folder: q.folder, Limit: math.MaxInt32})
	if err != nil {
		return err
	}
	list := d.ChatList
	if list == nil {
		return errors.New("the account answered with something other than its chats")
	}
	if q.folder != 0 && q.folder != domain.ArchiveFolderID && !hasFolder(list.Folders.Filters, q.folder) {
		return &exitCode{code: notFoundCode, msg: fmt.Sprintf(
			"There is no folder %d. The folders are:\n%s", q.folder, strings.TrimSuffix(foldersText(list.Folders.Filters), "\n"))}
	}
	var rows []project.ChatRow
	for _, c := range list.Rows {
		if !q.unread || c.Unread > 0 || c.UnreadMark {
			rows = append(rows, c)
		}
	}
	rows = firstOf(rows, q.limit)
	if q.asJSON {
		return chatsJSON(out, rows)
	}
	_, err = io.WriteString(out, chatsText(rows, q.nameWidth))
	return err
}

func hasFolder(filters []domain.FolderFilter, id int) bool {
	for _, f := range filters {
		if f.ID == id {
			return true
		}
	}
	return false
}

// foldersText is a line per folder the account has, its id and title. All
// Chats and the Archive are not folders here: they are the list and
// --archived.
func foldersText(filters []domain.FolderFilter) string {
	var b strings.Builder
	for _, f := range filters {
		if f.ID == 0 || f.ID == domain.ArchiveFolderID {
			continue
		}
		fmt.Fprintf(&b, "  %d %s\n", f.ID, f.Title)
	}
	if b.Len() == 0 {
		return "  none\n"
	}
	return b.String()
}

// chatJSON is a chat as `tele chats --json` prints it. Its fields are a
// contract kept within a major version (docs/cli.md), so it is its own type
// rather than ChatRow with tags.
type chatJSON struct {
	ID       int64            `json:"id"`
	Title    string           `json:"title"`
	Username string           `json:"username,omitempty"`
	Kind     project.ChatKind `json:"kind"`
	// Unread is a forum's topics with unread, as its badge counts them.
	Unread     int  `json:"unread"`
	Mentions   int  `json:"mentions"`
	Reactions  int  `json:"reactions"`
	UnreadMark bool `json:"unread_mark"`
	Muted      bool `json:"muted"`
	Archived   bool `json:"archived"`
	Forum      bool `json:"forum"`
}

// chatsJSON writes rows to out as JSON Lines, an object a chat.
func chatsJSON(out io.Writer, rows []project.ChatRow) error {
	enc := json.NewEncoder(out)
	for _, c := range rows {
		err := enc.Encode(chatJSON{
			ID: c.ID, Title: c.Title, Username: c.Username, Kind: c.Kind,
			Unread: c.Unread, Mentions: c.Mentions, Reactions: c.Reactions,
			UnreadMark: c.UnreadMark, Muted: c.Muted, Archived: c.Archived, Forum: c.IsForum,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// chatsText is a line per chat: its id, its title and its badges, in columns.
func chatsText(rows []project.ChatRow, nameWidth int) string {
	lines := make([]columnLine, 0, len(rows))
	for _, c := range rows {
		l := columnLine{id: fmt.Sprint(c.ID), title: c.Title, badges: chatBadges(c)}
		if c.Username != "" {
			l.username = "@" + c.Username
		}
		lines = append(lines, l)
	}
	return columnsText(lines, nameWidth)
}

// cutName cuts a name or title wider than width columns short, so one long
// one does not push everything after it to the right. The ellipsis follows
// the last word cut rather than a space. A width of zero cuts nothing.
func cutName(name string, width int) string {
	if width <= 0 || ansi.StringWidth(name) <= width {
		return name
	}
	return strings.TrimRight(ansi.Truncate(name, width-1, ""), " ") + "…"
}

// columnLine is one line of a listing: what it lists by id, username and
// title, and its badges.
type columnLine struct {
	id, username, title, badges string
}

// columnsText lays lines out in columns, measured as the terminal draws them,
// with each title cut at nameWidth. A username is never cut, since a cut one
// cannot be pasted into a command, and its column is left out when no line
// has one. A line with no badges ends at its title, so none carries trailing
// spaces.
func columnsText(lines []columnLine, nameWidth int) string {
	idW, userW, titleW := 0, 0, 0
	for i, l := range lines {
		lines[i].title = cutName(l.title, nameWidth)
		idW = max(idW, len(l.id))
		userW = max(userW, len(l.username))
		titleW = max(titleW, ansi.StringWidth(lines[i].title))
	}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.id)
		b.WriteString(strings.Repeat(" ", idW-len(l.id)+2))
		if userW > 0 {
			b.WriteString(l.username)
			b.WriteString(strings.Repeat(" ", userW-len(l.username)+2))
		}
		b.WriteString(l.title)
		if l.badges != "" {
			b.WriteString(strings.Repeat(" ", titleW-ansi.StringWidth(l.title)+2))
			b.WriteString(l.badges)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// chatBadges says what the chat list would badge a chat with. A forum's unread
// count is of its topics, as Telegram Desktop badges one.
func chatBadges(c project.ChatRow) string {
	var bs []string
	if c.IsForum {
		bs = append(bs, "forum")
		if c.Unread > 0 {
			bs = append(bs, count(c.Unread, "topic")+" unread")
		}
	} else if c.Unread > 0 {
		bs = append(bs, fmt.Sprintf("%d unread", c.Unread))
	}
	if c.Mentions > 0 {
		bs = append(bs, count(c.Mentions, "mention"))
	}
	if c.Reactions > 0 {
		bs = append(bs, count(c.Reactions, "reaction"))
	}
	if c.UnreadMark {
		bs = append(bs, "marked unread")
	}
	if c.Muted {
		bs = append(bs, "muted")
	}
	return strings.Join(bs, ", ")
}

// count is n of noun, the noun made plural for any n but one.
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
