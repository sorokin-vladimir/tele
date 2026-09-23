package ui

import (
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/notices"
)

func testNotices() []notices.Notice {
	return []notices.Notice{
		{ID: "first", Title: "First", Body: "body one", Delay: 2 * time.Second},
		{ID: "second", Title: "Second", Body: "body two", Delay: 1 * time.Second},
	}
}

func TestNotices_KeysAreSwallowedUntilCountdownEnds(t *testing.T) {
	seen := notices.NewMemorySeen()
	m := RootModel{}.WithNotices(testNotices(), seen)

	require.True(t, m.noticeActive())
	m, handled := m.handleNoticeKey()
	assert.True(t, handled, "key must be swallowed while the countdown runs")
	assert.True(t, m.noticeActive(), "notice must stay up")
	assert.False(t, seen.IsSeen("first"))
}

func TestNotices_DismissAfterCountdownAdvancesQueue(t *testing.T) {
	seen := notices.NewMemorySeen()
	m := RootModel{}.WithNotices(testNotices(), seen)

	m = m.noticeTick()
	m = m.noticeTick()
	assert.Equal(t, 0, m.noticeRemaining())

	m, handled := m.handleNoticeKey()
	assert.True(t, handled)
	assert.True(t, seen.IsSeen("first"), "a dismissed notice is marked seen")
	require.True(t, m.noticeActive(), "the second notice follows")
	assert.Equal(t, 1, m.noticeRemaining(), "its own delay starts fresh")
}

func TestNotices_QueueEndsAfterLastDismissal(t *testing.T) {
	seen := notices.NewMemorySeen()
	m := RootModel{}.WithNotices(testNotices()[:1], seen)

	m = m.noticeTick()
	m = m.noticeTick()
	m, _ = m.handleNoticeKey()

	assert.False(t, m.noticeActive())
	assert.True(t, seen.IsSeen("first"))
}

func TestNotices_QuittingEarlyLeavesNoticeUnseen(t *testing.T) {
	seen := notices.NewMemorySeen()
	m := RootModel{}.WithNotices(testNotices(), seen)

	m = m.noticeTick()
	assert.True(t, m.noticeActive(), "the notice is still up")
	assert.False(t, seen.IsSeen("first"),
		"seen is recorded on dismissal, not on display")
}

func TestNotices_InactiveWhenNothingPending(t *testing.T) {
	m := RootModel{}.WithNotices(nil, notices.NewMemorySeen())
	assert.False(t, m.noticeActive())

	_, handled := m.handleNoticeKey()
	assert.False(t, handled, "with no notice, keys pass through untouched")
}

// The queue tests above do not prove the modal is wired into rendering. This
// covers the login screen specifically: that is the path with no overlay stack
// of its own, and the one a migration notice must survive.
func TestNotices_RenderedOverTheLoginScreen(t *testing.T) {
	m := NewRootModel(50, false)
	m = m.WithNotices(testNotices(), notices.NewMemorySeen())
	m.width, m.height = 100, 30

	out := xansi.Strip(m.View().Content)

	assert.Contains(t, out, "First", "the notice title must be visible")
	assert.Contains(t, out, "continue in 2s", "the countdown must be visible")
}

func TestNotices_AbsentFromViewWhenNothingPending(t *testing.T) {
	m := NewRootModel(50, false)
	m = m.WithNotices(nil, notices.NewMemorySeen())
	m.width, m.height = 100, 30

	assert.NotContains(t, xansi.Strip(m.View().Content), "continue in")
}
