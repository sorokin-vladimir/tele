package components

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// anchorMessages returns n messages, the newest last, each long enough to wrap
// once at the widths used below.
func anchorMessages(from, n int) []domain.Message {
	now := time.Now()
	msgs := make([]domain.Message, 0, n)
	for i := 0; i < n; i++ {
		msgs = append(msgs, domain.Message{
			ID:   from + i,
			Date: now,
			Text: "сообщение номер " + strings.Repeat("x", 10),
		})
	}
	return msgs
}

// leadingBlankRows counts the empty rows a frame starts with.
func leadingBlankRows(view string) int {
	n := 0
	for _, line := range strings.Split(xansi.Strip(view), "\n") {
		if strings.TrimSpace(line) != "" {
			break
		}
		n++
	}
	return n
}

// assertNoBlankBand states the invariant the whole fix is about: a frame that
// runs to the last item must not open with blank rows while older items exist.
func assertNoBlankBand(t *testing.T, ml *MessageList, msg string) {
	t.Helper()
	view := ml.View()
	if ml.viewStart == 0 && ml.lineOffset == 0 {
		return // the genuine short-history case: padding at the top is correct
	}
	assert.Zero(t, leadingBlankRows(view), msg)
}

// The chat-open path: ChatReset seats a stale chat's last few messages, the
// backfill then prepends the history above them. PrependMessages preserves the
// visual position, which on an already top-padded frame parks the new messages
// off-screen and keeps the blank band (#225).
func TestView_PrependedHistoryFillsTheBlankBand(t *testing.T) {
	ml := NewMessageList(30, 80)
	ml.SetMessages(anchorMessages(100, 3))

	// The short window is bottom-anchored with padding above it - correct here.
	require.Positive(t, leadingBlankRows(ml.View()))

	ml.PrependMessages(anchorMessages(1, 40))

	out := xansi.Strip(ml.View())
	assert.Zero(t, leadingBlankRows(out), "backfilled history left the pane top blank")
	assert.Equal(t, 30, len(strings.Split(out, "\n")), "frame is not viewport-height")
	assertNoBlankBand(t, ml, "prepend")
}

// The scrollbar is built before the pane is rendered, so it has to see the
// repaired anchor too - otherwise the thumb describes a position that the very
// next View() discards.
func TestScrollInfo_ReportsTheRepairedAnchor(t *testing.T) {
	ml := NewMessageList(30, 80)
	ml.SetMessages(anchorMessages(100, 3))
	ml.PrependMessages(anchorMessages(1, 40))

	info := ml.ScrollInfo()
	assert.Equal(t, info.Total-info.Visible, info.Offset,
		"the viewport is at the bottom, so the thumb must be at the end of the track")
}

// A growing pane (the composer collapsing after a multi-line draft, or a
// terminal resize) frees rows above the anchor. Nothing recomputes the position
// on that path, so the freed rows used to stay blank until a scroll key.
func TestView_GrowingTheViewportPullsInHistory(t *testing.T) {
	for _, from := range []int{10, 16, 24} {
		ml := NewMessageList(from, 80)
		ml.SetMessages(anchorMessages(1, 40))
		ml.ScrollToBottom()

		ml.SetSize(80, from+12)

		out := ml.View()
		assert.Zero(t, leadingBlankRows(out), "height %d→%d left the pane top blank", from, from+12)
		assert.Equal(t, from+12, len(strings.Split(xansi.Strip(out), "\n")))
	}
}

// The second manifestation from the issue: a lineOffset larger than the lines
// actually available above the anchor, revealed one row per keypress. Any
// height shrink under a fixed position produces it (an image dropped from the
// LRU cache, an edit shortening a message).
func TestView_StaleLineOffsetDoesNotTearTheFrame(t *testing.T) {
	ml := NewMessageList(24, 80)
	ml.SetMessages(anchorMessages(1, 40))
	ml.ScrollToBottom()

	// Park the anchor far below where the content can fill the pane.
	ml.viewStart = len(ml.items) - 2
	ml.lineOffset = 3

	assertNoBlankBand(t, ml, "stale line offset")
	assert.Equal(t, 24, len(strings.Split(xansi.Strip(ml.View()), "\n")))
}

// Padding at the top stays right for the one case it was written for: a history
// shorter than the pane, with nothing above the viewport to pull in.
func TestView_ShortHistoryKeepsItsTopPadding(t *testing.T) {
	ml := NewMessageList(30, 80)
	ml.SetMessages(anchorMessages(1, 2))

	assert.Positive(t, leadingBlankRows(ml.View()))
	assert.Zero(t, ml.viewStart)
	assert.Zero(t, ml.lineOffset)
}

// Reading history must not be yanked to the bottom: a viewport that is full
// keeps exactly where it is.
func TestView_FullViewportKeepsItsPosition(t *testing.T) {
	ml := NewMessageList(20, 80)
	ml.SetMessages(anchorMessages(1, 60))
	ml.ScrollToTop()
	ml.ScrollDownBy(5)

	start, off := ml.viewStart, ml.lineOffset
	ml.View()
	ml.ScrollInfo()

	assert.Equal(t, start, ml.viewStart, "scrolled position moved")
	assert.Equal(t, off, ml.lineOffset, "scrolled position moved")
}
