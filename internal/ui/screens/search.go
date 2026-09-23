package screens

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

type CloseSearchMsg struct{}

const (
	searchMinChars    = 2
	searchDebounce    = 300 * time.Millisecond
	searchSpinnerRate = 120 * time.Millisecond
	searchGlobalLimit = 10
)

// SearchUsersRequest asks RootModel to run a server-side user search (#82).
type SearchUsersRequest struct {
	Query  string
	Serial int
}

// SearchUsersResult carries server-side search results back to the model (#82).
type SearchUsersResult struct {
	Query  string
	Serial int
	Chats  []project.ChatRow
	Err    error
}

type searchDebounceMsg struct{ Serial int }
type searchSpinnerTickMsg struct{ Serial int }

// IsSearchInternalMsg reports whether msg is an internal SearchModel tick that
// must be routed back into Update (debounce / spinner). RootModel uses this to
// forward unexported messages without naming their types.
func IsSearchInternalMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case searchDebounceMsg, searchSpinnerTickMsg:
		return true
	}
	return false
}

const searchOverlayWidth = 50
const searchMaxResults = 8

type forwardPhase int

const (
	forwardSelect forwardPhase = iota
	forwardComment
)

type SearchModel struct {
	query   string
	all     []project.ChatRow
	results []project.ChatRow
	// chatsLoaded is false while the chat list is still on its way from the
	// owner, so an empty list reads as waiting rather than as "no results".
	chatsLoaded bool
	list        *components.ListView
	width       int
	height      int
	keyMap      keys.KeyMap
	// forwardMsgID > 0 puts the picker in forward mode: confirming a chat emits
	// ForwardToChatRequest{ToChatID, MsgID} and rows show the unread count.
	forwardMsgID int
	phase        forwardPhase // forward mode only: select (default) | comment
	comment      string
	target       project.ChatRow // chat chosen when entering the comment phase
	// globalResults holds users found via server-side contacts.search (#82),
	// shown in the "New contacts" section. serial guards debounce/RPC staleness.
	globalResults []project.ChatRow
	globalLoading bool
	serial        int
	spinner       components.Spinner
}

// NewSearchModel builds the search overlay over chats. A nil list means it has
// not arrived yet: the overlay opens at once, takes typing, and is filled by
// SetChats when the owner answers (#278).
func NewSearchModel(chats []project.ChatRow, width, height int, km keys.KeyMap) *SearchModel {
	m := &SearchModel{width: width, height: height, keyMap: km, list: components.NewListView(false), spinner: components.NewSpinner()}
	if chats != nil {
		m.SetChats(chats)
	}
	m.syncCount()
	return m
}

// NewForwardPicker builds the chat picker in forward mode: confirming a chat
// emits ForwardToChatRequest{ToChatID, MsgID} and rows show the unread count.
func NewForwardPicker(chats []project.ChatRow, msgID int, width, height int, km keys.KeyMap) *SearchModel {
	m := NewSearchModel(chats, width, height, km)
	m.forwardMsgID = msgID
	return m
}

// SetChats installs the chat list the overlay searches, keeping whatever was
// typed while it was on its way.
func (m *SearchModel) SetChats(chats []project.ChatRow) {
	m.all = chats
	m.chatsLoaded = true
	// A global search that answered first may hold someone the list now shows.
	m.globalResults = dedupGlobal(m.globalResults, m.all)
	m.filter()
}

func (m *SearchModel) hint() string {
	if m.keyMap == nil {
		return ""
	}
	cancel := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionCancel)
	confirm := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionConfirm)
	down := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionDown)
	up := m.keyMap.KeyFor(keys.ContextSearch, keys.ActionUp)

	if m.forwardMsgID != 0 && m.phase == forwardComment {
		return components.OverlayHint([][2]string{{confirm, "send"}, {cancel, "back"}}, nil)
	}

	navKey := arrowSym(up) + "/" + arrowSym(down)
	pairs := [][2]string{{navKey, components.DescribeShort(keys.ContextSearch, keys.ActionDown)}}
	verb := components.DescribeShort(keys.ContextSearch, keys.ActionConfirm)
	if m.forwardMsgID != 0 {
		verb = "forward" // forward-mode wording is state-specific
	}
	pairs = append(pairs, [2]string{confirm, verb})
	if m.forwardMsgID != 0 {
		pairs = append(pairs, [2]string{"tab", "comment"})
	}
	pairs = append(pairs, [2]string{cancel, components.DescribeShort(keys.ContextSearch, keys.ActionCancel)})
	return components.OverlayHint(pairs, nil)
}

