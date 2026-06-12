package components_test

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMessageList_SelectedBubbleRect_Incoming(t *testing.T) {
	ml := components.NewMessageList(3, 40) // one message exactly fills the viewport
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	ml.View()

	rect, ok := ml.SelectedBubbleRect()
	require.True(t, ok)
	assert.Equal(t, 0, rect.Top)
	assert.Equal(t, 0, rect.Left) // incoming bubbles hug the left margin
	assert.Equal(t, 3, rect.Height)
	assert.Greater(t, rect.Width, 0)
}

func TestMessageList_SelectedBubbleRect_Outgoing(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", IsOut: true, Date: time.Now()}})
	ml.View()

	rect, ok := ml.SelectedBubbleRect()
	require.True(t, ok)
	assert.Equal(t, 0, rect.Top)
	assert.Greater(t, rect.Left, 0)           // pushed right
	assert.Equal(t, 40, rect.Left+rect.Width) // right edge == viewWidth
}

func TestMessageList_SelectedBubbleRect_NoSelection(t *testing.T) {
	ml := components.NewMessageList(10, 40) // empty list
	ml.View()
	_, ok := ml.SelectedBubbleRect()
	assert.False(t, ok)
}

func TestMessageList_ScrollUp_SmallMessage(t *testing.T) {
	// Small message (h=3, viewHeight=3): ScrollUp enters at lineOffset=h-2=1,
	// showing content+bottom. Never shows bottom-border-only (lineOffset=h-1).
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6)) // each h=3; positionAtBottom → viewStart=6
	ml.ScrollUp()
	assert.Equal(t, 5, ml.ViewStart())
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
	ml.SetMessages(makeMessages(6)) // viewStart=6 (one message in viewport)

	// Scroll up into msg5: enter at lineOffset=1, then reveal fully
	ml.ScrollUp() // viewStart=5, lineOffset=1
	ml.ScrollUp() // lineOffset=0 (full msg5 visible)
	assert.Equal(t, 5, ml.ViewStart())
	assert.Equal(t, 0, ml.LineOffset())

	// Scroll down: line-by-line through msg5
	ml.ScrollDown() // lineOffset: 0 → 1 (top border hidden)
	assert.Equal(t, 5, ml.ViewStart())
	assert.Equal(t, 1, ml.LineOffset())

	ml.ScrollDown() // next would be lineOffset=2=h-1 (bad frame): skip to next message
	assert.Equal(t, 6, ml.ViewStart())
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
	// items=[sep, msg1..msg4]; positionAtBottom → viewStart=4.
	// Each item takes 2 ScrollUps (enter at lineOffset=1, then lineOffset=0).
	// 4 items above viewStart=4: 4 × 2 = 8 ScrollUps total.
	ml.SetMessages(makeMessages(4))
	for i := 0; i < 8; i++ {
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
	ml.SetMessages(makeMessages(1)) // 1 msg × 3 lines = viewHeight=3 → viewStart=1
	older := []store.Message{
		{ID: 10, ChatID: 1, Text: "old1", Date: time.Now()},
		{ID: 11, ChatID: 1, Text: "old2", Date: time.Now()},
	}
	ml.PrependMessages(older)
	// viewStart shifts by len(newItems)-len(oldItems) so the same message stays on screen
	assert.Equal(t, 3, ml.ViewStart())
}

// Rapid scroll-up fires several identical "load older" requests before the first
// resolves, so the same chunk can be prepended more than once (issue #120). The
// merge must be idempotent: re-prepending already-present IDs is a no-op, never a
// duplicated date-range "ring".
func TestMessageList_PrependMessages_SkipsDuplicateIDs(t *testing.T) {
	now := time.Now()
	ml := components.NewMessageList(10, 40)
	ml.SetMessages([]store.Message{
		{ID: 10, ChatID: 1, Text: "a", Date: now},
		{ID: 11, ChatID: 1, Text: "b", Date: now},
	})
	older := []store.Message{
		{ID: 8, ChatID: 1, Text: "old1", Date: now},
		{ID: 9, ChatID: 1, Text: "old2", Date: now},
	}
	ml.PrependMessages(older)
	require.Equal(t, 4, ml.Count())

	// The same chunk arrives again (duplicate in-flight load): no growth, no dupes.
	ml.PrependMessages(older)
	assert.Equal(t, 4, ml.Count())
	assert.Equal(t, 8, ml.OldestID())
}

// A chunk that partially overlaps the current list (shared boundary message) must
// only contribute its genuinely-new messages.
func TestMessageList_PrependMessages_PartialOverlap(t *testing.T) {
	now := time.Now()
	ml := components.NewMessageList(10, 40)
	ml.SetMessages([]store.Message{
		{ID: 10, ChatID: 1, Text: "a", Date: now},
		{ID: 11, ChatID: 1, Text: "b", Date: now},
	})
	// 7,8,9 are new; 10 repeats the current oldest.
	older := []store.Message{
		{ID: 7, ChatID: 1, Text: "old0", Date: now},
		{ID: 8, ChatID: 1, Text: "old1", Date: now},
		{ID: 9, ChatID: 1, Text: "old2", Date: now},
		{ID: 10, ChatID: 1, Text: "a", Date: now},
	}
	ml.PrependMessages(older)
	assert.Equal(t, 5, ml.Count())
	assert.Equal(t, 7, ml.OldestID())
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
	assert.Equal(t, 0, ml.LineOffset()) // scrolled to top of the large message
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
	ml.SetMessages(makeMessages(5))    // IDs 1..5
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
	ml.SetMessages(makeMessages(10))       // IDs 1..10
	ml.ScrollToFirstUnread(3)              // msgs 4..10 unread, 7 msgs × h=3 = 21 > 6
	assert.Equal(t, 4, ml.ViewStart())     // index 4 = msg4
	assert.Equal(t, 0, ml.LineOffset())
	assert.Contains(t, stripANSI(ml.View()), "msg 4")
}

