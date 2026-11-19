package screens_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setChats loads a window holding exactly these chats, converting them to the
// projection rows the component consumes. Fixtures go on describing chats; the
// core is what turns them into rows in production.
func setChats(m *screens.ChatListModel, chats []domain.Chat) {
	rows := make([]project.ChatRow, 0, len(chats))
	for _, c := range chats {
		rows = append(rows, project.ChatRow{
			ID:         c.ID,
			Title:      c.Title,
			IsUser:     c.Peer.IsUser(),
			Online:     c.Online,
			Unread:     c.UnreadCount,
			Mentions:   c.UnreadMentionsCount,
			Reactions:  c.UnreadReactionsCount,
			UnreadMark: c.UnreadMark,
			Muted:      c.IsMuted,
		})
	}
	m.SetWindow(0, len(rows), rows)
}

func makeTestChats() []domain.Chat {
	return []domain.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
		{ID: 3, Title: "Charlie"},
	}
}

func TestChatList_View_ShowsChats(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, makeTestChats())
	view := m.View()
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Bob")
}

func TestChatList_Down_MovesCursor(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	cl := newPane.(*screens.ChatListModel)
	assert.Equal(t, 1, cl.Cursor())
}

func TestChatList_Confirm_EmitsOpenChat(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	_, cmd := m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	require.NotNil(t, cmd)
	msg := cmd()
	assert.IsType(t, screens.OpenChatMsg{}, msg)
	assert.Equal(t, int64(1), msg.(screens.OpenChatMsg).ChatID)
}

func TestChatList_Context(t *testing.T) {
	m := screens.NewChatListModel()
	assert.Equal(t, keys.ContextChatList, m.Context())
}

func TestChatList_ShowsUnreadBadge(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Alice", UnreadCount: 3},
		{ID: 2, Title: "Bob", UnreadCount: 0},
	})
	view := m.View()
	assert.Contains(t, view, "[3]")
	assert.NotContains(t, view, "[0]")
}

func TestChatList_Badge99Plus(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Spam", UnreadCount: 150}})
	view := m.View()
	assert.Contains(t, view, "[99+]")
}

func TestChatList_NoBadgeWhenZero(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Quiet", UnreadCount: 0}})
	view := m.View()
	assert.NotContains(t, view, "[0]")
}

func TestChatList_ManualUnread_ShowsDotBadge(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Marked", UnreadMark: true, UnreadCount: 0},
	})
	view := m.View()
	assert.Contains(t, view, "[•]", "manually-marked chat with no count shows the dot badge")
	assert.NotContains(t, view, "[0]")
}

func TestChatList_ManualUnread_NumericBadgeWinsWithCount(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Both", UnreadMark: true, UnreadCount: 5},
	})
	view := m.View()
	assert.Contains(t, view, "[5]")
	assert.NotContains(t, view, "[•]", "numeric count takes precedence over the manual-unread dot")
}

func TestChatList_Muted_ShowsMarker(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Quiet", IsMuted: true},
	})
	view := m.View()
	assert.Contains(t, view, "×", "muted chat shows the mute marker")
}

func TestChatList_ShowsReactionIndicator(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadReactionsCount: 1}})
	view := m.View()
	assert.Contains(t, view, "♥")
}

func TestChatList_ReactionIndicatorWithCount(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadReactionsCount: 3}})
	view := m.View()
	assert.Contains(t, view, "♥3")
}

func TestChatList_NoReactionIndicatorWhenZero(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadReactionsCount: 0}})
	view := m.View()
	assert.NotContains(t, view, "♥")
}

func TestChatList_ReactionAndUnreadBadgeCoexist(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadCount: 2, UnreadReactionsCount: 1}})
	view := m.View()
	assert.Contains(t, view, "♥")
	assert.Contains(t, view, "[2]")
}

func TestChatList_MutedReactionUnread_AllRender(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", IsMuted: true, UnreadCount: 5, UnreadReactionsCount: 1}})
	view := m.View()
	assert.Contains(t, view, "×")
	assert.Contains(t, view, "♥")
	assert.Contains(t, view, "[5]")
}

func TestChatList_ShowsMentionIndicator(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadMentionsCount: 1}})
	view := m.View()
	assert.Contains(t, view, "@")
}

