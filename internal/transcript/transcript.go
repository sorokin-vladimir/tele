// Package transcript lays messages out as a person reads them, a line per
// message with its time and sender: what `tele show` prints.
package transcript

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// quoteWidth is how much of a quoted message a reply line shows.
const quoteWidth = 60

// Format is how Text lays the messages out.
type Format struct {
	// NameWidth is how many columns a name gets before it is cut short.
	NameWidth int
	// Location is the time zone the times are in.
	Location *time.Location
	// Self names your own messages, and an empty one names them You.
	Self string
	// Quotable is what a reply line may quote besides the messages shown.
	Quotable []domain.Message
	// OmitUnquoted leaves out the reply line of a reply to a message neither
	// shown nor quotable, instead of naming that message by its id.
	OmitUnquoted bool
}

// Text is the messages as a person reads them: a line per message with its
// time and sender, the text of every message starting in one column, a date
// line wherever the day changes, and a reply line under a reply. A name wider
// than f.NameWidth columns is cut short, so one long name does not push
// everybody's text to the right.
func Text(msgs []domain.Message, f Format) string {
	byID := make(map[int]domain.Message, len(msgs)+len(f.Quotable))
	for _, m := range f.Quotable {
		byID[m.ID] = m
	}
	nameW := 0
	for _, m := range msgs {
		byID[m.ID] = m
		nameW = max(nameW, ansi.StringWidth(f.Name(m)))
	}
	indent := strings.Repeat(" ", len("15:04 ")+nameW+len(":  "))

	var b strings.Builder
	day := ""
	for _, m := range msgs {
		t := m.Date.In(f.Location)
		if d := t.Format(time.DateOnly); d != day {
			day = d
			b.WriteString(d + "\n")
		}
		name := f.Name(m) + ":"
		fmt.Fprintf(&b, "%s %s%s  ", t.Format("15:04"), name, strings.Repeat(" ", nameW+1-ansi.StringWidth(name)))
		for i, line := range strings.Split(Body(m), "\n") {
			if i > 0 {
				b.WriteString(indent)
			}
			b.WriteString(line + "\n")
		}
		if _, quotable := byID[m.ReplyToMsgID]; m.ReplyToMsgID != 0 && (quotable || !f.OmitUnquoted) {
			b.WriteString("  ↳ " + f.quote(m.ReplyToMsgID, byID) + "\n")
		}
	}
	return b.String()
}

// Name is who sent m as a line names them, cut to f.NameWidth columns.
func (f Format) Name(m domain.Message) string {
	name := m.SenderName
	switch {
	case m.IsOut && f.Self != "":
		name = f.Self
	case m.IsOut:
		name = "You"
	case name == "":
		name = "?"
	}
	return CutName(name, f.NameWidth)
}

// quote is what a reply line says of the message replied to: who wrote it and
// how it starts, when it is among those shown.
func (f Format) quote(id int, byID map[int]domain.Message) string {
	orig, ok := byID[id]
	if !ok {
		return fmt.Sprintf("reply to %d", id)
	}
	first, _, _ := strings.Cut(Body(orig), "\n")
	return f.Name(orig) + ": " + ansi.Truncate(first, quoteWidth, "…")
}

// Body is what a message says: its media named, then its text.
func Body(m domain.Message) string {
	if m.Media == nil {
		return m.Text
	}
	label := "[" + m.Media.Label() + "]"
	if m.Text == "" {
		return label
	}
	return label + " " + m.Text
}

// CutName cuts a name or title wider than width columns short, so one long
// one does not push everything after it to the right. The ellipsis follows
// the last word cut rather than a space. A width of zero cuts nothing.
func CutName(name string, width int) string {
	if width <= 0 || ansi.StringWidth(name) <= width {
		return name
	}
	return strings.TrimRight(ansi.Truncate(name, width-1, ""), " ") + "…"
}