func TestMessageList_ScrollToFirstUnread_AllReadReturnsFalse(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5))     // IDs 1..5
	found := ml.ScrollToFirstUnread(10) // all read
	assert.False(t, found)
}

func TestMessageList_ScrollToFirstUnread_AllUnread(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5))    // IDs 1..5
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

func TestMessageList_PhotoPlaceholderInView(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	msg := store.Message{
		ID:    1,
		Media: &store.MediaRef{Kind: store.MediaPhoto},
		Photo: &store.PhotoRef{ID: 42},
	}
	ml.SetMessages([]store.Message{msg})
	view := ml.View()
	require.Contains(t, view, "📷 photo", "should show placeholder when image not loaded")
}

func TestMessageList_SetImage_UpdatesView(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	msg := store.Message{
		ID:    1,
		Media: &store.MediaRef{Kind: store.MediaPhoto},
		Photo: &store.PhotoRef{ID: 99},
	}
	ml.SetMessages([]store.Message{msg})
	before := ml.View()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	ml.SetImage(99, img)
	after := ml.View()

	require.NotContains(t, after, "📷 photo", "placeholder should be gone after image loaded")
	require.Greater(t, len(after), len(before), "view should grow with actual art lines")
}

// An in-place edit can change a message's line count. When the viewport was at
// the natural bottom, it must stay pinned to the bottom (newest fully visible)
// rather than freezing the stale top anchor — otherwise the grown message is
// clipped and the view appears to jump. Mirrors SetImage's re-anchoring.
func TestMessageList_SetMessagesKeepScroll_AtBottom_ReanchorsOnHeightChange(t *testing.T) {
	ml := components.NewMessageList(9, 80)
	msgs := makeMessages(6) // each h=3; SetMessages pins to the natural bottom
	ml.SetMessages(msgs)

	// Edit the newest message to be taller (3 body lines → h=5).
	msgs[5].Text = "line1\nline2\nline3"
	ml.SetMessagesKeepScroll(msgs)

	// Expected: re-anchored to the new natural bottom, identical to a fresh load.
	exp := components.NewMessageList(9, 80)
	exp.SetMessages(msgs)
	assert.Equal(t, exp.ViewStart(), ml.ViewStart())
	assert.Equal(t, exp.LineOffset(), ml.LineOffset())
}

// When scrolled up in history (not at the bottom), an in-place edit must NOT move
// the viewport, even if the edited message changes height.
func TestMessageList_SetMessagesKeepScroll_ScrolledUp_KeepsPosition(t *testing.T) {
	ml := components.NewMessageList(9, 80)
	msgs := makeMessages(10) // each h=3
	ml.SetMessages(msgs)
	ml.ScrollUpBy(6)
	vs, lo := ml.ViewStart(), ml.LineOffset()
	require.False(t, ml.ViewStart() == 0 && ml.LineOffset() == 0, "precondition: not at top")

	// Edit the newest message to be taller; the top anchor must stay put.
	msgs[9].Text = "line1\nline2\nline3"
	ml.SetMessagesKeepScroll(msgs)
	assert.Equal(t, vs, ml.ViewStart())
	assert.Equal(t, lo, ml.LineOffset())
}

func TestMessageList_SelectedMessageID_EmptyList(t *testing.T) {
	ml := components.NewMessageList(10, 80)
	assert.Equal(t, 0, ml.SelectedMessageID())
}

func TestMessageList_SelectedMessageID_SingleMessage(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(1))
	assert.Equal(t, 1, ml.SelectedMessageID())
}

func TestMessageList_SelectedMessageID_AtBottom_SelectsLast(t *testing.T) {
	// 6 msgs × h=3 = 18 lines < viewHeight=20 → all visible, last selected
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(6))
	assert.Equal(t, 6, ml.SelectedMessageID())
}

func TestMessageList_SelectedMessageID_AfterScrollUp_SelectsPrevious(t *testing.T) {
	// viewHeight=3, 6 msgs each h=3 → viewStart=5 initially → msg6 selected.
	// After one ScrollUp: viewStart=4, lineOffset=1.
	//   msg5 (i=4): firstContentVP = 0 + (1-1) = 0 < 3 → selected
	//   msg6 (i=5): linesUsed after msg5 slice = 2; firstContentVP = 2+1=3, not < 3
	// → selectedID = 5
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6))
	assert.Equal(t, 6, ml.SelectedMessageID())
	ml.ScrollUp()
	assert.Equal(t, 5, ml.SelectedMessageID())
}

func TestMessageList_SelectedMessageID_FirstContentCutOff_SelectsNext(t *testing.T) {
	// viewHeight=4, 6 msgs each h=3.
	// positionAtBottom: i=5 lineCount=3, i=4 → 3+3=6>=4 → overflow=2 → (viewStart=4, lineOffset=2).
	// msg5 (i=4): firstContentVP = 0+(1-2) = -1 → not selected.
	// msg6 (i=5): linesUsed after msg5 slice (1 line) = 1; firstContentVP=1+1=2 < 4 → selected.
	ml := components.NewMessageList(4, 40)
	ml.SetMessages(makeMessages(6))
	assert.Equal(t, 6, ml.SelectedMessageID())
}

func TestMessageList_Indicator_Incoming_ShowsBar(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetShowIndicator(true)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now()}})
	plain := stripANSI(ml.View())
	assert.Contains(t, plain, "┃")
}

func TestMessageList_Indicator_Outgoing_ShowsBar(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetShowIndicator(true)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now(), IsOut: true}})
	plain := stripANSI(ml.View())
	assert.Contains(t, plain, "┃")
}

func TestMessageList_Indicator_HiddenWhenShowIndicatorFalse(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetShowIndicator(false)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hello", Date: time.Now()}})
	plain := stripANSI(ml.View())
	assert.NotContains(t, plain, "┃")
}

func TestMessageList_Indicator_SpansAllContentLines(t *testing.T) {
	// multiline message: bar should appear on every content line, not just the first
	ml := components.NewMessageList(20, 80)
	ml.SetShowIndicator(true)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "line1\nline2\nline3", Date: time.Now()}})
	plain := stripANSI(ml.View())
	assert.Equal(t, 3, strings.Count(plain, "┃"))
}