func arrowSym(key string) string {
	switch key {
	case "up":
		return "↑"
	case "down":
		return "↓"
	default:
		return key
	}
}

func (m *SearchModel) Cursor() int                      { return m.list.Cursor() }
func (m *SearchModel) Query() string                    { return m.query }
func (m *SearchModel) Results() []project.ChatRow       { return m.results }
func (m *SearchModel) GlobalResults() []project.ChatRow { return m.globalResults }
func (m *SearchModel) GlobalLoading() bool              { return m.globalLoading }

// searchRow is one rendered line: either a non-selectable section header or a
// selectable chat row. The list (incl. headers) is what the ListView windows
// and scrolls; headers are skipped by cursor movement.
type searchRow struct {
	header string
	chat   project.ChatRow
	isChat bool
}

// showSections reports whether the two-section layout (with headers) is active:
// only when there is a global block (results or loading) and not in forward mode.
func (m *SearchModel) showSections() bool {
	return m.forwardMsgID == 0 && (len(m.globalResults) > 0 || m.globalLoading)
}

// rowModel builds the full flat row list: optional "Chats" header, existing
// filtered chats, optional "New contacts" header (with spinner while loading),
// then new-contact results.
func (m *SearchModel) rowModel() []searchRow {
	sections := m.showSections()
	rows := make([]searchRow, 0, len(m.results)+len(m.globalResults)+2)
	if sections && len(m.results) > 0 {
		rows = append(rows, searchRow{header: "Chats"})
	}
	for _, c := range m.results {
		rows = append(rows, searchRow{chat: c, isChat: true})
	}
	if sections {
		header := "New contacts"
		if m.globalLoading {
			header += " " + m.spinner.View()
		}
		rows = append(rows, searchRow{header: header})
	}
	for _, c := range m.globalResults {
		rows = append(rows, searchRow{chat: c, isChat: true})
	}
	return rows
}

func (m *SearchModel) selectableAt(cursor int) (project.ChatRow, bool) {
	rows := m.rowModel()
	if cursor < 0 || cursor >= len(rows) || !rows[cursor].isChat {
		return project.ChatRow{}, false
	}
	return rows[cursor].chat, true
}

// syncCount registers the flat row count and marks header rows non-selectable so
// the ListView clamps/moves the cursor over chat rows only.
func (m *SearchModel) syncCount() {
	rows := m.rowModel()
	m.list.SetSelectable(func(i int) bool {
		return i >= 0 && i < len(rows) && rows[i].isChat
	})
	m.list.SetCount(len(rows))
}

func (m *SearchModel) spinnerTick(serial int) tea.Cmd {
	return tea.Tick(searchSpinnerRate, func(time.Time) tea.Msg {
		return searchSpinnerTickMsg{Serial: serial}
	})
}

// onQueryChanged re-runs the local filter and (re)schedules a debounced global
// search. Returns the command to run (debounce tick or nil). Forward mode never
// searches globally.
func (m *SearchModel) onQueryChanged() tea.Cmd {
	m.filter()
	m.serial++
	if m.forwardMsgID != 0 || len([]rune(m.query)) < searchMinChars {
		m.globalResults = nil
		m.globalLoading = false
		m.syncCount()
		return nil
	}
	serial := m.serial
	return tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchDebounceMsg{Serial: serial}
	})
}

// dedupGlobal drops global results whose chat already appears in the local chat
// list, so a user with an existing chat stays only in the top section.
func dedupGlobal(global []project.ChatRow, existing ...[]project.ChatRow) []project.ChatRow {
	seen := make(map[int64]struct{})
	for _, set := range existing {
		for _, c := range set {
			seen[c.ID] = struct{}{}
		}
	}
	out := make([]project.ChatRow, 0, len(global))
	for _, c := range global {
		if _, dup := seen[c.ID]; dup {
			continue
		}
		seen[c.ID] = struct{}{}
		out = append(out, c)
	}
	return out
}

