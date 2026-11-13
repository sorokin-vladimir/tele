package screens

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// messageSearchMaxWidth is the widest the window gets: wide enough for a
// snippet to say something, narrow enough to read as a window over the chat.
const messageSearchMaxWidth = 80

// MessageSearchOpen is what a message search window is opened on: the chat
// (or topic) it can search, named by Title, and whether it starts by searching
// every chat. A zero Here has no chat to search, and the window searches every
// chat only.
type MessageSearchOpen struct {
	Here       project.SearchScope
	Title      string
	Everywhere bool
}

// MessageSearchModel is the window that asks Telegram which messages hold a
// text, in the open chat or in every chat, and lists what it found (#349). It
// asks nothing itself: it hands its questions to whoever opened it.
type MessageSearchModel struct {
	width, height int
	keyMap        keys.KeyMap

	open       MessageSearchOpen
	everywhere bool
	query      string

	// serial names the question in flight, so an answer to an older one is
	// told apart and dropped.
	serial    int
	searching bool
	// sent is the last query asked, and shown the one the results on screen
	// answer: they differ while a question is in flight.
	sent, shown string

	answered bool
	results  []project.FoundMessage
	list     *components.ListView
	now      func() time.Time
}

// MessageSearchResult is the answer to the request Serial names: a page of
// what was found, or why nothing was.
type MessageSearchResult struct {
	Serial int
	Page   project.SearchPage
	Err    error
}

// MessageSearchRequest is a question the window asks: which messages in Scope
// hold Text, from After down. Serial names it in the answer.
type MessageSearchRequest struct {
	Scope  project.SearchScope
	Text   string
	After  project.SearchPosition
	Serial int
}

// NewMessageSearchModel opens the window on open, sized for a screen of width
// by height.
func NewMessageSearchModel(open MessageSearchOpen, width, height int, km keys.KeyMap) *MessageSearchModel {
	return &MessageSearchModel{
		width: width, height: height, keyMap: km,
		open: open, everywhere: open.Everywhere || open.Here.ChatID == 0,
		list: components.NewListView(false), now: time.Now,
	}
}

// WithNow sets the clock dates are told against: today's messages show their
// time, older ones their date.
func (m *MessageSearchModel) WithNow(now func() time.Time) *MessageSearchModel {
	m.now = now
	return m
}

// scope is where the window looks, as the owner is asked.
func (m *MessageSearchModel) scope() project.SearchScope {
	if m.everywhere {
		return project.SearchScope{}
	}
	return m.open.Here
}

// Update handles a key. The window's keys follow the search context of the
// keymap; any other key with text types into the query.
func (m *MessageSearchModel) Update(msg tea.Msg) (*MessageSearchModel, tea.Cmd) {
	switch msg := msg.(type) {
	case MessageSearchResult:
		m.take(msg)
		return m, nil
	case tea.PasteMsg:
		m.query += msg.Content
		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg)
	}
	return m, nil
}

func (m *MessageSearchModel) press(km tea.KeyPressMsg) (*MessageSearchModel, tea.Cmd) {
	if m.keyMap.Resolve(keys.ContextSearch, keys.NormalizeKey(km.String())) == keys.ActionConfirm {
		return m, m.ask()
	}
	switch {
	case km.Code == tea.KeyBackspace:
		if runes := []rune(m.query); len(runes) > 0 {
			m.query = string(runes[:len(runes)-1])
		}
	case km.Text != "":
		m.query += km.Text
	}
	return m, nil
}

// ask sends the query from the first page. An empty query asks nothing.
func (m *MessageSearchModel) ask() tea.Cmd {
	text := strings.TrimSpace(m.query)
	if text == "" {
		return nil
	}
	m.serial++
	m.searching = true
	m.sent = text
	req := MessageSearchRequest{Scope: m.scope(), Text: text, Serial: m.serial}
	return func() tea.Msg { return req }
}