func TestMessageList_Indicator_OnlyOnSelectedMessage(t *testing.T) {
	// 3 messages all visible; only the selected one gets the bar
	ml := components.NewMessageList(20, 80)
	ml.SetShowIndicator(true)
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "a", Date: time.Now()},
		{ID: 2, ChatID: 1, Text: "b", Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "c", Date: time.Now()},
	})
	plain := stripANSI(ml.View())
	// each unselected message has 1 content line, selected has 1 → only 1 bar total
	assert.Equal(t, 1, strings.Count(plain, "┃"))
}

func TestMessageList_SelectedMessageIsOut_Outgoing(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", IsOut: true, Date: time.Now()}})
	assert.True(t, ml.SelectedMessageIsOut())
}

func TestMessageList_SelectedMessageIsOut_Incoming(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, ChatID: 1, Text: "hi", IsOut: false, Date: time.Now()}})
	assert.False(t, ml.SelectedMessageIsOut())
}

func TestMessageList_SelectedMessageIsOut_NoMessages(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	assert.False(t, ml.SelectedMessageIsOut())
}

func TestMessageList_MsgHeight_ReplyFound_ShowsGlyph(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	orig := store.Message{ID: 1, ChatID: 1, Text: "original text", Date: time.Now()}
	reply := store.Message{ID: 2, ChatID: 1, Text: "reply", Date: time.Now(), ReplyToMsgID: 1}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "▌")
}

func TestMessageList_MsgHeight_ReplyNotFound_ShowsPlaceholder(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	reply := store.Message{ID: 2, ChatID: 1, Text: "reply", Date: time.Now(), ReplyToMsgID: 99}
	ml.SetMessages([]store.Message{reply})
	view := ml.View()
	assert.Contains(t, stripANSI(view), "Original not available")
}

func TestMessageList_View_ReplyToSelfNotInBuffer_ShowsPlaceholder(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	reply := store.Message{ID: 5, ChatID: 1, Text: "hi", Date: time.Now(), ReplyToMsgID: 1}
	ml.SetMessages([]store.Message{reply})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "Original not available")
	assert.NotContains(t, view, "▌ ?")
}

func TestMessageList_View_ReplyShowsQuoteBlock(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	orig := store.Message{
		ID:         1,
		ChatID:     1,
		SenderName: "Alice",
		Text:       "Sure, let me check that",
		Date:       time.Now(),
	}
	reply := store.Message{
		ID:           2,
		ChatID:       1,
		SenderName:   "Bob",
		Text:         "Actually it was Tuesday",
		Date:         time.Now(),
		ReplyToMsgID: 1,
	}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "▌")
	assert.Contains(t, view, "Alice")
	assert.Contains(t, view, "Sure, let me check that")
	assert.Contains(t, view, "Actually it was Tuesday")
}

func TestMessageList_View_ReplySnippetTruncated(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	longText := strings.Repeat("x", 200)
	orig := store.Message{ID: 1, ChatID: 1, SenderName: "A", Text: longText, Date: time.Now()}
	reply := store.Message{ID: 2, ChatID: 1, Text: "ok", Date: time.Now(), ReplyToMsgID: 1}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "▌")
	assert.Contains(t, view, "…")
}

func TestMessageList_GroupChat_LongSenderName_NoBubbleOverflow(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetIsGroup(true)
	msg := store.Message{
		ID:         1,
		ChatID:     1,
		SenderName: "VeryLongSenderNameThatExceedsText",
		Text:       "ok",
		Date:       time.Now(),
	}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	// The top border must contain the sender name and end with the corner glyph.
	var topLine string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "VeryLongSenderNameThatExceedsText") {
			topLine = l
			break
		}
	}
	require.NotEmpty(t, topLine, "sender name not found in view")
	assert.True(t, strings.HasSuffix(strings.TrimRight(topLine, " "), "╮"), "top border must end with ╮, got: %q", topLine)
}

func TestMessageList_ScrollToMessage_Found(t *testing.T) {
	// viewHeight=6: 2 msgs of h=3 fit. Scrolling to msg2 (index 2) leaves
	// 4 msgs below (12 lines > 6), so positionAtBottom is NOT triggered.
	ml := components.NewMessageList(6, 80)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	found := ml.ScrollToMessage(2)
	assert.True(t, found)
	assert.Equal(t, 2, ml.ViewStart()) // index 2 = msg2
	assert.Equal(t, 0, ml.LineOffset())
}

func TestMessageList_ScrollToMessage_FewMsgsBelow_AnchorToBottom(t *testing.T) {
	// When messages from target to end don't fill viewport, positionAtBottom is called.
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5)) // IDs 1..5; 5×h=3=15 < viewHeight=20
	found := ml.ScrollToMessage(4)
	assert.True(t, found)
	// positionAtBottom: all 5 msgs fit → viewStart=0
	view := stripANSI(ml.View())
	assert.Contains(t, view, "msg 4") // target visible
	assert.Contains(t, view, "msg 1") // context above also visible
}

func TestMessageList_ScrollToMessage_NotFound(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(5))
	before := ml.ViewStart()
	found := ml.ScrollToMessage(99)
	assert.False(t, found)
	assert.Equal(t, before, ml.ViewStart())
}

func TestMessageList_ScrollToMessage_Empty(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	found := ml.ScrollToMessage(1)
	assert.False(t, found)
}

func TestMessageList_SelectedMessageReplyToMsgID_IsReply(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	msg := store.Message{ID: 1, ChatID: 1, Text: "hi", Date: time.Now(), ReplyToMsgID: 42}
	ml.SetMessages([]store.Message{msg})
	assert.Equal(t, 42, ml.SelectedMessageReplyToMsgID())
}

func TestMessageList_SelectedMessageReplyToMsgID_NotReply(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages(makeMessages(1))
	assert.Equal(t, 0, ml.SelectedMessageReplyToMsgID())
}