func TestChatList_MentionIndicatorWithCount(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadMentionsCount: 3}})
	view := m.View()
	assert.Contains(t, view, "@3")
}

func TestChatList_NoMentionIndicatorWhenZero(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadMentionsCount: 0}})
	view := m.View()
	assert.NotContains(t, view, "@")
}

func TestChatList_MentionReactionUnread_AllRender(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{{ID: 1, Title: "Alice", UnreadCount: 5, UnreadReactionsCount: 1, UnreadMentionsCount: 2}})
	view := m.View()
	assert.Contains(t, view, "♥")
	assert.Contains(t, view, "@2")
	assert.Contains(t, view, "[5]")
}

func TestChatList_NotMuted_NoMarker(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Loud", IsMuted: false, UnreadCount: 3},
	})
	view := m.View()
	assert.NotContains(t, view, "×", "non-muted chat has no mute marker")
}

func TestChatList_MutedUnread_WidthConsistent(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(30, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Plain", UnreadCount: 5},
		{ID: 2, Title: "Muted", UnreadCount: 5, IsMuted: true},
		{ID: 3, Title: "Mark", UnreadMark: true},
	})
	lines := strings.Split(m.View(), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	w0 := lipgloss.Width(lines[0])
	w1 := lipgloss.Width(lines[1])
	w2 := lipgloss.Width(lines[2])
	assert.Equal(t, w0, w1, "muted row must match plain unread row width")
	assert.Equal(t, w0, w2, "manual-unread row must match plain unread row width")
}

func TestChatList_SetChats_PreservesCursorByID(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats()) // [A(1), B(2), C(3)], cursor at 0 (A)
	assert.Equal(t, 0, m.Cursor())

	// Reorder: A moves to index 1. Cursor must follow A to index 1.
	setChats(m, []domain.Chat{
		{ID: 2, Title: "Bob"},
		{ID: 1, Title: "Alice"},
		{ID: 3, Title: "Charlie"},
	})
	assert.Equal(t, 1, m.Cursor()) // cursor followed A(id=1) to its new position
}

func TestChatList_SetChats_CursorClampsWhenChatRemoved(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats()) // [A(1), B(2), C(3)]
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionDown}) // cursor -> 2 (C)
	m = newPane.(*screens.ChatListModel)
	assert.Equal(t, 2, m.Cursor())

	setChats(m, []domain.Chat{
		{ID: 1, Title: "Alice"},
		{ID: 2, Title: "Bob"},
	})

	// The chat the cursor was on is gone, so there is nothing to follow: the
	// cursor stays where it was, clamped to the shorter list, rather than
	// jumping the user back to the top.
	assert.Equal(t, 1, m.Cursor())
}

func TestChatList_EmptyShowsSpinner(t *testing.T) {
	m := screens.NewChatListModel()
	view := m.View()
	assert.Contains(t, view, "Loading chats...")
	assert.True(t, strings.HasPrefix(view, "["))
}

func TestChatList_Confirm_SetsActiveIdx(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	m = newPane.(*screens.ChatListModel)
	assert.Equal(t, 1, m.ActiveIdx())
}

func TestChatList_Navigation_DoesNotChangeActiveIdx(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	assert.Equal(t, -1, m.ActiveIdx(), "no chat is open yet, so no row is the active one")
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	assert.Equal(t, 2, m.Cursor(), "cursor moved")
	assert.Equal(t, -1, m.ActiveIdx(), "moving the cursor does not open a chat")
}

func TestChatList_SetChats_PreservesActiveIdxByID(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	m = newPane.(*screens.ChatListModel)
	require.Equal(t, 1, m.ActiveIdx())
	setChats(m, []domain.Chat{
		{ID: 2, Title: "Bob"},
		{ID: 1, Title: "Alice"},
		{ID: 3, Title: "Charlie"},
	})
	assert.Equal(t, 0, m.ActiveIdx(), "activeIdx followed Bob(id=2) to new position")
}

func TestChatList_SetActiveByID_SetsBothCursorAndActive(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	m.SetActiveByID(3)
	assert.Equal(t, 2, m.ActiveIdx())
	assert.Equal(t, 2, m.Cursor())
}

