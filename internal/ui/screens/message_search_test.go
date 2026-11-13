package screens_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

var aliceHere = screens.MessageSearchOpen{Here: project.SearchScope{ChatID: 1}, Title: "Alice"}

// searchLines is the window as a person reads it: no colour, one string a row.
func searchLines(m *screens.MessageSearchModel) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// typeQuery types text into the window, a key at a time.
func typeQuery(t *testing.T, m *screens.MessageSearchModel, text string) *screens.MessageSearchModel {
	t.Helper()
	for _, r := range text {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// enter presses Enter and returns what the window asked for, if anything.
func enter(t *testing.T, m *screens.MessageSearchModel) (*screens.MessageSearchModel, tea.Msg) {
	t.Helper()
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

// Enter asks for the query, in the scope the window is on, from the first
// page; until the answer comes the window says it is searching (#349).
func TestMessageSearch_EnterAsksForTheQuery(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap())
	m = typeQuery(t, m, "deploy")

	m, msg := enter(t, m)

	req, ok := msg.(screens.MessageSearchRequest)
	require.True(t, ok, "Enter asks for the query, got %T", msg)
	assert.Equal(t, project.SearchScope{ChatID: 1}, req.Scope)
	assert.Equal(t, "deploy", req.Text)
	assert.Equal(t, project.SearchPosition{}, req.After)
	assert.Contains(t, searchLines(m)[3], "Searching…")
}

// Backspace takes back the last character, and pasted text is typed in.
func TestMessageSearch_TheQueryIsEdited(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap())
	m = typeQuery(t, m, "dep")
	m, _ = m.Update(tea.PasteMsg{Content: "loyé"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})

	_, msg := enter(t, m)

	req, ok := msg.(screens.MessageSearchRequest)
	require.True(t, ok)
	assert.Equal(t, "deploy", req.Text)
}

// An empty query asks nothing: every question counts against the rate limit.
func TestMessageSearch_AnEmptyQueryAsksNothing(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap())
	m = typeQuery(t, m, "  ")

	_, msg := enter(t, m)

	assert.Nil(t, msg)
}

// searchNow is the clock the window reads dates against in these tests.
var searchNow = time.Date(2026, 11, 13, 15, 0, 0, 0, time.Local)

// answered asks for query and hands the window found as the answer.
func answered(t *testing.T, m *screens.MessageSearchModel, query string, found ...project.FoundMessage) *screens.MessageSearchModel {
	t.Helper()
	m = typeQuery(t, m, query)
	m, msg := enter(t, m)
	req, ok := msg.(screens.MessageSearchRequest)
	require.True(t, ok)
	m, _ = m.Update(screens.MessageSearchResult{Serial: req.Serial, Page: project.SearchPage{Found: found}})
	return m
}

// A result is two lines, who and when, then what it says, with a blank line
// before the next: the sender, or You, and the date at the right edge (#349).
func TestMessageSearch_AResultIsWhoWhenAndWhat(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap()).WithNow(func() time.Time { return searchNow })

	m = answered(t, m, "deploy",
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 9, ChatID: 1, SenderName: "Bob", Text: "so the deploy failed", Date: time.Date(2026, 3, 12, 10, 0, 0, 0, time.Local)}},
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 7, ChatID: 1, IsOut: true, Text: "deploy is green now", Date: time.Date(2026, 11, 13, 9, 41, 0, 0, time.Local)}},
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 2, ChatID: 1, SenderName: "Bob", Text: "first deploy", Date: time.Date(2025, 12, 30, 9, 0, 0, 0, time.Local)}},
	)

	lines := searchLines(m)
	require.GreaterOrEqual(t, len(lines), 12)
	assert.Contains(t, lines[3], "Bob")
	assert.True(t, strings.HasSuffix(lines[3], "March 12 │"), "the date sits at the right edge: %q", lines[3])
	assert.Contains(t, lines[4], "so the deploy failed")
	assert.Equal(t, "│"+strings.Repeat(" ", 78)+"│", lines[5], "a blank line between results")
	assert.Contains(t, lines[6], "You")
	assert.True(t, strings.HasSuffix(lines[6], "09:41 │"), "today is a time: %q", lines[6])
	assert.Contains(t, lines[7], "deploy is green now")
	assert.True(t, strings.HasSuffix(lines[9], "December 30, 2025 │"), "another year names it: %q", lines[9])
	assert.Contains(t, lines[10], "first deploy")
	assert.True(t, strings.HasPrefix(lines[11], "╰"), "no blank line after the last result")
}