func TestMessageList_SelectedMessageReplyToMsgID_Empty(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	assert.Equal(t, 0, ml.SelectedMessageReplyToMsgID())
}

func TestMessageList_SetImage_AtBottom_ReanchorsToBottom(t *testing.T) {
	// 3 msgs: text, photo (placeholder), text. All fit in viewport (9 < 10).
	// After image loads the photo message expands to ~30 lines; the newest message
	// must remain visible (re-anchored to new natural bottom).
	ml := components.NewMessageList(10, 80)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "msg 1", Date: time.Now()},
		{ID: 2, ChatID: 1, Media: &store.MediaRef{Kind: store.MediaPhoto}, Photo: &store.PhotoRef{ID: 42}, Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "msg 3", Date: time.Now()},
	}
	ml.SetMessages(msgs)

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	ml.SetImage(42, img)

	assert.Contains(t, stripANSI(ml.View()), "msg 3", "newest message must remain visible after image expands")
}

func TestMessageList_SetKnownImages_AtBottom_ReanchorsToBottom(t *testing.T) {
	// Same as SetImage case but via bulk load.
	ml := components.NewMessageList(10, 80)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "msg 1", Date: time.Now()},
		{ID: 2, ChatID: 1, Media: &store.MediaRef{Kind: store.MediaPhoto}, Photo: &store.PhotoRef{ID: 42}, Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "msg 3", Date: time.Now()},
	}
	ml.SetMessages(msgs)

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	ml.SetKnownImages(map[int64]image.Image{42: img})

	assert.Contains(t, stripANSI(ml.View()), "msg 3", "newest message must remain visible after bulk image load")
}

func TestMessageList_SetImage_ScrolledUp_DoesNotReanchor(t *testing.T) {
	// When user has scrolled up, SetImage must not snap back to bottom.
	ml := components.NewMessageList(9, 80)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "msg 1", Date: time.Now()},
		{ID: 2, ChatID: 1, Text: "msg 2", Date: time.Now()},
		{ID: 3, ChatID: 1, Text: "msg 3", Date: time.Now()},
		{ID: 4, ChatID: 1, Text: "msg 4", Date: time.Now()},
		{ID: 5, ChatID: 1, Media: &store.MediaRef{Kind: store.MediaPhoto}, Photo: &store.PhotoRef{ID: 42}, Date: time.Now()},
	}
	ml.SetMessages(msgs)
	ml.ScrollUp()

	beforeStart := ml.ViewStart()
	beforeOff := ml.LineOffset()

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	ml.SetImage(42, img)

	assert.Equal(t, beforeStart, ml.ViewStart(), "ViewStart must not change after SetImage when scrolled up")
	assert.Equal(t, beforeOff, ml.LineOffset(), "LineOffset must not change after SetImage when scrolled up")
}

func TestMessageList_View_ReplySnippetFirstLineOnly(t *testing.T) {
	// viewHeight=5 fits only the reply bubble (5 lines), hiding orig from the viewport.
	ml := components.NewMessageList(5, 80)
	orig := store.Message{ID: 1, ChatID: 1, SenderName: "Alice", Text: "line1\nline2\nline3", Date: time.Now()}
	reply := store.Message{ID: 2, ChatID: 1, Text: "ok", Date: time.Now(), ReplyToMsgID: 1}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "line1")
	assert.NotContains(t, view, "line2")
}

func TestMessageList_EditedMessage_ShowsEditedLabel(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "hello", Date: now, IsOut: true, EditDate: &now},
	})
	assert.Contains(t, ml.View(), "edited")
}

func TestMessageList_NotEdited_NoEditedLabel(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "normal", Date: now, IsOut: true},
	})
	assert.NotContains(t, ml.View(), "edited")
}

func TestMessageList_DateSeparator_FirstMessageHasSeparator(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	msg := store.Message{ID: 1, ChatID: 1, Text: "hi", Date: time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "May 18")
}

func TestMessageList_DateSeparator_AppearsOnDayBoundary(t *testing.T) {
	ml := components.NewMessageList(40, 40)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "yesterday", Date: time.Date(2026, 5, 17, 23, 0, 0, 0, time.UTC)},
		{ID: 2, ChatID: 1, Text: "today", Date: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)},
	}
	ml.SetMessages(msgs)
	view := stripANSI(ml.View())
	assert.Contains(t, view, "May 17")
	assert.Contains(t, view, "May 18")
}

func TestMessageList_DateSeparator_SameDayNoExtra(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "a", Date: time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)},
		{ID: 2, ChatID: 1, Text: "b", Date: time.Date(2026, 5, 18, 10, 0, 0, 0, time.UTC)},
	}
	ml.SetMessages(msgs)
	view := stripANSI(ml.View())
	assert.Equal(t, 1, strings.Count(view, "May 18"), "same-day messages must share a single separator")
}

func TestMessageList_DateSeparator_TodayLabel(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	now := time.Now()
	msg := store.Message{ID: 1, ChatID: 1, Text: "hi", Date: now}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "Today")
}

func TestMessageList_DateSeparator_CurrentYearNotTodayShowsDate(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	yesterday := time.Now().AddDate(0, 0, -1)
	msg := store.Message{ID: 1, ChatID: 1, Text: "hi", Date: yesterday}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	assert.Contains(t, view, yesterday.Format("January 2"))
	assert.NotContains(t, view, "Today")
}

func TestMessageList_DateSeparator_PreviousYearWithYear(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	msg := store.Message{ID: 1, ChatID: 1, Text: "hi", Date: time.Date(2025, 3, 7, 12, 0, 0, 0, time.UTC)}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	assert.Contains(t, view, "March 7, 2025")
}

func TestMessageList_ReactionsRenderedOnBottomBorder(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "hello", Date: now, IsOut: false,
			Reactions: []store.Reaction{
				{Emoji: "❤️", Count: 3, IsChosen: false},
				{Emoji: "👍", Count: 1, IsChosen: false},
			}},
	})
	v := ml.View()
	assert.Contains(t, v, "❤️")
	assert.Contains(t, v, "3")
	assert.Contains(t, v, "·")
	assert.Contains(t, v, "👍")
}

