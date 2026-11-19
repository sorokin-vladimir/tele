package components_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func selectedIDs(ml *components.MessageList) []int {
	var ids []int
	for _, m := range ml.VisualSelection() {
		ids = append(ids, m.ID)
	}
	return ids
}

func TestVisual_RangeCoversAnchorToCursorOldestFirst(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	assert.True(t, ml.InVisual())
	assert.Equal(t, []int{3, 4, 5}, selectedIDs(ml))
	assert.False(t, ml.VisualHasGaps())
}

func TestVisual_RangeShrinksWhenTheCursorComesBack(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	ml.CursorDown()
	assert.Equal(t, []int{4, 5}, selectedIDs(ml))
}

func TestVisual_RangeFollowsTheCursorPastTheAnchor(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	ml.CursorUp()
	ml.CursorUp()
	require.True(t, ml.StartVisualRange())
	ml.CursorDown()
	ml.CursorDown()
	assert.Equal(t, []int{3, 4, 5}, selectedIDs(ml))
}

func TestVisual_MarkStartsWithTheFocusedMessageOnly(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualMark())
	ml.CursorUp()
	ml.CursorUp()
	assert.Equal(t, []int{5}, selectedIDs(ml))
}

func TestVisual_MarkAddsAndRemovesTheFocusedMessage(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualMark())
	ml.CursorUp()
	ml.CursorUp()
	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{3, 5}, selectedIDs(ml))
	assert.True(t, ml.VisualHasGaps())

	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{5}, selectedIDs(ml))
	assert.False(t, ml.VisualHasGaps())
}

func TestVisual_MarkFixesTheRangeWithTheFocusedMessageIn(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{3, 4, 5}, selectedIDs(ml))

	ml.CursorUp()
	ml.CursorUp()
	assert.Equal(t, []int{3, 4, 5}, selectedIDs(ml), "after the range is fixed the cursor only moves")

	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{1, 3, 4, 5}, selectedIDs(ml))
	ml.CursorDown()
	ml.CursorDown()
	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{1, 4, 5}, selectedIDs(ml), "the next m removes the focused message")
}

func TestVisual_RangeKeyFixesTheRangeBeingStretched(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(5))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.FixOrStartVisualRange()
	ml.CursorUp()
	ml.CursorUp()
	assert.Equal(t, []int{4, 5}, selectedIDs(ml))
}

func TestVisual_RangeKeyStartsAnotherRangeThatAddsToTheSet(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(6))
	require.True(t, ml.StartVisualMark())
	ml.CursorUp()
	ml.CursorUp()
	ml.FixOrStartVisualRange()
	ml.CursorUp()
	assert.Equal(t, []int{3, 4, 6}, selectedIDs(ml))
	assert.True(t, ml.VisualHasGaps())
}

func TestVisual_AnAlbumIsOneItem(t *testing.T) {
	now := time.Now()
	ml := components.NewMessageList(40, 60)
	ml.SetMessages([]domain.Message{
		{ID: 1, ChatID: 1, Text: "before", Date: now},
		{ID: 2, ChatID: 1, GroupedID: 9, Photo: &domain.PhotoRef{ID: 21}, Text: "caption", Date: now},
		{ID: 3, ChatID: 1, GroupedID: 9, Photo: &domain.PhotoRef{ID: 22}, Date: now},
		{ID: 4, ChatID: 1, Text: "after", Date: now},
	})
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	assert.Equal(t, []int{2, 3, 4}, selectedIDs(ml))

	ml.FixRangeOrToggleMark()
	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{4}, selectedIDs(ml), "unmarking the album unmarks both of its parts")
	ml.FixRangeOrToggleMark()
	assert.Equal(t, []int{2, 3, 4}, selectedIDs(ml), "marking the album marks both of its parts")
}