func (m *SearchModel) Update(msg tea.Msg) (*SearchModel, tea.Cmd) {
	if m.forwardMsgID != 0 && m.phase == forwardComment {
		return m.updateComment(msg)
	}
	switch tm := msg.(type) {
	case searchDebounceMsg:
		if tm.Serial != m.serial || m.forwardMsgID != 0 || len([]rune(m.query)) < searchMinChars {
			return m, nil
		}
		m.globalLoading = true
		m.syncCount() // the "New contacts" header now counts as a row
		serial := m.serial
		query := m.query
		return m, tea.Batch(
			func() tea.Msg { return SearchUsersRequest{Query: query, Serial: serial} },
			m.spinnerTick(serial),
		)
	case searchSpinnerTickMsg:
		if tm.Serial != m.serial || !m.globalLoading {
			return m, nil
		}
		m.spinner.Tick()
		return m, m.spinnerTick(tm.Serial)
	case SearchUsersResult:
		if tm.Serial != m.serial {
			return m, nil
		}
		m.globalLoading = false
		m.globalResults = dedupGlobal(tm.Chats, m.results, m.all)
		m.syncCount()
		return m, nil
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.query += paste.Content
		return m, m.onQueryChanged()
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch km.Code {
	case tea.KeyTab:
		if m.forwardMsgID != 0 {
			if chat, ok := m.selectableAt(m.list.Cursor()); ok {
				m.target = chat
				m.comment = ""
				m.phase = forwardComment
			}
		}
		return m, nil
	case tea.KeyEsc:
		return m, func() tea.Msg { return CloseSearchMsg{} }
	case tea.KeyEnter:
		chat, ok := m.selectableAt(m.list.Cursor())
		if !ok {
			return m, nil
		}
		if m.forwardMsgID != 0 {
			msgID := m.forwardMsgID
			to, title := chat.ID, chat.Title
			return m, func() tea.Msg {
				return ForwardToChatRequest{ToChatID: to, Title: title, MsgID: msgID}
			}
		}
		// A search hit may be a contact with no dialog. It opens by id like any
		// chat: the owner kept the address search returned (#278).
		chatID, title := chat.ID, chat.Title
		return m, func() tea.Msg { return OpenChatMsg{ChatID: chatID, Title: title} }
	case tea.KeyBackspace:
		if len(m.query) > 0 {
			runes := []rune(m.query)
			m.query = string(runes[:len(runes)-1])
			return m, m.onQueryChanged()
		}
		return m, nil
	case tea.KeyDown:
		m.list.MoveDown()
		return m, nil
	case tea.KeyUp:
		m.list.MoveUp()
		return m, nil
	default:
		// NormalizeKey remaps non-Latin layouts (e.g. Russian ctrl+о → ctrl+j) so
		// list navigation works regardless of keyboard layout.
		switch keys.NormalizeKey(km.String()) {
		case "ctrl+j":
			m.list.MoveDown()
		case "ctrl+k":
			m.list.MoveUp()
		default:
			if km.Text != "" {
				m.query += km.Text
				return m, m.onQueryChanged()
			}
		}
		return m, nil
	}
}

// updateComment handles the forward-mode comment phase: typed text (any layout,
// via km.Text) builds the comment, Enter emits the forward request with it, and
// Esc returns to the select phase.
func (m *SearchModel) updateComment(msg tea.Msg) (*SearchModel, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		m.comment += paste.Content
		return m, nil
	}
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch km.Code {
	case tea.KeyEsc:
		m.phase = forwardSelect
		return m, nil
	case tea.KeyEnter:
		comment := m.comment
		to, title := m.target.ID, m.target.Title
		msgID := m.forwardMsgID
		return m, func() tea.Msg {
			return ForwardToChatRequest{ToChatID: to, Title: title, MsgID: msgID, Comment: comment}
		}
	case tea.KeyBackspace:
		if len(m.comment) > 0 {
			r := []rune(m.comment)
			m.comment = string(r[:len(r)-1])
		}
		return m, nil
	default:
		if km.Text != "" {
			m.comment += km.Text
		}
		return m, nil
	}
}