func TestMessageList_NoReactions_NoSeparator(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "hello", Date: now, IsOut: false},
	})
	v := ml.View()
	assert.NotContains(t, v, "·")
}

func TestMessageList_ReplyBubble_NameFitsWidth(t *testing.T) {
	ml := components.NewMessageList(40, 80)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, SenderName: "Aleksandra Petrovna", Text: "hi", Date: now},
		{ID: 2, ChatID: 1, Text: "ok", ReplyToMsgID: 1, Date: now},
	}
	ml.SetMessages(msgs)
	view := ml.View()

	lines := strings.Split(view, "\n")

	nameIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "Aleksandra Petrovna") {
			nameIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, nameIdx, 1, "name preview line not found")

	topBorder := lines[nameIdx-1]
	nameLine := lines[nameIdx]

	assert.Equal(t, lipgloss.Width(topBorder), lipgloss.Width(nameLine),
		"name line must not overflow bubble border")
}

func TestMessageList_ReplyBubble_LongNameTruncated(t *testing.T) {
	const longName = "Александра Александровна Петровна Захаренко"
	ml := components.NewMessageList(40, 40)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, SenderName: longName, Text: "hi", Date: now},
		{ID: 2, ChatID: 1, Text: "ok", ReplyToMsgID: 1, Date: now},
	}
	ml.SetMessages(msgs)
	view := ml.View()

	for _, l := range strings.Split(view, "\n") {
		if strings.ContainsAny(l, "╭╰│") {
			assert.LessOrEqual(t, lipgloss.Width(l), 30,
				"bubble line exceeds maxBubbleW: %q", l)
		}
	}
	assert.Contains(t, view, "…")
	assert.NotContains(t, view, longName)
}

func TestMessageList_ForwardBubble_ShowsLabelAndName(t *testing.T) {
	ml := components.NewMessageList(40, 80)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "hey check this", Date: now,
			Forward: &store.ForwardInfo{From: "Bob Smith"}},
	}
	ml.SetMessages(msgs)
	view := ml.View()

	assert.Contains(t, view, "Forwarded from")
	assert.Contains(t, view, "Bob Smith")
}

func TestMessageList_ForwardBubble_HiddenSender(t *testing.T) {
	ml := components.NewMessageList(40, 80)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "hey", Date: now,
			Forward: &store.ForwardInfo{From: ""}},
	}
	ml.SetMessages(msgs)
	view := ml.View()

	assert.Contains(t, view, "Forwarded from")
	assert.Contains(t, view, "Hidden")
}

func TestMessageList_ForwardBubble_LongNameNoOverflow(t *testing.T) {
	const longName = "Александр Александрович Длинноимённый Захаренко"
	ml := components.NewMessageList(40, 40)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "ok", Date: now,
			Forward: &store.ForwardInfo{From: longName}},
	}
	ml.SetMessages(msgs)
	view := ml.View()

	for _, l := range strings.Split(view, "\n") {
		if strings.ContainsAny(l, "╭╰│") {
			assert.LessOrEqual(t, lipgloss.Width(l), 30,
				"bubble line exceeds maxBubbleW: %q", l)
		}
	}
	assert.Contains(t, view, "…")
	assert.NotContains(t, view, longName)
}

func TestMessageList_ForwardBubble_HasBlankLineSeparator(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "forwarded body", Date: now,
			Forward: &store.ForwardInfo{From: "Bob Smith"}},
	}
	ml.SetMessages(msgs)
	lines := strings.Split(stripANSI(ml.View()), "\n")

	nameIdx, textIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "Bob Smith") && nameIdx == -1 {
			nameIdx = i
		}
		if strings.Contains(l, "forwarded body") {
			textIdx = i
		}
	}
	require.Greater(t, nameIdx, 0, "forward name line not found")
	require.Greater(t, textIdx, nameIdx, "message body must come after forward header")

	// Without separator: gap=1 (name immediately followed by body).
	// With separator: gap=2 (name + blank + body).
	assert.Greater(t, textIdx-nameIdx, 1,
		"expected blank separator line between forward header and message body")
}

func TestMessageList_ForwardBubble_NoBlankLineWhenEmpty(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Date: now, Forward: &store.ForwardInfo{From: "Bob Smith"}},
	}
	ml.SetMessages(msgs)
	lines := strings.Split(stripANSI(ml.View()), "\n")

	// Header-only forward: 2 border lines + label + name = 4 bubble lines, no
	// trailing blank separator.
	bubbleLines := 0
	for _, l := range lines {
		if strings.ContainsAny(l, "╭╰│") {
			bubbleLines++
		}
	}
	assert.Equal(t, 4, bubbleLines,
		"header-only forward should not append a blank separator line")
}

func TestMessageList_ReplyBubble_WidensToShowSnippet(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	const origText = "This is a fairly long original message worth reading"
	orig := store.Message{ID: 1, ChatID: 1, SenderName: "Alice", Text: origText, Date: now}
	// A short reply must not squeeze the quoted original down to nothing.
	reply := store.Message{ID: 2, ChatID: 1, Text: "ok", ReplyToMsgID: 1, Date: now}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())

	assert.Contains(t, view, origText,
		"reply bubble should widen to show the full original snippet")
	assert.NotContains(t, view, "…",
		"original snippet should fit without truncation at this width")
}

func TestMessageList_NoForward_NoForwardedLabel(t *testing.T) {
	ml := components.NewMessageList(40, 80)
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "plain", Date: time.Now()},
	}
	ml.SetMessages(msgs)
	assert.NotContains(t, ml.View(), "Forwarded from")
}

func TestMessageList_ReplyBubble_NilOrig_ShowsPlaceholder(t *testing.T) {
	ml := components.NewMessageList(40, 80)
	msgs := []store.Message{
		{ID: 2, ChatID: 1, Text: "ok", ReplyToMsgID: 999, Date: time.Now()},
	}
	ml.SetMessages(msgs)
	require.NotPanics(t, func() { ml.View() })
	assert.Contains(t, ml.View(), "Original not available")
}

