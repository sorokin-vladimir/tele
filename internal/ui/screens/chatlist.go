package screens

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// OpenChatMsg asks the root model to open a chat. It carries an id and a title
// rather than a domain.Chat: a client holds no peer, and the chat's header
// arrives on the chat:<id> projection once the subscription lands. The title is
// here only so the pane has something to draw during that gap.
type OpenChatMsg struct {
	ChatID int64
	Title  string
	// Peer addresses outgoing commands for a chat the owner does not hold: a
	// contact found by search that has never been messaged has no dialog and no
	// history, so nothing but a send can address it. Zero when the owner knows
	// the chat, which is every chat opened from the list.
	//
	// TRANSITIONAL (#198): when commands become owner API members addressed by
	// chat id, this goes.
	Peer domain.Peer
}

// ForwardToChatRequest is emitted by the forward-mode chat picker when the user
// confirms a target chat. Both ends are chat ids; addressing them is the
// owner's business, including a search hit with no dialog (#278).
type ForwardToChatRequest struct {
	ToChatID int64
	// Title names the target in the result status. The picker had the chat in
	// hand when the user chose it, so it travels along rather than being looked
	// up again — a search hit may not be a chat the owner holds at all.
	Title   string
	MsgID   int
	Comment string // optional; sent as a separate message before the forward
}

// The row styles used to live here as package-level vars carrying no colour.
// They come from the theme now: a row is body text, and with a canvas set it has
// a background to carry, which a value built once at init could not know about.

func formatUnread(count int) string {
	if count <= 0 {
		return ""
	}
	if count > 99 {
		return "[99+]"
	}
	return fmt.Sprintf("[%d]", count)
}

// formatReactions renders the unread-reaction token: empty when none, a bare
// heart for one, or a heart with the count for many.
func formatReactions(count int) string {
	switch {
	case count <= 0:
		return ""
	case count == 1:
		return "♥"
	default:
		return fmt.Sprintf("♥%d", count)
	}
}

// formatMentions renders the unread-mention token: empty when none, a bare
// at-sign for one, or an at-sign with the count for many.
func formatMentions(count int) string {
	switch {
	case count <= 0:
		return ""
	case count == 1:
		return "@"
	default:
		return fmt.Sprintf("@%d", count)
	}
}

// rowIndicators builds the right-aligned status column for a chat row. Tokens
// appear in order [mute] [reaction] [mention] [unread], each separated by a
// single space and omitted when empty: the dim mute marker, the pink
// unread-reaction glyph, the blue unread-mention glyph, then the unread token
// (numeric badge, or a manual-unread dot when marked unread with no real count).
// padRow renders n spaces through the row's own style, so the gap carries
// whatever fills that row — the canvas, or the selection highlight.
func padRow(base lipgloss.Style, n int) string {
	if n <= 0 {
		return ""
	}
	// canvas:ok base always carries a background, and these spaces go through it.
	return base.Render(strings.Repeat(" ", n))
}

// base is the row's own style, and every indicator is built on top of it rather
// than from the theme directly. Each indicator ends in a reset, so what follows
// it inherits nothing: taking only the foreground from the token and the fill
// from base is what keeps a selected row's highlight solid across them.
func rowIndicators(c project.ChatRow, base lipgloss.Style) string {
	var unread string
	switch {
	case c.Unread > 0:
		unread = base.Render(formatUnread(c.Unread))
	case c.UnreadMark:
		unread = base.Render("[•]")
	}
	var reaction string
	if c.Reactions > 0 {
		reaction = base.Foreground(theme.T().UnreadReaction).Render(formatReactions(c.Reactions))
	}
	var mention string
	if c.Mentions > 0 {
		mention = base.Foreground(theme.T().UnreadMention).Render(formatMentions(c.Mentions))
	}
	var muted string
	if c.Muted {
		muted = base.Foreground(theme.T().TextMuted).Render("×")
	}
	parts := make([]string, 0, 4)
	for _, p := range []string{muted, reaction, mention, unread} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, base.Render(" "))
}

// ChatListModel renders a window onto the chat list rather than the whole list:
// the core owns order and filtering, and hands over a slice around what is on
// screen. cursor indexes the whole list, so it stays meaningful when the window
// moves under it.
type ChatListModel struct {
	rows   []project.ChatRow
	offset int // index of rows[0] in the whole filtered list
	total  int // length of the whole filtered list
	cursor int // index into the whole list, not into rows
	// reqOffset/reqLimit are the last window asked for, so a repeated request
	// for the same window is silent. Zero limit means nothing asked for yet.
	reqOffset int
	reqLimit  int
	// activeID is the open chat, held by id rather than index so it survives a
	// window that no longer contains it.
	activeID int64
	width    int
	height   int
	focused  bool
	spinner  components.Spinner

	// highlightChatID is the chat currently flashed because a new incoming
	// message bumped it to the top; highlightStep counts down to 0 (none).
	// Tracked by id so the highlight survives a window reset.
	highlightChatID int64
	highlightStep   int
}