func TestVisual_DeletingAMessageInTheSetKeepsTheRest(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(6))
	require.True(t, ml.StartVisualMark())
	ml.CursorUp()
	ml.CursorUp()
	ml.FixRangeOrToggleMark()
	ml.CursorUp()
	ml.FixRangeOrToggleMark()
	require.Equal(t, []int{3, 4, 6}, selectedIDs(ml))

	ml.RemoveMessage(4)
	ml.RemoveMessage(1)

	assert.True(t, ml.InVisual())
	assert.Equal(t, []int{3, 6}, selectedIDs(ml))
	assert.Equal(t, 3, ml.SelectedMessageID())
}

func TestVisual_DeletingTheCursorMovesItTowardTheAnchor(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(6))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	ml.CursorUp()
	ml.RemoveMessage(3)
	assert.Equal(t, 4, ml.SelectedMessageID())
	assert.Equal(t, []int{4, 5, 6}, selectedIDs(ml))
}

func TestVisual_DeletingTheAnchorKeepsTheRangeFromItsNeighbour(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(6))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	ml.RemoveMessage(6)
	assert.Equal(t, []int{4, 5}, selectedIDs(ml))
	ml.CursorUp()
	assert.Equal(t, []int{3, 4, 5}, selectedIDs(ml))
}

func TestVisual_DeletingAMessageDoesNotMoveTheView(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(40))
	for range 20 {
		ml.CursorUp()
	}
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	ml.View()
	start, offset := ml.ViewStart(), ml.LineOffset()
	cursor := ml.SelectedMessageID()

	ml.RemoveMessage(40)
	ml.RemoveMessage(cursor + 1)

	assert.Equal(t, start, ml.ViewStart())
	assert.Equal(t, offset, ml.LineOffset())
	assert.Equal(t, cursor, ml.SelectedMessageID())
	assert.Equal(t, []int{cursor, cursor + 2}, selectedIDs(ml))
}

func TestVisual_OlderMessagesLoadedAboveKeepTheRange(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	all := makeMessages(6)
	ml.SetMessages(all[3:])
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()
	require.True(t, ml.CursorUp(), "the cursor is at the oldest loaded message")
	ml.PrependMessages(all[:3])
	ml.CursorUp()
	assert.Equal(t, []int{3, 4, 5, 6}, selectedIDs(ml))
}

func TestVisual_CursorToOldestStretchesTheRangeToTheTop(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(4))
	require.True(t, ml.StartVisualRange())
	ml.CursorToOldest()
	assert.Equal(t, []int{1, 2, 3, 4}, selectedIDs(ml))
}

func TestVisual_ClearEndsTheSelection(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.ClearVisual()
	assert.False(t, ml.InVisual())
	assert.Empty(t, ml.VisualSelection())
}

func TestVisual_AReloadOfTheSameChatKeepsTheSelection(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	all := makeMessages(6)
	ml.SetMessages(all)
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.CursorUp()

	ml.SetMessages(all)
	assert.True(t, ml.InVisual())
	assert.Equal(t, []int{4, 5, 6}, selectedIDs(ml))
	assert.Equal(t, 4, ml.SelectedMessageID())

	withoutFive := append(append([]domain.Message{}, all[:4]...), all[5])
	ml.SetMessages(withoutFive)
	assert.True(t, ml.InVisual())
	assert.Equal(t, []int{4, 6}, selectedIDs(ml))
}

func TestVisual_AReloadOfTheSameChatKeepsTheMarkedSet(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	all := makeMessages(6)
	ml.SetMessages(all)
	require.True(t, ml.StartVisualMark())
	ml.CursorUp()
	ml.CursorUp()
	ml.FixRangeOrToggleMark()

	ml.SetMessages(all[1:])
	assert.Equal(t, []int{4, 6}, selectedIDs(ml))
	assert.True(t, ml.VisualHasGaps())
}

func TestVisual_AReloadOfTheSameChatDoesNotMoveTheView(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	all := makeMessages(40)
	ml.SetMessages(all)
	for range 20 {
		ml.CursorUp()
	}
	require.True(t, ml.StartVisualRange())
	ml.CursorUp()
	ml.View()
	start, offset := ml.ViewStart(), ml.LineOffset()

	ml.SetMessages(all[1:])

	assert.Equal(t, start-1, ml.ViewStart())
	assert.Equal(t, offset, ml.LineOffset())
}