func TestMessageList_View_PhotoTextHasBlankLineSeparator(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	msg := store.Message{
		ID:     1,
		ChatID: 1,
		Media:  &store.MediaRef{Kind: store.MediaPhoto},
		Photo:  &store.PhotoRef{ID: 77},
		Text:   "caption text",
		Date:   time.Now(),
	}
	ml.SetMessages([]store.Message{msg})
	view := stripANSI(ml.View())
	lines := strings.Split(view, "\n")

	photoIdx, textIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "photo") && photoIdx == -1 {
			photoIdx = i
		}
		if strings.Contains(l, "caption text") {
			textIdx = i
		}
	}
	require.Greater(t, photoIdx, 0, "photo placeholder line not found")
	require.Greater(t, textIdx, photoIdx, "caption text must come after photo")

	// Without blank separator: gap=1 (photo immediately followed by text).
	// With blank separator: gap=2 (photo + blank + text).
	assert.Greater(t, textIdx-photoIdx, 1,
		"expected blank separator line between photo and caption text")
}

func TestMessageList_View_ReplyHasBlankLineSeparator(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	now := time.Now()
	orig := store.Message{ID: 1, ChatID: 1, SenderName: "Alice", Text: "original text", Date: now}
	reply := store.Message{ID: 2, ChatID: 1, Text: "reply body", ReplyToMsgID: 1, Date: now}
	ml.SetMessages([]store.Message{orig, reply})
	view := stripANSI(ml.View())
	lines := strings.Split(view, "\n")

	// "Alice" appears only in the reply preview (non-group chat, no border name).
	// "reply body" is the message text.
	nameIdx, textIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "Alice") && nameIdx == -1 {
			nameIdx = i
		}
		if strings.Contains(l, "reply body") {
			textIdx = i
		}
	}
	require.Greater(t, nameIdx, 0, "name preview line not found")
	require.Greater(t, textIdx, nameIdx, "reply body must come after name preview")

	// Without blank separator: gap=2 (name + snippet).
	// With blank separator: gap=3 (name + snippet + blank).
	assert.Greater(t, textIdx-nameIdx, 2,
		"expected blank separator line between reply preview and message body")
}

// ansiOpenBeforeName returns the last ANSI escape sequence opening (e.g. "\x1b[1;32m")
// that appears immediately before name in rawLine. Returns "" if not found.
func ansiOpenBeforeName(rawLine, name string) string {
	pos := strings.Index(rawLine, name)
	if pos < 0 {
		return ""
	}
	prefix := rawLine[:pos]
	lastEsc := strings.LastIndex(prefix, "\x1b[")
	if lastEsc < 0 {
		return ""
	}
	return prefix[lastEsc:]
}

func TestMessageList_GroupChat_SenderColors_DifferentIDs(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetIsGroup(true)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, SenderID: 0, SenderName: "Alice", Text: "msg1", Date: now},
		{ID: 2, ChatID: 1, SenderID: 1, SenderName: "Bob", Text: "msg2", Date: now},
	})
	raw := ml.View()

	var aliceLine, bobLine string
	for _, l := range strings.Split(raw, "\n") {
		plain := stripANSI(l)
		if strings.Contains(plain, "Alice") {
			aliceLine = l
		}
		if strings.Contains(plain, "Bob") {
			bobLine = l
		}
	}
	require.NotEmpty(t, aliceLine, "Alice not found in view")
	require.NotEmpty(t, bobLine, "Bob not found in view")

	aliceANSI := ansiOpenBeforeName(aliceLine, "Alice")
	bobANSI := ansiOpenBeforeName(bobLine, "Bob")
	require.NotEmpty(t, aliceANSI, "Alice has no ANSI color before name")
	require.NotEmpty(t, bobANSI, "Bob has no ANSI color before name")
	assert.NotEqual(t, aliceANSI, bobANSI, "different senders must render different colors")
}

func TestMessageList_GroupChat_SenderColors_SameID(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetIsGroup(true)
	now := time.Now()
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, SenderID: 42, SenderName: "Alice", Text: "first", Date: now},
		{ID: 2, ChatID: 1, SenderID: 42, SenderName: "Alice", Text: "second", Date: now},
	})
	raw := ml.View()

	var lines []string
	for _, l := range strings.Split(raw, "\n") {
		if strings.Contains(stripANSI(l), "Alice") {
			lines = append(lines, l)
		}
	}
	require.Len(t, lines, 2, "expected two lines containing Alice")
	assert.Equal(t,
		ansiOpenBeforeName(lines[0], "Alice"),
		ansiOpenBeforeName(lines[1], "Alice"),
		"same sender must render same color in both bubbles")
}

func TestMessageList_ReplyPreview_GlyphAndName_SenderColor(t *testing.T) {
	now := time.Now()
	ml := components.NewMessageList(40, 80)
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, SenderID: 5, SenderName: "Carol", Text: "original", Date: now},
		{ID: 2, ChatID: 1, Text: "reply", ReplyToMsgID: 1, Date: now},
	})
	raw := ml.View()

	// Find the reply preview name row: plain text contains both ▌ and Carol
	var glyphNameLine string
	for _, l := range strings.Split(raw, "\n") {
		plain := stripANSI(l)
		if strings.Contains(plain, "▌") && strings.Contains(plain, "Carol") {
			glyphNameLine = l
			break
		}
	}
	require.NotEmpty(t, glyphNameLine, "reply preview name row not found")

	// ▌ must be immediately preceded by an ANSI escape (i.e., the byte before ▌ is 'm')
	glyphPos := strings.Index(glyphNameLine, "▌")
	require.Positive(t, glyphPos)
	assert.Equal(t, byte('m'), glyphNameLine[glyphPos-1],
		"▌ glyph must be preceded by an ANSI color escape (sender-colored)")
}