func NewChatListModel() *ChatListModel {
	return &ChatListModel{}
}

// TickSpinner advances the spinner frame. Called by root on SpinnerTickMsg.
func (m *ChatListModel) TickSpinner() { m.spinner.Tick() }

// IsLoadingChats reports whether the chat list is still showing its
// "Loading chats..." spinner (no chats received yet), matching View. Drives the
// spinner tick loop (issue #147).
func (m *ChatListModel) IsLoadingChats() bool { return m.total == 0 }

// SetWindow replaces the window with the contents of a chatlist Reset delta.
// The cursor follows the chat it was on, which is what makes a Reset — emitted
// on every reorder — non-disruptive.
//
// When the cursor sits outside the window being replaced, there is no chat to
// follow and the numeric position is kept instead. That case is not exotic: a
// jump to the end of the list moves the cursor first and only then asks for the
// window around it, so the arriving window is precisely the one the cursor is
// no longer inside.
func (m *ChatListModel) SetWindow(offset, total int, rows []project.ChatRow) {
	cursorID := int64(0)
	if r, ok := m.rowAt(m.cursor); ok {
		cursorID = r.ID
	}

	m.rows = rows
	m.offset = offset
	m.total = total

	if cursorID != 0 {
		for i, r := range rows {
			if r.ID == cursorID {
				m.cursor = offset + i
				break
			}
		}
	}
	m.clampCursor()
}

// SetRow replaces one row in place, from a chatlist Row delta. The row is found
// by id: a Row is only emitted while the window's order is unchanged, so the id
// is unambiguous and no index travels on the wire.
func (m *ChatListModel) SetRow(row project.ChatRow) {
	for i := range m.rows {
		if m.rows[i].ID == row.ID {
			m.rows[i] = row
			return
		}
	}
}

// rowAt returns the row at a whole-list index, translating through the window.
// ok is false when the index falls outside the window the core has sent.
func (m *ChatListModel) rowAt(i int) (project.ChatRow, bool) {
	j := i - m.offset
	if j < 0 || j >= len(m.rows) {
		return project.ChatRow{}, false
	}
	return m.rows[j], true
}

// viewStart is the whole-list index of the first visible row. It is the single
// definition of the scroll position: View, ScrollInfo, CursorViewportRow and
// ChatAtViewportRow all derive from it, because four copies of this arithmetic
// is how a click lands on the wrong chat.
func (m *ChatListModel) viewStart() int {
	if m.height <= 0 {
		return 0
	}
	start := m.cursor - m.height + 1
	if start < 0 {
		start = 0
	}
	if max := m.total - m.height; start > max {
		if max < 0 {
			max = 0
		}
		start = max
	}
	return start
}

