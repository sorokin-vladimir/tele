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
	// selected is the query as a reopened window shows it: the next key
	// replaces it rather than adding to it.
	selected bool

	// serial names the question in flight, so an answer to an older one is
	// told apart and dropped.
	serial    int
	searching bool
	// sent is the last query asked, and shown the one the results on screen
	// answer: they differ while a question is in flight.
	sent, shown string
	// shownEverywhere is the scope the results on screen were found in.
	shownEverywhere bool

	answered bool
	results  []project.FoundMessage
	// next is where the page after those shown starts, nil when Telegram has
	// no more; paging is a question for it in flight.
	next    *project.SearchPosition
	paging  bool
	spinner components.Spinner
	list    *components.ListView
	now     func() time.Time
}

// MessageSearchClose asks for the window to be closed, and a question in
// flight to be cancelled.
type MessageSearchClose struct{}

// MessageSearchJump asks for the chat a result was found in to be opened at
// the message found.
type MessageSearchJump struct {
	Found project.FoundMessage
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
	everywhere := open.Everywhere || open.Here.ChatID == 0
	return &MessageSearchModel{
		width: width, height: height, keyMap: km,
		open: open, everywhere: everywhere, shownEverywhere: everywhere,
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
		if m.selected {
			m.query, m.selected = "", false
		}
		m.query += msg.Content
		return m, nil
	case components.SpinnerTickMsg:
		if m.searching || m.paging {
			m.spinner.Tick()
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.press(msg)
	}
	return m, nil
}

func (m *MessageSearchModel) press(km tea.KeyPressMsg) (*MessageSearchModel, tea.Cmd) {
	//exhaustive:ignore
	switch m.keyMap.Resolve(keys.ContextSearch, keys.NormalizeKey(km.String())) {
	case keys.ActionConfirm:
		return m, m.confirm()
	case keys.ActionDown:
		if m.searching {
			return m, nil
		}
		m.list.MoveDown()
		return m, m.askMore()
	case keys.ActionUp:
		if !m.searching {
			m.list.MoveUp()
		}
		return m, nil
	case keys.ActionToggleScope:
		return m, m.toggle()
	case keys.ActionCancel:
		return m, func() tea.Msg { return MessageSearchClose{} }
	}
	if m.selected {
		switch {
		case km.Code == tea.KeyBackspace:
			m.query, m.selected = "", false
		case km.Code == tea.KeyRight || km.Code == tea.KeyEnd:
			m.selected = false
		case km.Text != "":
			m.query, m.selected = km.Text, false
		}
		return m, nil
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

// Reopen readies a window opened again on what it held: the query is shown
// selected, so typing replaces it, and the results and the cursor are as they
// were.
func (m *MessageSearchModel) Reopen() *MessageSearchModel {
	m.selected = m.query != ""
	return m
}

// opens reports whether Enter opens a result rather than asking: the query is
// the one the results on screen answer, and there are results.
func (m *MessageSearchModel) opens() bool {
	return !m.searching && len(m.results) > 0 && strings.TrimSpace(m.query) == m.shown
}

// confirm is Enter: it opens the result under the cursor when the query is the
// one its results answer, and asks the query otherwise. While a question is in
// flight it does nothing.
func (m *MessageSearchModel) confirm() tea.Cmd {
	if m.searching {
		return nil
	}
	if m.opens() {
		jump := MessageSearchJump{Found: m.results[m.list.Cursor()]}
		return func() tea.Msg { return jump }
	}
	return m.ask()
}

// toggle switches between the open chat and every chat. A query already asked
// is asked again in the other scope at once.
func (m *MessageSearchModel) toggle() tea.Cmd {
	if !m.canToggle() {
		return nil
	}
	m.everywhere = !m.everywhere
	asked := m.shown
	if m.searching {
		asked = m.sent
	}
	if asked == "" {
		// Nothing on screen was found anywhere: the switch is the scope now.
		m.shownEverywhere = m.everywhere
		return nil
	}
	return m.askText(asked)
}

// askMore asks for the next page once the cursor reaches the second-to-last
// result, a little before the end; one page at a time, and none past the last.
func (m *MessageSearchModel) askMore() tea.Cmd {
	if m.paging || m.next == nil || m.list.Cursor() < len(m.results)-2 {
		return nil
	}
	m.serial++
	m.paging = true
	scope := project.SearchScope{}
	if !m.shownEverywhere {
		scope = m.open.Here
	}
	req := MessageSearchRequest{Scope: scope, Text: m.shown, After: *m.next, Serial: m.serial}
	return func() tea.Msg { return req }
}

// ask sends the query from the first page. An empty query asks nothing.
func (m *MessageSearchModel) ask() tea.Cmd {
	return m.askText(strings.TrimSpace(m.query))
}

func (m *MessageSearchModel) askText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	m.serial++
	m.searching = true
	m.paging = false
	m.sent = text
	req := MessageSearchRequest{Scope: m.scope(), Text: text, Serial: m.serial}
	return func() tea.Msg { return req }
}

// take shows the answer to the question in flight; an answer to an older one
// is dropped.
func (m *MessageSearchModel) take(res MessageSearchResult) {
	if res.Serial != m.serial {
		return
	}
	if m.paging {
		m.paging = false
		if res.Err == nil {
			// A failed page keeps its position: moving again asks for it again.
			m.results = append(m.results, res.Page.Found...)
			m.next = res.Page.Next
			m.list.SetCount(len(m.results))
		}
		return
	}
	if !m.searching {
		return
	}
	m.searching = false
	if res.Err != nil {
		// What was shown before stays, marked for the query it answered and
		// titled with the scope it was found in.
		m.sent, m.everywhere = m.shown, m.shownEverywhere
		return
	}
	m.shown, m.shownEverywhere = m.sent, m.everywhere
	m.answered = true
	m.results = res.Page.Found
	m.next = res.Page.Next
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
	verb := "search"
	if m.opens() {
		verb = "open"
	}
	pairs := [][2]string{{key(keys.ActionConfirm), verb}}
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
	query := inputLine(m.query, inner-2)
	if m.selected {
		query = selectedInputLine(m.query, inner-2)
	}
	lines := []string{
		theme.Pad(1) + query + theme.Pad(1),
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
		if m.paging {
			more := m.spinner.View() + " Loading more"
			lines = append(lines, theme.Pad(inner), theme.Pad(1)+theme.S().SearchHeader.Width(inner-2).Render(more)+theme.Pad(1))
		}
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
		return m.spinner.View() + " Searching…"
	case !m.answered:
		return "Press ⏎ to search " + m.scopeName()
	case len(m.results) == 0:
		return "Nothing found in " + m.scopeName()
	}
	return ""
}