func TestMessageList_SenderColor_DarkVsLight(t *testing.T) {
	now := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, SenderID: 0, SenderName: "Alice", Text: "hi", Date: now},
	}

	mlDark := components.NewMessageList(20, 80)
	mlDark.SetIsGroup(true)
	mlDark.SetDarkBackground(true)
	mlDark.SetMessages(msgs)

	mlLight := components.NewMessageList(20, 80)
	mlLight.SetIsGroup(true)
	mlLight.SetDarkBackground(false)
	mlLight.SetMessages(msgs)

	darkLine := ""
	for _, l := range strings.Split(mlDark.View(), "\n") {
		if strings.Contains(stripANSI(l), "Alice") {
			darkLine = l
			break
		}
	}
	lightLine := ""
	for _, l := range strings.Split(mlLight.View(), "\n") {
		if strings.Contains(stripANSI(l), "Alice") {
			lightLine = l
			break
		}
	}
	require.NotEmpty(t, darkLine)
	require.NotEmpty(t, lightLine)
	assert.NotEqual(t,
		ansiOpenBeforeName(darkLine, "Alice"),
		ansiOpenBeforeName(lightLine, "Alice"),
		"dark and light backgrounds must use different colors for the same sender")
}

func TestMessageList_ReplyPreview_CJKSenderNameTruncated(t *testing.T) {
	// 30 CJK chars = 60 visual cols, far wider than any bubble content width.
	// Rune-count (30) is below the budget computed from actualW (e.g. ~20), so
	// the current rune-based code does NOT truncate. runewidth-based code must.
	now := time.Now()
	ml := components.NewMessageList(40, 80)
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, SenderID: 5, SenderName: strings.Repeat("中", 30), Text: "original", Date: now},
		{ID: 2, ChatID: 1, Text: "reply", ReplyToMsgID: 1, Date: now},
	})
	raw := ml.View()

	var previewLine string
	for _, l := range strings.Split(raw, "\n") {
		plain := stripANSI(l)
		if strings.Contains(plain, "▌") && strings.Contains(plain, "中") {
			previewLine = plain
			break
		}
	}
	require.NotEmpty(t, previewLine, "reply preview name row not found")
	assert.Contains(t, previewLine, "…")
}

func TestMessageList_NoUnreadSeparator_WhenReadMaxIDZero(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(6))
	view := ml.View()
	assert.NotContains(t, view, "New Messages")
}

func TestMessageList_UnreadSeparator_AppearsAfterSetInboxReadMaxID(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetInboxReadMaxID(3)
	ml.SetMessages(makeMessages(6)) // IDs 1-6, same day
	view := ml.View()
	assert.Contains(t, view, "New Messages")
}

func TestMessageList_UnreadSeparator_NotCountedAsMessage(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetInboxReadMaxID(3)
	ml.SetMessages(makeMessages(6))
	assert.Equal(t, 6, ml.Count())
}

func TestMessageList_NoUnreadSeparator_WhenAllMessagesRead(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetInboxReadMaxID(100)       // higher than all message IDs
	ml.SetMessages(makeMessages(6)) // IDs 1-6
	view := ml.View()
	assert.NotContains(t, view, "New Messages")
}

func TestMessageList_NoUnreadSeparator_BeforeOwnOutgoingMessage(t *testing.T) {
	// Sending an outgoing message (ID > inboxReadMaxID) must not anchor the
	// "New Messages" divider — it only marks the first incoming unread message.
	now := time.Now()
	ml := components.NewMessageList(20, 40)
	ml.SetInboxReadMaxID(3)
	ml.SetMessages([]store.Message{
		{ID: 1, ChatID: 1, Text: "read", Date: now},
		{ID: 2, ChatID: 1, Text: "read", Date: now},
		{ID: 3, ChatID: 1, Text: "read", Date: now},
		{ID: 4, ChatID: 1, Text: "my reply", Date: now, IsOut: true},
	})
	view := ml.View()
	assert.NotContains(t, view, "New Messages")
}

func TestMessageList_UnreadSeparator_AnchorsToFirstIncoming_SkippingOutgoing(t *testing.T) {
	// An outgoing message before the first incoming unread one must be skipped:
	// the divider anchors to the incoming message, not the outgoing one.
	now := time.Now()
	ml := components.NewMessageList(20, 40)
	ml.SetInboxReadMaxID(3)
	ml.SetMessages([]store.Message{
		{ID: 3, ChatID: 1, Text: "read", Date: now},
		{ID: 4, ChatID: 1, Text: "my reply", Date: now, IsOut: true},
		{ID: 5, ChatID: 1, Text: "incoming unread", Date: now},
	})
	view := ml.View()
	require.Contains(t, view, "New Messages")
	sepIdx := strings.Index(view, "New Messages")
	outIdx := strings.Index(view, "my reply")
	incIdx := strings.Index(view, "incoming unread")
	assert.Less(t, outIdx, sepIdx, "outgoing message must render above the divider")
	assert.Less(t, sepIdx, incIdx, "divider must render above the first incoming unread message")
}

func TestMessageList_UnreadSeparator_VisibleWhenManyUnread(t *testing.T) {
	// When unread messages exceed the viewport height, ScrollToFirstUnread must
	// include the separator at the top rather than hiding it above the boundary.
	ml := components.NewMessageList(6, 40) // small viewport: fits ~2 messages
	ml.SetInboxReadMaxID(2)
	ml.SetMessages(makeMessages(10)) // IDs 1-10; sep before msg 3, msgs 3-10 are unread
	ml.ScrollToFirstUnread(2)
	view := ml.View()
	assert.Contains(t, view, "New Messages")
}

func TestMessageList_UnreadSepAppearsAfterDateSep_WhenFirstUnreadStartsNewDay(t *testing.T) {
	ml := components.NewMessageList(30, 40)
	yesterday := time.Now().Add(-24 * time.Hour)
	today := time.Now()
	msgs := []store.Message{
		{ID: 1, ChatID: 1, Text: "read msg", Date: yesterday},
		{ID: 2, ChatID: 1, Text: "unread msg", Date: today},
	}
	ml.SetInboxReadMaxID(1)
	ml.SetMessages(msgs)
	view := ml.View()
	require.Contains(t, view, "New Messages")
	require.Contains(t, view, "Today")
	todayIdx := strings.Index(view, "Today")
	unreadIdx := strings.Index(view, "New Messages")
	assert.Less(t, todayIdx, unreadIdx, "date separator should appear before unread separator")
}