func (m *ChatListModel) clampCursor() {
	if m.total == 0 {
		m.cursor = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.total {
		m.cursor = m.total - 1
	}
}

// WindowRequest reports the window the model wants for its current scroll
// position: the visible rows plus one screen of overscan either side, so
// ordinary scrolling never waits on a round trip. changed is false when this is
// the window it last asked for, which keeps a quiet list quiet.
//
// It compares against the last request rather than against the window it holds,
// because at the end of the list the core legitimately returns fewer rows than
// asked for and a held-window comparison would ask again forever.
func (m *ChatListModel) WindowRequest() (offset, limit int, changed bool) {
	h := m.height
	if h <= 0 {
		h = 1
	}
	offset = m.viewStart() - h
	if offset < 0 {
		offset = 0
	}
	limit = 3 * h
	if offset == m.reqOffset && limit == m.reqLimit {
		return offset, limit, false
	}
	m.reqOffset, m.reqLimit = offset, limit
	return offset, limit, true
}

// HighlightChat starts a fade highlight on the chat-list row for the given id.
func (m *ChatListModel) HighlightChat(id int64) {
	m.highlightChatID = id
	m.highlightStep = components.HighlightInitialStep
}

// StepChatHighlight advances the chat-row highlight fade by one step. Returns
// true while still active; clears the highlight and returns false at 0. No-op
// (false) when no highlight is active.
func (m *ChatListModel) StepChatHighlight() bool {
	if m.highlightStep <= 0 {
		return false
	}
	m.highlightStep--
	if m.highlightStep <= 0 {
		m.highlightChatID = 0
		return false
	}
	return true
}

// HighlightedChatID returns the currently highlighted chat id (0 when none).
func (m *ChatListModel) HighlightedChatID() int64 { return m.highlightChatID }

// HighlightStep returns the current chat-row fade step (0 when none).
func (m *ChatListModel) HighlightStep() int { return m.highlightStep }

// styleTitle applies the fade-accent foreground to a row's (already truncated)
// title while that row is the active highlight target. The focused-cursor row
// keeps its selection background instead, so it is left unstyled here.
func (m *ChatListModel) styleTitle(i int, id int64, truncated string, base lipgloss.Style) string {
	if m.highlightStep <= 0 || id != m.highlightChatID {
		return base.Render(truncated)
	}
	if i == m.cursor && m.focused {
		// The selection fill supplies the colour; the title only takes the fill.
		return base.Render(truncated)
	}
	fg := components.FadeAccentColor(theme.T().HighlightAccent, theme.T().HighlightBaseChat, m.highlightStep, components.HighlightFadeSteps)
	return base.Foreground(fg).Render(truncated)
}

func (m *ChatListModel) Cursor() int { return m.cursor }

// Rows returns the window's rows. For assertions and for the mouse mapping;
// callers must not assume it is the whole list.
func (m *ChatListModel) Rows() []project.ChatRow { return m.rows }

// Total is the length of the whole filtered list, which the window is a slice of.
func (m *ChatListModel) Total() int { return m.total }

// ActiveIdx is the whole-list index of the open chat, or -1 when it is outside
// the window (or no chat is open).
func (m *ChatListModel) ActiveIdx() int {
	if m.activeID == 0 {
		return -1
	}
	for i, r := range m.rows {
		if r.ID == m.activeID {
			return m.offset + i
		}
	}
	return -1
}

// SetActive marks a chat as the open one. It does not touch the cursor: the
// window is replaced on every reorder, and dragging the cursor back to the open
// chat each time would pin it there for as long as a chat is open.
func (m *ChatListModel) SetActive(id int64) { m.activeID = id }

// SetActiveByID marks a chat as the open one and moves the cursor onto it. For
// the moment a chat is opened, where the cursor should follow.
func (m *ChatListModel) SetActiveByID(id int64) {
	m.activeID = id
	m.SetCursorByID(id)
}

// SelectedChat returns the open chat's row, when the window holds it.
func (m *ChatListModel) SelectedChat() (project.ChatRow, bool) {
	if idx := m.ActiveIdx(); idx >= 0 {
		return m.rowAt(idx)
	}
	return project.ChatRow{}, false
}

func (m *ChatListModel) SetCursorByID(id int64) {
	for i, r := range m.rows {
		if r.ID == id {
			m.cursor = m.offset + i
			return
		}
	}
}
func (m *ChatListModel) Context() keys.Context { return keys.ContextChatList }
func (m *ChatListModel) Focused() bool         { return m.focused }
func (m *ChatListModel) SetFocused(f bool)     { m.focused = f }

func (m *ChatListModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// CursorChat returns the row currently under the cursor.
func (m *ChatListModel) CursorChat() (project.ChatRow, bool) { return m.rowAt(m.cursor) }

// Height returns the pane's content height in rows.
func (m *ChatListModel) Height() int { return m.height }

// ScrollInfo reports the chat list's scroll position for the pane scrollbar. It
// reflects the whole list, not the window: the scrollbar is the user's sense of
// how much account there is.
func (m *ChatListModel) ScrollInfo() components.ScrollInfo {
	if m.height <= 0 {
		return components.ScrollInfo{Total: m.total, Visible: m.total, Offset: 0}
	}
	return components.ScrollInfo{Total: m.total, Visible: m.height, Offset: m.viewStart()}
}

// CursorViewportRow returns the cursor's row index within the visible viewport.
func (m *ChatListModel) CursorViewportRow() int {
	if m.height <= 0 {
		return m.cursor
	}
	return m.cursor - m.viewStart()
}

// SetCursor moves the selection cursor to a whole-list index, clamped. It is a
// no-op when the list is empty.
func (m *ChatListModel) SetCursor(i int) {
	if m.total == 0 {
		return
	}
	m.cursor = i
	m.clampCursor()
}

// ChatAtViewportRow maps a content row (0-based, within the visible viewport) to
// a chat row. ok is false when the viewport row holds no chat: past the end of
// the list, or inside a window the core has not sent yet.
func (m *ChatListModel) ChatAtViewportRow(row int) (project.ChatRow, bool) {
	if row < 0 || m.total == 0 {
		return project.ChatRow{}, false
	}
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	if row >= visible {
		return project.ChatRow{}, false
	}
	return m.rowAt(m.viewStart() + row)
}

// ChatIndexAtViewportRow maps a viewport row to a whole-list index, for callers
// that move the cursor rather than read the row.
func (m *ChatListModel) ChatIndexAtViewportRow(row int) (int, bool) {
	if row < 0 || m.total == 0 {
		return 0, false
	}
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	if row >= visible {
		return 0, false
	}
	idx := m.viewStart() + row
	if idx >= m.total {
		return 0, false
	}
	return idx, true
}

func (m *ChatListModel) Init() tea.Cmd { return nil }

func (m *ChatListModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case keys.ActionMsg:
		switch msg.Action {
		case keys.ActionDown:
			if m.cursor < m.total-1 {
				m.cursor++
			}
		case keys.ActionUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case keys.ActionGoTop:
			m.cursor = 0
		case keys.ActionGoBottom:
			if m.total > 0 {
				m.cursor = m.total - 1
			}
		case keys.ActionScrollHalfDown:
			step := m.height * 2 / 3
			if step < 1 {
				step = 1
			}
			m.cursor += step
			m.clampCursor()
		case keys.ActionScrollHalfUp:
			step := m.height * 2 / 3
			if step < 1 {
				step = 1
			}
			m.cursor -= step
			m.clampCursor()
		case keys.ActionConfirm:
			if row, ok := m.rowAt(m.cursor); ok {
				m.activeID = row.ID
				return m, func() tea.Msg { return OpenChatMsg{ChatID: row.ID, Title: row.Title} }
			}
		}
	}
	return m, nil
}

func (m *ChatListModel) View() string {
	if m.total == 0 {
		// Painted here: the pane this returns to frames the content and paints
		// only what it adds itself (#260).
		return theme.S().Body.Render(m.spinner.View() + " Loading chats...")
	}
	visible := m.height
	if visible <= 0 {
		visible = m.total
	}
	start := m.viewStart()
	end := start + visible
	if end > m.total {
		end = m.total
	}

	w := m.width
	if w < 1 {
		w = 1
	}
	// Subtract 1 for outer container safety and 4 for the selection + presence prefix.
	const prefixW = 4
	inner := w - 1 - prefixW
	if inner < 1 {
		inner = 1
	}

	activeIdx := m.ActiveIdx()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		row, ok := m.rowAt(i)
		if !ok {
			// Inside the viewport but outside the window the core has sent. The
			// overscan in WindowRequest makes this unreachable in ordinary
			// scrolling; it shows as a blank row rather than a wrong one.
			lines = append(lines, "")
			continue
		}
		// The row's fill is decided first, because every piece of the row is
		// built on top of it. Rendering the pieces separately and wrapping the
		// finished row instead would lose the fill at the first reset inside it
		// — which is the presence dot, or the unread badge, or a highlighted
		// title. On a selected row that reads as the highlight tearing halfway
		// across, and on an unselected one as a hole in the canvas.
		base := theme.S().Body
		switch {
		case i == m.cursor && m.focused:
			base = theme.S().SelectedChat
		case i == activeIdx:
			base = theme.S().BodyBold
		}
		base = base.Inline(true)

		badge := rowIndicators(row, base)

		prefix := base.Render("    ")
		if i == activeIdx {
			prefix = base.Render("▶   ")
		}
		if row.IsUser && row.Online {
			dot := base.Foreground(theme.T().StatusOnline).Render("●")
			if i == activeIdx {
				prefix = base.Render("▶ ") + dot + base.Render(" ")
			} else {
				prefix = base.Render("  ") + dot + base.Render(" ")
			}
		}

		var content string
		if badge == "" {
			trunc := runewidth.Truncate(row.Title, inner, "…")
			lw := lipgloss.Width(trunc)
			content = m.styleTitle(i, row.ID, trunc, base) + padRow(base, inner-lw)
		} else {
			badgeW := lipgloss.Width(badge)
			maxTitleW := inner - badgeW - 1
			if maxTitleW < 0 {
				maxTitleW = 0
			}
			truncTitle := runewidth.Truncate(row.Title, maxTitleW, "…")
			titleW := lipgloss.Width(truncTitle)
			pad := inner - titleW - badgeW
			if pad < 0 {
				pad = 0
			}
			content = m.styleTitle(i, row.ID, truncTitle, base) + padRow(base, pad) + badge
		}

		lines = append(lines, prefix+content)
	}
	return strings.Join(lines, "\n")
}
