package components_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func makeMessages(n int) []store.Message {
	msgs := make([]store.Message, n)
	now := time.Now()
	for i := range msgs {
		msgs[i] = store.Message{ID: i + 1, ChatID: 1, Text: fmt.Sprintf("msg %d", i+1), Date: now}
	}
	return msgs
}

func TestMessageList_ShowsAll_WhenFewMessages(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	view := ml.View()
	assert.Contains(t, view, "msg 1")
	assert.Contains(t, view, "msg 3")
}

func TestMessageList_VirtualViewport(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(10))
	view := ml.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 3)
}

func TestMessageList_Count(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(5))
	assert.Equal(t, 5, ml.Count())
}

func TestMessageList_ScrollUp_SmallMessage(t *testing.T) {
	// Small message (h=3, viewHeight=3): ScrollUp enters at lineOffset=h-2=1,
	// showing content+bottom. Never shows bottom-border-only (lineOffset=h-1).
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6)) // each h=3; positionAtBottom → viewStart=5
	ml.ScrollUp()
	assert.Equal(t, 4, ml.ViewStart())
	assert.Equal(t, 1, ml.LineOffset()) // h-2 = 1: content+bottom visible
}

func TestMessageList_ScrollUp_LineLevelWithinLargeMessage(t *testing.T) {
	// Large message (h > viewHeight): entered at lineOffset=h-viewHeight, scrolled line-by-line.
	ml := components.NewMessageList(3, 80)
	bigMsg := store.Message{ID: 1, ChatID: 1, Text: "L1\nL2\nL3\nL4\nL5", Date: time.Now()}
	ml.SetMessages([]store.Message{bigMsg})
	// positionAtBottom → (0, h-viewHeight=4): lineOffset=4
	assert.Equal(t, 4, ml.LineOffset())
	ml.ScrollUp() // 4 → 3
	assert.Equal(t, 3, ml.LineOffset())
	ml.ScrollUp() // 3 → 2
	assert.Equal(t, 2, ml.LineOffset())
}

func TestMessageList_ScrollDown_SmallMessage_LineByLine(t *testing.T) {
	// After scrolling up, ScrollDown goes line-by-line through small messages.
	// For h=3: lineOffset 0 → 1 → jump to next (h-1=2 frame is always skipped).
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6)) // viewStart=5 (one message in viewport)

	// Scroll up into msg4: enter at lineOffset=1, then reveal fully
	ml.ScrollUp() // viewStart=4, lineOffset=1
	ml.ScrollUp() // lineOffset=0 (full msg4 visible)
	assert.Equal(t, 4, ml.ViewStart())
	assert.Equal(t, 0, ml.LineOffset())

	// Scroll down: line-by-line through msg4
	ml.ScrollDown() // lineOffset: 0 → 1 (top border hidden)
	assert.Equal(t, 4, ml.ViewStart())
	assert.Equal(t, 1, ml.LineOffset())

	ml.ScrollDown() // next would be lineOffset=2=h-1 (bad frame): skip to next message
	assert.Equal(t, 5, ml.ViewStart())
	assert.Equal(t, 0, ml.LineOffset())
}

func TestMessageList_AtTop_TrueWhenAtStart(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(2)) // 2 msgs × 3 lines = 6 < 20 → viewStart=0, lineOffset=0
	assert.True(t, ml.AtTop())
}

func TestMessageList_AtTop_FalseAfterScroll(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6)) // viewStart=5 after SetMessages
	assert.False(t, ml.AtTop())
}

func TestMessageList_AtTop_TrueAfterScrollingToStart(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	// 4 msgs × h=3; positionAtBottom → viewStart=3.
	// Each message takes 2 ScrollUps (enter at lineOffset=1, then lineOffset=0).
	// 3 messages above viewStart=3: 3 × 2 = 6 ScrollUps total.
	ml.SetMessages(makeMessages(4))
	for i := 0; i < 6; i++ {
		ml.ScrollUp()
	}
	assert.True(t, ml.AtTop())
}

func TestMessageList_OldestID_ReturnsFirstMessage(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	assert.Equal(t, 1, ml.OldestID())
}

func TestMessageList_OldestID_ZeroWhenEmpty(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	assert.Equal(t, 0, ml.OldestID())
}