// Searching every chat, a result names where it was found: the chat, its topic
// when it has one the owner knows, then the sender; a private chat is named
// alone, or with You on your own messages, and a line too long for the window
// is cut before the date (#349).
func TestMessageSearch_EverywhereAResultNamesItsChat(t *testing.T) {
	m := screens.NewMessageSearchModel(screens.MessageSearchOpen{}, 100, 60, keys.DefaultKeyMap()).WithNow(func() time.Time { return searchNow })
	day := time.Date(2026, 3, 3, 10, 0, 0, 0, time.Local)

	m = answered(t, m, "deploy",
		project.FoundMessage{Chat: "Go Club", Topic: "General", Message: domain.Message{ID: 9, ChatID: 20, SenderName: "Carol", Text: "deploy", Date: day}},
		project.FoundMessage{Chat: "Go Club", Message: domain.Message{ID: 8, ChatID: 20, TopicID: 30, SenderName: "Carol", Text: "deploy", Date: day}},
		project.FoundMessage{Chat: "Alice", Private: true, Message: domain.Message{ID: 7, ChatID: 1, SenderName: "Alice", Text: "deploy", Date: day}},
		project.FoundMessage{Chat: "Alice", Private: true, Message: domain.Message{ID: 6, ChatID: 1, IsOut: true, Text: "deploy", Date: day}},
		project.FoundMessage{Chat: strings.Repeat("Infrastructure ", 6), Topic: "Releases", Message: domain.Message{ID: 5, ChatID: 30, SenderName: "Dmitri", Text: "deploy", Date: day}},
	)

	lines := searchLines(m)
	require.GreaterOrEqual(t, len(lines), 16)
	assert.Contains(t, lines[0], "Messages in all chats")
	assert.Contains(t, lines[3], "Go Club › General · Carol")
	assert.Contains(t, lines[6], "Go Club · Carol", "a topic the owner does not know is left out")
	assert.Regexp(t, `│ Alice +March 3 │`, lines[9], "a private chat is named alone")
	assert.Contains(t, lines[12], "Alice · You")
	assert.Contains(t, lines[15], "…")
	assert.True(t, strings.HasSuffix(lines[15], "… March 3 │"), "cut before the date: %q", lines[15])
}

// A match past the width of the window is brought into view, the text cut
// with … where it goes on; case is ignored, and media with no text is named
// (#349).
func TestMessageSearch_TheSnippetShowsTheMatch(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap()).WithNow(func() time.Time { return searchNow })
	long := strings.Repeat("words before it ", 10) + "the Deploy failed again " + strings.Repeat("and words after it ", 10)

	m = answered(t, m, "deploy",
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 9, ChatID: 1, SenderName: "Bob", Text: long, Date: searchNow}},
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 8, ChatID: 1, SenderName: "Bob", Media: &domain.MediaRef{Kind: domain.MediaVoice, Duration: 12}, Date: searchNow}},
	)

	lines := searchLines(m)
	require.GreaterOrEqual(t, len(lines), 8)
	assert.Contains(t, lines[4], "the Deploy failed")
	assert.True(t, strings.HasPrefix(lines[4], "│ …"), "cut at the start: %q", lines[4])
	assert.True(t, strings.HasSuffix(lines[4], "… │"), "cut at the end: %q", lines[4])
	assert.Contains(t, lines[7], "Voice 0:12")
}

// A question that fails leaves the window as it was before it was asked: the
// failure itself is a toast, shown by whoever asked (#349).
func TestMessageSearch_AFailureLeavesWhatWasThere(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap()).WithNow(func() time.Time { return searchNow })
	m = answered(t, m, "deploy",
		project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 9, ChatID: 1, SenderName: "Bob", Text: "deploy", Date: searchNow}},
	)
	m = typeQuery(t, m, "s")
	m, msg := enter(t, m)
	req := msg.(screens.MessageSearchRequest)

	m, _ = m.Update(screens.MessageSearchResult{Serial: req.Serial, Err: errors.New("flood wait")})

	lines := searchLines(m)
	assert.Contains(t, lines[3], "Bob", "the results before are back")
	assert.Contains(t, lines[4], "deploy")

	fresh := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap())
	fresh = typeQuery(t, fresh, "deploy")
	fresh, msg = enter(t, fresh)
	fresh, _ = fresh.Update(screens.MessageSearchResult{Serial: msg.(screens.MessageSearchRequest).Serial, Err: errors.New("flood wait")})
	assert.Contains(t, searchLines(fresh)[3], "Press ⏎ to search Alice")
}

// Opened, the window names where it looks in its title, and before anything is
// asked it says how to ask; the bottom border names the keys (#349).
func TestMessageSearch_OpensAskingForAQuery(t *testing.T) {
	m := screens.NewMessageSearchModel(aliceHere, 100, 40, keys.DefaultKeyMap())

	lines := searchLines(m)

	require.GreaterOrEqual(t, len(lines), 5)
	assert.Contains(t, lines[0], "Messages in Alice")
	assert.Contains(t, lines[1], "> ")
	assert.Contains(t, lines[3], "Press ⏎ to search Alice")
	last := lines[len(lines)-1]
	assert.Contains(t, last, "search")
	assert.Contains(t, last, "all chats")
	assert.Contains(t, last, "close")
}