func TestChatList_View_ShowsArrowOnActiveItem(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	m = newPane.(*screens.ChatListModel)

	lines := strings.Split(m.View(), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Contains(t, lines[1], "▶", "active item (Bob, index 1) must show ▶")
	assert.NotContains(t, lines[0], "▶", "non-active item (Alice, index 0) must not show ▶")
}

func TestChatList_View_NoHighlightWhenUnfocused(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, makeTestChats())
	m.SetFocused(false)
	view := m.View()
	assert.Contains(t, view, "Alice")
}

func TestChatList_View_HighlightOnCursorWhenFocused(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, makeTestChats())
	m.SetFocused(true)
	view := m.View()
	assert.Contains(t, view, "Alice", "cursor row must appear")
}

func TestChatList_SelectedChat_ReturnsActiveItem(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, makeTestChats())
	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionConfirm})
	m = newPane.(*screens.ChatListModel)
	newPane, _ = m.Update(keys.ActionMsg{Action: keys.ActionDown})
	m = newPane.(*screens.ChatListModel)
	chat, ok := m.SelectedChat()
	require.True(t, ok)
	assert.Equal(t, int64(2), chat.ID, "SelectedChat returns confirmed active item, not cursor")
}

func TestChatList_View_OnlineUserShowsDot(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Alice", Peer: domain.Peer{Type: domain.PeerUser}, Online: true},
		{ID: 2, Title: "Bob", Peer: domain.Peer{Type: domain.PeerUser}, Online: false},
	})
	view := m.View()
	assert.Contains(t, view, "●")
}

func TestChatList_LongTitleShowsEllipsis(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(20, 10)
	setChats(m, []domain.Chat{{ID: 1, Title: strings.Repeat("x", 50)}})
	view := m.View()
	assert.Contains(t, view, "…")
}

func TestChatList_EmojiTitleBadgeWidthConsistent(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(30, 10)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Plain title", UnreadCount: 5},
		{ID: 2, Title: "Emoji 🌐 title", UnreadCount: 5},
	})
	lines := strings.Split(m.View(), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	w0 := lipgloss.Width(lines[0])
	w1 := lipgloss.Width(lines[1])
	assert.Equal(t, w0, w1, "lines with and without emoji must have same visual width")
}

func TestChatList_View_OfflineUserNoDot(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Alice", Peer: domain.Peer{Type: domain.PeerUser}, Online: false},
	})
	view := m.View()
	assert.NotContains(t, view, "●")
}

func TestChatList_View_GroupNoDot(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Team", Peer: domain.Peer{Type: domain.PeerGroup}, Online: true},
	})
	view := m.View()
	assert.NotContains(t, view, "●")
}

func TestChatList_View_OnlineDotWidthConsistent(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(30, 20)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Alice", Peer: domain.Peer{Type: domain.PeerUser}, Online: true},
		{ID: 2, Title: "Bob", Peer: domain.Peer{Type: domain.PeerUser}, Online: false},
	})
	lines := strings.Split(m.View(), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	w0 := lipgloss.Width(lines[0])
	w1 := lipgloss.Width(lines[1])
	assert.Equal(t, w0, w1, "online and offline rows must have the same visual width")
}

func TestChatList_CursorChat(t *testing.T) {
	m := screens.NewChatListModel()
	setChats(m, []domain.Chat{{ID: 1, Title: "A"}, {ID: 2, Title: "B"}})
	m.SetCursorByID(2)
	c, ok := m.CursorChat()
	require.True(t, ok)
	assert.Equal(t, int64(2), c.ID)
}

func TestChatList_CursorViewportRow_Scrolled(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(20, 3) // 3 visible rows
	chats := make([]domain.Chat, 10)
	for i := range chats {
		chats[i] = domain.Chat{ID: int64(i + 1)}
	}
	setChats(m, chats)
	m.SetCursorByID(6) // index 5: start = 5-3+1 = 3, row = 5-3 = 2
	assert.Equal(t, 2, m.CursorViewportRow())
}

func TestChatListModel_ScrollInfo(t *testing.T) {
	m := screens.NewChatListModel()
	chats := make([]domain.Chat, 20)
	for i := range chats {
		chats[i] = domain.Chat{ID: int64(i + 1), Title: "c"}
	}
	setChats(m, chats)
	m.SetSize(20, 5) // 5 visible rows

	info := m.ScrollInfo()
	assert.Equal(t, 20, info.Total)
	assert.Equal(t, 5, info.Visible)
	assert.Equal(t, 0, info.Offset, "cursor at top")

	for i := 0; i < 19; i++ {
		m.Update(keys.ActionMsg{Action: keys.ActionDown})
	}
	info = m.ScrollInfo()
	assert.Equal(t, 15, info.Offset, "cursor at bottom => start = 20-5")
}