func TestMessageList_MediaPlaceholders(t *testing.T) {
	cases := []struct {
		kind store.MediaKind
		want string
	}{
		{store.MediaPhoto, "📷 photo"},
		{store.MediaVideo, "🎥 video"},
		{store.MediaVideoNote, "⭕ video note"},
		{store.MediaVoice, "🎤 voice"},
		{store.MediaAudio, "🎵 audio"},
		{store.MediaGIF, "🎞 GIF"},
		{store.MediaFile, "📎 file"},
		{store.MediaLocation, "📍 location"},
		{store.MediaOther, "📦 media"},
	}
	for _, tc := range cases {
		ml := components.NewMessageList(20, 80)
		ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{Kind: tc.kind}}})
		assert.Contains(t, ml.View(), tc.want)
	}
}

func TestMessageList_MediaOnlyBubble_BordersAligned(t *testing.T) {
	// A media-only message (no text/reactions) must still size its bubble to the
	// placeholder label, so every rendered bubble line has the same width.
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{Kind: store.MediaVoice}}})
	var widths []int
	for _, line := range strings.Split(strings.TrimRight(stripANSI(ml.View()), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		widths = append(widths, lipgloss.Width(line))
	}
	require.NotEmpty(t, widths)
	for _, w := range widths {
		assert.Equal(t, widths[0], w, "all bubble lines must share the same width")
	}
}

func TestMessageList_StickerPlaceholder_UsesAltEmoji(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{Kind: store.MediaSticker, Emoji: "🐱"}}})
	assert.Contains(t, ml.View(), "🐱 sticker")
}

func TestMessageList_StickerPlaceholder_NoEmojiFallback(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{Kind: store.MediaSticker}}})
	assert.Contains(t, ml.View(), "sticker")
}

func TestMessageList_VoiceWaveform(t *testing.T) {
	// Waveform packing samples [31,1,31] -> LE {0x1F, 0x7C}; renders block bars.
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{
		Kind: store.MediaVoice, Duration: 15, Waveform: []byte{0x1F, 0x7C},
	}}})
	view := ml.View()
	assert.Contains(t, view, "🎤")
	assert.Contains(t, view, "█")
	assert.Contains(t, view, "0:15")
}

func TestMessageList_VideoPlaceholder_ShowsDuration(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1,
		Media:    &store.MediaRef{Kind: store.MediaVideo, Duration: 42},
		Document: &store.DocumentRef{ID: 99, ThumbSize: "m"},
	}})
	assert.Contains(t, ml.View(), "🎥 video 0:42")
}

func TestMessageList_VideoThumbnail_ShowsPlayOverlay(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1,
		Media:    &store.MediaRef{Kind: store.MediaVideo, Duration: 42},
		Document: &store.DocumentRef{ID: 99, ThumbSize: "m"},
	}})
	// Inject the downloaded thumbnail under the document id.
	ml.SetImage(99, image.NewRGBA(image.Rect(0, 0, 16, 12)))
	view := ml.View()
	assert.Contains(t, view, "▶ 0:42", "play affordance + duration over the preview")
	assert.NotContains(t, view, "🎥 video", "text placeholder is replaced by the thumbnail")
}

func TestMessageList_VoicePlayback_ShowsLivePosition(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1,
		Media:    &store.MediaRef{Kind: store.MediaVoice, Duration: 15, Waveform: []byte{0x1F, 0x7C}},
		Document: &store.DocumentRef{ID: 55},
	}})
	assert.Contains(t, ml.View(), "0:15", "total duration before playback")

	ml.SetVoicePlayback(55, 0.5, 7)
	assert.Contains(t, ml.View(), "0:07", "live position while the voice plays")
}

func TestMessageList_VideoNoteThumbnail_ShowsPlayOverlay(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1,
		Media:    &store.MediaRef{Kind: store.MediaVideoNote, Duration: 8},
		Document: &store.DocumentRef{ID: 77, ThumbSize: "m"},
	}})
	ml.SetImage(77, image.NewRGBA(image.Rect(0, 0, 12, 12)))
	view := ml.View()
	assert.Contains(t, view, "▶ 0:08", "round video shows play affordance + duration")
	assert.NotContains(t, view, "⭕ video note", "text placeholder replaced by the thumbnail")
}

func TestMessageList_AudioMetadata(t *testing.T) {
	ml := components.NewMessageList(20, 80)
	ml.SetMessages([]store.Message{{ID: 1, Media: &store.MediaRef{
		Kind: store.MediaAudio, Duration: 200, Title: "Song", Performer: "Artist",
	}}})
	view := ml.View()
	assert.Contains(t, view, "Song")
	assert.Contains(t, view, "Artist")
	assert.Contains(t, view, "3:20")
}

func TestMessageList_ScrollInfo_TopAndBottom(t *testing.T) {
	ml := components.NewMessageList(5, 40) // viewHeight 5
	msgs := make([]store.Message, 0, 30)
	for i := 1; i <= 30; i++ {
		msgs = append(msgs, store.Message{ID: i, Text: "line"})
	}
	ml.SetMessages(msgs) // anchors at bottom

	bottom := ml.ScrollInfo()
	assert.Equal(t, 5, bottom.Visible)
	assert.Greater(t, bottom.Total, bottom.Visible, "content overflows")
	assert.Equal(t, bottom.Total-bottom.Visible, bottom.Offset, "anchored at bottom")

	ml.ScrollToTop()
	top := ml.ScrollInfo()
	assert.Equal(t, 0, top.Offset, "scrolled to top")
	assert.Equal(t, bottom.Total, top.Total, "total unchanged by scrolling")
}