func TestVisual_LoadingAnotherChatEndsTheSelection(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisualRange())
	other := makeMessages(3)
	for i := range other {
		other[i].ChatID = 2
	}
	ml.SetMessages(other)
	assert.False(t, ml.InVisual())
}

func TestVisual_ClearingTheMessagesEndsTheSelection(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisualRange())
	ml.SetMessages(nil)
	assert.False(t, ml.InVisual())
}

func TestVisual_CannotStartOnAnEmptyList(t *testing.T) {
	ml := components.NewMessageList(20, 40)
	assert.False(t, ml.StartVisualRange())
	assert.False(t, ml.StartVisualMark())
	assert.False(t, ml.InVisual())
}

func TestVisual_HalfPageUpThenDownStretchesAndShrinksTheRange(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(40))
	ml.View()
	require.True(t, ml.StartVisualRange())

	ml.CursorUpByLines(6)
	stretched := selectedIDs(ml)
	assert.Greater(t, len(stretched), 1)
	assert.Equal(t, 40, stretched[len(stretched)-1])

	ml.CursorDownByLines(6)
	assert.Equal(t, []int{40}, selectedIDs(ml))
}

func TestVisual_HalfPageUpStopsAtTheOldestLoadedMessage(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisualRange())
	assert.True(t, ml.CursorUpByLines(100))
	assert.Equal(t, []int{1, 2, 3}, selectedIDs(ml))
}

func TestVisual_HalfPageDownOnTheNewestMessageReportsTheBottom(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(3))
	require.True(t, ml.StartVisualRange())
	assert.True(t, ml.CursorDownByLines(6))
	assert.Equal(t, []int{3}, selectedIDs(ml))
}

func screenRowOf(t *testing.T, view, text string) int {
	t.Helper()
	for i, line := range strings.Split(view, "\n") {
		if strings.Contains(line, " "+text+" ") {
			return i
		}
	}
	t.Fatalf("%q is not on screen", text)
	return -1
}

func TestVisual_AReloadThatLoadsOlderMessagesKeepsTheMessagesOnTheirRows(t *testing.T) {
	ml := components.NewMessageList(30, 40)
	all := makeMessages(40)
	all[9].Text = "msg 10\nline two\nline three"
	ml.SetMessages(all[10:])
	for ml.SelectedMessageID() != 14 {
		ml.CursorUp()
	}
	require.True(t, ml.StartVisualMark())
	before := ml.View()

	reloaded := append(append([]domain.Message{}, all[8:15]...), all[17:]...)
	ml.SetMessages(reloaded)
	after := ml.View()

	assert.Equal(t, screenRowOf(t, before, "msg 14"), screenRowOf(t, after, "msg 14"))
	assert.Equal(t, screenRowOf(t, before, "msg 12"), screenRowOf(t, after, "msg 12"))
	assert.Equal(t, []int{14}, selectedIDs(ml))
}

func TestVisual_DeletingThePartlyHiddenTopMessageKeepsTheOthersOnTheirRows(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	msgs := makeMessages(30)
	for i := range msgs {
		msgs[i].Text = fmt.Sprintf("msg %d\nline two\nline three", i+1)
	}
	ml.SetMessages(msgs)
	for ml.SelectedMessageID() != 20 {
		ml.CursorUp()
	}
	require.True(t, ml.StartVisualMark())
	ml.View()
	for ml.LineOffset() != 2 {
		ml.ScrollUp()
	}
	top := ml.ViewStart()
	before := ml.View()
	nextRow := screenRowOf(t, before, fmt.Sprintf("msg %d", top+1))

	ml.RemoveMessage(top)
	after := ml.View()

	assert.Equal(t, nextRow, screenRowOf(t, after, fmt.Sprintf("msg %d", top+1)))
	assert.True(t, ml.InVisual())
}