func TestMessageList_PrependMessages_PreservesViewStart(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(1)) // 1 msg × 3 lines = viewHeight=3 → viewStart=0
	older := []store.Message{
		{ID: 10, ChatID: 1, Text: "old1", Date: time.Now()},
		{ID: 11, ChatID: 1, Text: "old2", Date: time.Now()},
	}
	ml.PrependMessages(older)
	// viewStart shifts by len(older) so the same message stays on screen
	assert.Equal(t, 2, ml.ViewStart())
}

func TestMessageList_LargeMessage_ShowsBottomPortion(t *testing.T) {
	// Single message taller than viewport: should show the bottom portion by default.
	ml := components.NewMessageList(3, 80)
	bigMsg := store.Message{ID: 1, ChatID: 1, Text: "L1\nL2\nL3\nL4\nL5", Date: time.Now()}
	ml.SetMessages([]store.Message{bigMsg})
	// lineOffset > 0: some top lines are hidden
	assert.Greater(t, ml.LineOffset(), 0)
	view := ml.View()
	assert.Contains(t, stripANSI(view), "L5")
	assert.NotContains(t, stripANSI(view), "L1")
}

func TestMessageList_LargeMessage_ScrollUpRevealsTopLines(t *testing.T) {
	ml := components.NewMessageList(3, 80)
	bigMsg := store.Message{ID: 1, ChatID: 1, Text: "L1\nL2\nL3\nL4\nL5", Date: time.Now()}
	ml.SetMessages([]store.Message{bigMsg})
	initialOffset := ml.LineOffset()
	for i := 0; i < initialOffset; i++ {
		ml.ScrollUp()
	}
	assert.True(t, ml.AtTop())
	view := ml.View()
	assert.Contains(t, stripANSI(view), "L1")
}

func TestMessageList_ScrollToFirstUnread_ViewportFilled(t *testing.T) {
	// When few unread messages don't fill the viewport, older messages should fill
	// the remaining space so the viewport is not mostly empty.
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(10)) // IDs 1..10, last one is "unread"
	ml.ScrollToFirstUnread(9)        // ID 10 is unread
	view := ml.View()
	plain := stripANSI(view)
	// First unread must be visible.
	assert.Contains(t, plain, "msg 10")
	// Older messages must also be visible (viewport filled from history).
	// positionAtBottom starts at index 3 with lineOffset=1 → msg 4 is the oldest visible.
	assert.Contains(t, plain, "msg 5")
}

func TestMessageList_ScrollToFirstUnread_PositionsAtFirstUnread(t *testing.T) {
	// When the first unread and all following messages fit in the viewport,
	// positionAtBottom fills upward — first unread must still be visible.
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	found := ml.ScrollToFirstUnread(3) // msgs with ID>3 are unread: msg4, msg5
	assert.True(t, found)
	plain := stripANSI(ml.View())
	assert.Contains(t, plain, "msg 4") // first unread visible
	assert.Contains(t, plain, "msg 5") // subsequent unread also visible
}

func TestMessageList_ScrollToFirstUnread_ManyUnread_AtTop(t *testing.T) {
	// When many unread messages overflow the viewport, the first unread is at
	// the top (viewStart points exactly to the first unread message index).
	ml := components.NewMessageList(6, 80) // viewport = 6 lines = 2 messages
	ml.SetMessages(makeMessages(10))        // IDs 1..10
	ml.ScrollToFirstUnread(3)              // msgs 4..10 unread, 7 msgs × h=3 = 21 > 6
	assert.Equal(t, 3, ml.ViewStart())     // index 3 = msg4
	assert.Equal(t, 0, ml.LineOffset())
	assert.Contains(t, stripANSI(ml.View()), "msg 4")
}

func TestMessageList_ScrollToFirstUnread_AllReadReturnsFalse(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	found := ml.ScrollToFirstUnread(10) // all read
	assert.False(t, found)
}

func TestMessageList_ScrollToFirstUnread_AllUnread(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	found := ml.ScrollToFirstUnread(0) // none read
	assert.True(t, found)
	assert.Equal(t, 0, ml.ViewStart())
}

func TestMessageList_View_RendersEntityStyledText(t *testing.T) {
	ml := components.NewMessageList(5, 80)
	msgs := []store.Message{
		{
			ID:     1,
			ChatID: 1,
			Text:   "hello",
			Date:   time.Now(),
			Entities: []store.MessageEntity{
				{Type: "bold", Offset: 0, Length: 5},
			},
		},
	}
	ml.SetMessages(msgs)
	view := ml.View()
	assert.Contains(t, stripANSI(view), "hello")
}