func TestChatIndexAtViewportRow_NoScroll(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(20, 10)
	chats := make([]domain.Chat, 5)
	for i := range chats {
		chats[i] = domain.Chat{ID: int64(i + 1), Peer: domain.Peer{ID: int64(i + 1), Type: domain.PeerUser}}
	}
	setChats(m, chats)

	idx, ok := m.ChatIndexAtViewportRow(0)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	idx, ok = m.ChatIndexAtViewportRow(3)
	require.True(t, ok)
	assert.Equal(t, 3, idx)

	// Row past the last chat -> not ok.
	_, ok = m.ChatIndexAtViewportRow(5)
	assert.False(t, ok)
}

func TestChatIndexAtViewportRow_Scrolled(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(20, 3) // viewport height 3
	chats := make([]domain.Chat, 10)
	for i := range chats {
		chats[i] = domain.Chat{ID: int64(i + 1), Peer: domain.Peer{ID: int64(i + 1), Type: domain.PeerUser}}
	}
	setChats(m, chats)
	m.SetCursor(9) // scrolled to bottom: start = 9-3+1 = 7

	idx, ok := m.ChatIndexAtViewportRow(0)
	require.True(t, ok)
	assert.Equal(t, 7, idx) // top visible row is chat index 7

	idx, ok = m.ChatIndexAtViewportRow(2)
	require.True(t, ok)
	assert.Equal(t, 9, idx)
}

// Jumping to the end moves the cursor first and only then asks for the window
// around it, so the arriving window is one the cursor is not inside. Resetting
// the cursor there blanks the pane until something moves it again.
func TestChatList_WindowArrivingWithTheCursorOutsideItKeepsTheCursor(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 5)
	m.SetWindow(0, 200, rowsFor(1, 40))
	m.SetCursor(199) // G

	m.SetWindow(120, 200, rowsFor(121, 200))

	assert.Equal(t, 199, m.Cursor(), "the cursor must not be dragged back to the top")
	row, ok := m.CursorChat()
	require.True(t, ok, "the window that arrived contains the cursor")
	assert.Equal(t, int64(200), row.ID)
}

func TestChatList_JumpToEndAsksForTheWindowAroundIt(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(40, 5)
	m.SetWindow(0, 200, rowsFor(1, 40))
	m.WindowRequest() // the window it already has

	newPane, _ := m.Update(keys.ActionMsg{Action: keys.ActionGoBottom})
	m = newPane.(*screens.ChatListModel)

	offset, limit, changed := m.WindowRequest()
	require.True(t, changed)
	assert.LessOrEqual(t, offset, 195, "the window must reach the end of the list")
	assert.GreaterOrEqual(t, offset+limit, 200)
}

func TestChatList_VS16EmojiTitleWidthConsistent(t *testing.T) {
	m := screens.NewChatListModel()
	m.SetSize(22, 10)
	setChats(m, []domain.Chat{
		{ID: 1, Title: "Plain title here", UnreadCount: 2},
		{ID: 2, Title: "⚙️⚙️⚙️ Long tool name", UnreadCount: 2},
		{ID: 3, Title: "⚙️⚙️⚙️⚙️⚙️⚙️⚙️⚙️⚙️⚙️⚙️⚙️"},
	})
	lines := strings.Split(m.View(), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	w0 := lipgloss.Width(lines[0])
	assert.Equal(t, w0, lipgloss.Width(lines[1]), "VS16 emoji title with badge must not overflow")
	assert.Equal(t, w0, lipgloss.Width(lines[2]), "VS16 emoji title without badge must not overflow")
}

// rowsFor builds rows with ids first..last inclusive.
func rowsFor(first, last int64) []project.ChatRow {
	out := make([]project.ChatRow, 0, last-first+1)
	for id := first; id <= last; id++ {
		out = append(out, project.ChatRow{ID: id, Title: "chat"})
	}
	return out
}
