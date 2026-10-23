package screens

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/layout"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// OpenChatMsg asks the root model to open a chat. It carries an id and a title
// rather than a domain.Chat: a client holds no peer, and the chat's header
// arrives on the chat:<id> projection once the subscription lands. The title is
// here only so the pane has something to draw during that gap.
//
// A contact found by search that has never been messaged opens the same way:
// the owner kept the address search returned, so the id is enough (#278).
type OpenChatMsg struct {
	ChatID int64
	Title  string
	// IsForum opens the chat through its topic list instead (#275). TopicID,
	// when set, opens that topic of it as well, as a notification does;
	// TopicTitle is what the header shows until the topic's projection lands.
	IsForum    bool
	TopicID    int
	TopicTitle string
}

// ForwardToChatRequest is emitted by the forward-mode chat picker when the user
// confirms a target chat. Both ends are chat ids; addressing them is the
// owner's business, including a search hit with no dialog (#278).
type ForwardToChatRequest struct {
	ToChatID int64
	// Title names the target in the result status. The picker had the chat in
	// hand when the user chose it, so it travels along rather than being looked
	// up again — a search hit may not be a chat the owner holds at all.
	Title string
	// ToTopicID is the forum topic forwarded into, 0 outside a forum (#275).
	ToTopicID int
	MsgID     int
	Comment   string // optional; sent as a separate message before the forward
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
// screen. The window, the cursor and the highlight are rowList's; what is here
// is what makes the rows chats.
type ChatListModel struct {
	rowList[project.ChatRow]
	width   int
	spinner components.Spinner
}

func NewChatListModel() *ChatListModel {
	return &ChatListModel{rowList: rowList[project.ChatRow]{idOf: func(r project.ChatRow) int64 { return r.ID }}}
}

// TickSpinner advances the spinner frame. Called by root on SpinnerTickMsg.
func (m *ChatListModel) TickSpinner() { m.spinner.Tick() }

// IsLoadingChats reports whether the chat list is still showing its
// "Loading chats..." spinner (no chats received yet), matching View. Drives the
// spinner tick loop (issue #147).
func (m *ChatListModel) IsLoadingChats() bool { return m.total == 0 }

// HighlightChat starts a fade highlight on the chat-list row for the given id.
func (m *ChatListModel) HighlightChat(id int64) { m.highlight(id) }

// StepChatHighlight advances the chat-row highlight fade by one step. Returns
// true while still active; clears the highlight and returns false at 0. No-op
// (false) when no highlight is active.
func (m *ChatListModel) StepChatHighlight() bool { return m.StepHighlight() }

// HighlightedChatID returns the currently highlighted chat id (0 when none).
func (m *ChatListModel) HighlightedChatID() int64 { return m.highlightID }

// styleTitle applies the fade-accent foreground to a row's (already truncated)
// title while that row is the active highlight target. The focused-cursor row
// keeps its selection background instead, so it is left unstyled here.
func (m *ChatListModel) styleTitle(i int, id int64, truncated string, base lipgloss.Style) string {
	return styleHighlighted(&m.rowList, i, id, truncated, base, theme.T().HighlightBaseChat)
}

// styleHighlighted paints a row title with the fade accent while the row is the
// highlight target, for any list built on rowList.
func styleHighlighted[R any](m *rowList[R], i int, id int64, truncated string, base lipgloss.Style, from color.Color) string {
	if m.highlightStep <= 0 || id != m.highlightID {
		return base.Render(truncated)
	}
	if i == m.cursor && m.focused {
		// The selection fill supplies the colour; the title only takes the fill.
		return base.Render(truncated)
	}
	fg := components.FadeAccentColor(theme.T().HighlightAccent, from, m.highlightStep, components.HighlightFadeSteps)
	return base.Foreground(fg).Render(truncated)
}

// SelectedChat returns the open chat's row, when the window holds it.
func (m *ChatListModel) SelectedChat() (project.ChatRow, bool) {
	if idx := m.ActiveIdx(); idx >= 0 {
		return m.rowAt(idx)
	}
	return project.ChatRow{}, false
}

func (m *ChatListModel) Context() keys.Context { return keys.ContextChatList }

func (m *ChatListModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Width returns the pane's content width in cells.
func (m *ChatListModel) Width() int { return m.width }

// CursorChat returns the row currently under the cursor.
func (m *ChatListModel) CursorChat() (project.ChatRow, bool) { return m.rowAt(m.cursor) }

// ChatAtViewportRow maps a content row (0-based, within the visible viewport) to
// a chat row. ok is false when the viewport row holds no chat: past the end of
// the list, or inside a window the core has not sent yet.
func (m *ChatListModel) ChatAtViewportRow(row int) (project.ChatRow, bool) {
	return m.rowAtViewportRow(row)
}

// ChatIndexAtViewportRow maps a viewport row to a whole-list index, for callers
// that move the cursor rather than read the row.
func (m *ChatListModel) ChatIndexAtViewportRow(row int) (int, bool) {
	return m.indexAtViewportRow(row)
}

func (m *ChatListModel) Init() tea.Cmd { return nil }

func (m *ChatListModel) Update(msg tea.Msg) (layout.Pane, tea.Cmd) {
	if msg, ok := msg.(keys.ActionMsg); ok {
		if m.move(msg.Action) {
			return m, nil
		}
		if msg.Action == keys.ActionConfirm {
			if row, ok := m.rowAt(m.cursor); ok {
				m.activeID = row.ID
				return m, func() tea.Msg { return OpenChatMsg{ChatID: row.ID, Title: row.Title, IsForum: row.IsForum} }
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
	start, end := m.visibleRange()

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
		base := rowBase(i == m.cursor && m.focused, i == activeIdx)

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

		lines = append(lines, prefix+rowContent(row.Title, badge, inner, base, func(t string) string {
			return m.styleTitle(i, row.ID, t, base)
		}))
	}
	return strings.Join(lines, "\n")
}

// rowBase is a list row's own style: the selection fill under a focused
// cursor, bold for the open row, body text otherwise.
func rowBase(selected, active bool) lipgloss.Style {
	base := theme.S().Body
	switch {
	case selected:
		base = theme.S().SelectedChat
	case active:
		base = theme.S().BodyBold
	}
	return base.Inline(true)
}

// rowContent lays out a row's title and its right-aligned indicators across
// inner cells, truncating the title to leave the indicators room.
func rowContent(title, badge string, inner int, base lipgloss.Style, styleTitle func(string) string) string {
	if badge == "" {
		trunc := layout.TruncateText(title, inner)
		return styleTitle(trunc) + padRow(base, inner-lipgloss.Width(trunc))
	}
	badgeW := lipgloss.Width(badge)
	maxTitleW := max(inner-badgeW-1, 0)
	truncTitle := layout.TruncateText(title, maxTitleW)
	pad := max(inner-lipgloss.Width(truncTitle)-badgeW, 0)
	return styleTitle(truncTitle) + padRow(base, pad) + badge
}