func (m *SearchModel) filter() {
	q := strings.ToLower(m.query)
	filtered := make([]project.ChatRow, 0, len(m.all))
	for _, c := range m.all {
		if q == "" || strings.Contains(strings.ToLower(c.Title), q) {
			filtered = append(filtered, c)
		}
	}
	m.results = filtered
	m.syncCount()
}

const searchCaret = "█"

// inputLine renders the "> text█" prompt line clamped to inner cells, so it can
// never push RenderBox's right border out (RenderBox pads short lines but does
// not truncate long ones). Neither the query nor the comment supports left/right
// cursor movement, so the caret always sits at the end of the text: overflow
// windows to the tail, keeping the newest text visible with a leading "…" for the
// hidden head. Adding caret movement would require a real scroll offset here.
func inputLine(text string, inner int) string {
	const prompt = "> "
	budget := inner - runewidth.StringWidth(prompt) - runewidth.StringWidth(searchCaret)
	if budget < 1 {
		// Too narrow for any text at all: keep the frame intact regardless.
		return runewidth.Truncate(prompt+searchCaret, max(inner, 0), "")
	}
	// TruncateLeft pads the prefix when a wide grapheme straddles the cut, so the
	// result is exactly budget cells wide for CJK/emoji too.
	if w := runewidth.StringWidth(text); w > budget {
		text = runewidth.TruncateLeft(text, w-budget+1, "…")
	}
	// The query text and the caret follow a styled prompt, so each ends up its
	// own run: they take the body style rather than whatever the prompt left.
	return theme.S().SearchPrompt.Render(prompt) + theme.S().Body.Render(text) +
		theme.S().Body.Render(searchCaret)
}

func (m *SearchModel) View() string {
	w := searchOverlayWidth
	if m.width > 0 && m.width < w {
		w = m.width
	}

	hint := m.hint()
	if hintW := lipgloss.Width(" " + hint + " "); hintW+2 > w-2 {
		w = hintW + 4
	}

	inner := w - 2

	if m.forwardMsgID != 0 && m.phase == forwardComment {
		// A title has no caret to keep visible, so it cuts at the tail (unlike
		// inputLine, which windows to it).
		title := runewidth.Truncate("Comment to "+m.target.Title, inner, "…")
		promptLine := inputLine(m.comment, inner)
		lines := []string{
			theme.S().Body.Render(title),
			theme.S().Separator.Render(strings.Repeat("─", inner)),
			promptLine,
		}
		content := strings.Join(lines, "\n")
		h := len(lines) + 2
		return components.RenderBox(content, "", "", hint, "", lipgloss.RoundedBorder(), nil, w, h)
	}

	queryLine := inputLine(m.query, inner)
	divider := theme.S().Separator.Render(strings.Repeat("─", inner))

	lines := []string{queryLine, divider}

	headerStyle := theme.S().SearchHeader
	rows := m.rowModel()
	rowFn := func(i int, selected bool) string {
		r := rows[i]
		if !r.isChat {
			return headerStyle.Inline(true).Width(inner).MaxWidth(inner).Render(r.header)
		}
		row := r.chat.Title
		if m.forwardMsgID != 0 && r.chat.Unread > 0 {
			row = fmt.Sprintf("%s (%d)", r.chat.Title, r.chat.Unread)
		}
		style := theme.S().Body
		if selected {
			style = theme.S().SearchActiveRow
		}
		return style.Inline(true).Width(inner).MaxWidth(inner).Render(row)
	}

	lines = append(lines, m.list.Render(searchMaxResults, rowFn)...)
	if len(rows) == 0 && !m.globalLoading && m.chatsLoaded {
		lines = append(lines, theme.S().SearchHeader.Width(inner).Render("no results"))
	}

	content := strings.Join(lines, "\n")
	h := len(lines) + 2
	// List rows begin after the query line and divider, so the scrollbar track
	// starts at content row 2.
	sb := m.list.Scrollbar(searchMaxResults, 2)
	return components.RenderBox(content, "", "", hint, "", lipgloss.RoundedBorder(), nil, w, h, sb)
}