// take shows the answer to the question in flight; an answer to an older one
// is dropped.
func (m *MessageSearchModel) take(res MessageSearchResult) {
	if res.Serial != m.serial || !m.searching {
		return
	}
	m.searching = false
	if res.Err != nil {
		// What was shown before stays, marked for the query it answered.
		m.sent = m.shown
		return
	}
	m.shown = m.sent
	m.answered = true
	m.results = res.Page.Found
	m.list.SetCount(len(m.results))
	m.list.SetCursor(0)
}

// scopeName is where the window looks, as a sentence ends with it.
func (m *MessageSearchModel) scopeName() string {
	if m.everywhere {
		return "all chats"
	}
	return m.open.Title
}

// canToggle reports whether the window has a chat to search besides every chat.
func (m *MessageSearchModel) canToggle() bool { return m.open.Here.ChatID != 0 }

func (m *MessageSearchModel) hint() string {
	if m.keyMap == nil {
		return ""
	}
	key := func(a keys.Action) string { return m.keyMap.KeyFor(keys.ContextSearch, a) }
	pairs := [][2]string{{key(keys.ActionConfirm), "search"}}
	if m.canToggle() {
		other := "all chats"
		if m.everywhere {
			other = "this chat"
		}
		pairs = append(pairs, [2]string{key(keys.ActionToggleScope), other})
	}
	pairs = append(pairs, [2]string{key(keys.ActionCancel), "close"})
	return components.OverlayHint(pairs, nil)
}

// boxWidth is the window's outer width on the current screen.
func (m *MessageSearchModel) boxWidth() int {
	w := messageSearchMaxWidth
	if m.width > 0 && m.width-4 < w {
		w = max(m.width-4, 20)
	}
	return w
}

// listRows is how many rows the list may take: the window grows with its
// results up to about two thirds of the screen.
func (m *MessageSearchModel) listRows() int {
	rows := m.height*2/3 - 4 // borders, the query and the line under it
	return max(rows, 2)
}

// resultRows is the rows one result takes: two lines and the blank line
// before the next.
const resultRows = 3

func (m *MessageSearchModel) View() string {
	w := m.boxWidth()
	inner := w - 2
	lines := []string{
		theme.Pad(1) + inputLine(m.query, inner-2) + theme.Pad(1),
		theme.S().Separator.Render(strings.Repeat("─", inner)),
	}
	var sb *components.Scrollbar
	if state := m.stateLine(); state != "" {
		lines = append(lines, theme.Pad(1)+theme.S().SearchHeader.Width(inner-2).Render(state)+theme.Pad(1))
	} else {
		// A result is two lines and the blank before the next, the last has
		// none: a window of n results is 3n-1 rows.
		visible := (m.listRows() + 1) / resultRows
		rows := m.list.Render(visible, func(i int, selected bool) string {
			return m.renderResult(m.results[i], inner, selected)
		})
		lines = append(lines, strings.Split(strings.Join(rows, "\n"+theme.Pad(inner)+"\n"), "\n")...)
		info := m.list.ScrollInfo(visible)
		sb = &components.Scrollbar{
			Info:     components.ScrollInfo{Total: info.Total*resultRows - 1, Visible: info.Visible*resultRows - 1, Offset: info.Offset * resultRows},
			TrackTop: 2,
			TrackLen: info.Visible*resultRows - 1,
		}
	}
	content := strings.Join(lines, "\n")
	return components.RenderBox(content, "Messages in "+m.scopeName(), "", m.hint(), "", lipgloss.RoundedBorder(), nil, w, len(lines)+2, sb)
}

// stateLine is the one line shown in place of the list, empty when there is a
// list to show.
func (m *MessageSearchModel) stateLine() string {
	switch {
	case m.searching:
		return "Searching…"
	case !m.answered:
		return "Press ⏎ to search " + m.scopeName()
	case len(m.results) == 0:
		return "Nothing found in " + m.scopeName()
	}
	return ""
}
