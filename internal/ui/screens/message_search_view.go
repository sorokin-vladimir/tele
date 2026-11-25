package screens

import (
	"strings"
	"time"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// renderResult draws one result in inner columns: who and when on the first
// line, what it says on the second (#349). Each line keeps a cell of padding
// at either end.
func (m *MessageSearchModel) renderResult(f project.FoundMessage, inner int, selected bool) string {
	width := inner - 2
	plain, dim, pad := theme.S().Body, theme.S().Timestamp, theme.Pad
	if selected {
		plain, dim = theme.S().SearchActiveRow, theme.S().SearchActiveRow
		pad = func(n int) string {
			if n <= 0 {
				return ""
			}
			return theme.S().SearchActiveRow.Width(n).Render("")
		}
	}

	date := searchDate(f.Message.Date, m.now())
	who := m.who(f, selected)
	if room := width - lipgloss.Width(date) - 1; lipgloss.Width(who) > room {
		// The ellipsis is a run of its own, painted like the rest of the row.
		who = ansi.Truncate(who, max(room, 1), plain.Render("…"))
	}
	gap := width - lipgloss.Width(who) - lipgloss.Width(date)

	mark := theme.NewStyle().Foreground(theme.T().Accent).Bold(true)
	if selected {
		mark = theme.S().SearchActiveRow.Bold(true).Underline(true)
	}
	var said string
	if text := strings.Join(strings.Fields(f.Message.Text), " "); text != "" || f.Message.Media == nil {
		said = snippet(text, m.shown, width, plain, mark)
	} else {
		label := f.Message.Media.Label()
		said = dim.Render(ansi.Truncate(strings.ToUpper(label[:1])+label[1:], width, "…"))
	}

	first := pad(1) + who + pad(gap) + dim.Render(date) + pad(1)
	second := pad(1) + said + pad(width-lipgloss.Width(said)) + pad(1)
	return first + "\n" + second
}

// snippet is text in width columns with every occurrence of query marked, case
// ignored. A text too long for the line is moved so its first match shows,
// with a third of the line kept before it, and cut with … wherever it goes on.
func snippet(text, query string, width int, plain, mark lipgloss.Style) string {
	runes := []rune(text)
	marked := matches(runes, []rune(query))
	cells := make([]int, len(runes))
	for i, r := range runes {
		cells[i] = runewidth.RuneWidth(r)
	}
	span := func(from, to int) int {
		n := 0
		for _, c := range cells[from:to] {
			n += c
		}
		return n
	}

	start, end := 0, len(runes)
	if span(0, len(runes)) > width {
		if first := firstMarked(marked); first >= 0 && span(0, matchEnd(marked, first)) > width-1 {
			for start = first; start > 0 && span(start-1, first) <= width/3; start-- {
			}
			// The … stands for what was cut: a space after it says nothing.
			for start < first && unicode.IsSpace(runes[start]) {
				start++
			}
		}
		room := width
		if start > 0 {
			room-- // the leading …
		}
		if span(start, len(runes)) > room {
			room-- // the trailing …
			for end = start; end < len(runes) && span(start, end+1) <= room; end++ {
			}
		}
	}

	var b strings.Builder
	if start > 0 {
		b.WriteString(plain.Render("…"))
	}
	for i := start; i < end; {
		j := i
		for j < end && marked[j] == marked[i] {
			j++
		}
		style := plain
		if marked[i] {
			style = mark
		}
		b.WriteString(style.Render(string(runes[i:j])))
		i = j
	}
	if end < len(runes) {
		b.WriteString(plain.Render("…"))
	}
	return b.String()
}

// firstMarked is the index of the first marked rune, or -1.
func firstMarked(marked []bool) int {
	for i, on := range marked {
		if on {
			return i
		}
	}
	return -1
}

// matchEnd is the index just past the run of marked runes starting at from.
func matchEnd(marked []bool, from int) int {
	for from < len(marked) && marked[from] {
		from++
	}
	return from
}

// matches marks the runes of text that are part of an occurrence of query,
// case ignored. Occurrences do not overlap.
func matches(text, query []rune) []bool {
	marked := make([]bool, len(text))
	if len(query) == 0 {
		return marked
	}
	for i := 0; i+len(query) <= len(text); {
		hit := true
		for k, q := range query {
			if unicode.ToLower(text[i+k]) != unicode.ToLower(q) {
				hit = false
				break
			}
		}
		if !hit {
			i++
			continue
		}
		for k := range query {
			marked[i+k] = true
		}
		i += len(query)
	}
	return marked
}

// who is the first line's left side: the sender, or You, and searching every
// chat the chat and topic it was found in before them. A private chat is named
// alone, its sender being the chat, and with You on your own messages.
func (m *MessageSearchModel) who(f project.FoundMessage, selected bool) string {
	name := f.Message.SenderName
	if f.Message.IsOut {
		name = "You"
	}
	plain, dim, sender := theme.S().Body, theme.S().Timestamp, components.SenderStyle(f.Message.SenderID)
	if selected {
		plain, dim, sender = theme.S().SearchActiveRow, theme.S().SearchActiveRow, theme.S().SearchActiveRow
	}
	if !m.everywhere {
		return sender.Render(name)
	}
	where := plain.Render(f.Chat)
	if f.Topic != "" {
		where += plain.Render(" › " + f.Topic)
	}
	if f.Private && !f.Message.IsOut {
		return where
	}
	return where + dim.Render(" · ") + sender.Render(name)
}

// selectedInputLine is the query line of a reopened window: the query drawn as
// selected, the next key replacing it, and the rest of the line padded. A query
// too long for the line shows its tail, as inputLine does.
func selectedInputLine(text string, inner int) string {
	const prompt = "> "
	budget := inner - lipgloss.Width(prompt)
	if w := runewidth.StringWidth(text); w > budget {
		text = runewidth.TruncateLeft(text, w-budget+1, "…")
	}
	line := theme.S().SearchPrompt.Render(prompt) + theme.S().SearchActiveRow.Render(text)
	return line + theme.Pad(inner-lipgloss.Width(line))
}

// searchDate is when a message was sent as a result says it: the time for
// today, the day for this year, the day and the year before that, in the words
// the chat's date separators use.
func searchDate(t, now time.Time) string {
	t = t.In(now.Location())
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("January 2")
	default:
		return t.Format("January 2, 2006")
	}
}
