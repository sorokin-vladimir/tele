package components_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func plain(s string) string { return xansi.Strip(s) }

func TestHelpModal_ListsBindingWithKey(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	assert.Contains(t, plain(h.View()), "Keyboard shortcuts")
	assert.Contains(t, plain(h.View()), "Global") // first section visible at top

	// The chat 'reply' binding sits further down; scrolling must reveal it.
	var seen string
	for i := 0; i < 100; i++ {
		seen += plain(h.View())
		h, _ = h.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	assert.Contains(t, seen, "reply")
	assert.Contains(t, seen, "Chat")
}

func TestHelpModal_ListsPasteImage(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	// The composer paste-image binding (#163) is listed; scroll to reveal it.
	var seen string
	for i := 0; i < 100; i++ {
		seen += plain(h.View())
		h, _ = h.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	assert.Contains(t, seen, "ctrl+v")
	assert.Contains(t, seen, "paste image from clipboard as photo")
}

func TestHelpModal_FitsIn80x24(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	// lipgloss.Width measures display cells (box-drawing runes are multi-byte).
	for _, line := range strings.Split(h.View(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 80,
			"line wider than terminal: %q", plain(line))
	}
	assert.LessOrEqual(t, len(strings.Split(h.View(), "\n")), 24)
}

func TestHelpModal_Scroll(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	before := h.View()
	// Scroll down several rows.
	for i := 0; i < 5; i++ {
		h, _ = h.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	}
	after := h.View()
	assert.NotEqual(t, before, after, "scrolling changes the visible window")
}

func TestHelpModal_CloseKeys(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	_, open := h.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	assert.False(t, open, "esc closes")

	h2 := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	_, open2 := h2.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	assert.False(t, open2, "'?' closes")
}

func TestDescribeShort_OverlayWording(t *testing.T) {
	// Overlay hint bars route wording through DescribeShort so an action reads
	// the same everywhere as the status bar and the help modal.
	assert.Equal(t, "move", components.DescribeShort(keys.ContextContextMenu, keys.ActionDown))
	assert.Equal(t, "close", components.DescribeShort(keys.ContextContextMenu, keys.ActionCancel))
	assert.Equal(t, "open/select", components.DescribeShort(keys.ContextFilePicker, keys.ActionConfirm))
	assert.Equal(t, "open", components.DescribeShort(keys.ContextSearch, keys.ActionConfirm))
	assert.Equal(t, "react", components.DescribeShort(keys.ContextContextMenu, keys.ActionReact))
}

func TestHelpModal_LongestDefaultBindingShownInFull(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	assert.GreaterOrEqual(t, h.KeyCol(), 13, "key column must accommodate ctrl+d/ctrl+u")

	var allBody string
	for _, l := range h.LinesForTest() {
		allBody += plain(l) + "\n"
	}
	assert.Contains(t, allBody, "ctrl+d/ctrl+u", "collapsed scroll pair must appear in full")
	assert.NotContains(t, allBody, "ctrl+d/ctr ", "binding must not be cut at 10 chars")
}

func TestHelpModal_KeyWidthMeasuredInCellsNotBytes(t *testing.T) {
	km := keys.KeyMap{
		keys.ContextGlobal: map[string]keys.Action{
			"alt+←/alt+→": keys.ActionQuit,
		},
	}
	h := components.NewHelpModal(km, 80, 24)
	assert.Equal(t, 11, h.KeyCol(), "key column must measure display cells (11), not bytes (15)")

	var found bool
	for _, l := range h.LinesForTest() {
		stripped := plain(l)
		if strings.Contains(stripped, "alt+←/alt+→") {
			found = true
			require.True(t, utf8.ValidString(stripped), "rendered line must have valid UTF-8")
			assert.True(t, strings.HasPrefix(stripped, "  alt+←/alt+→  "), "padding and gap must align by display cells")
		}
	}
	assert.True(t, found, "unicode binding must be rendered")
}

func TestHelpModal_NarrowTerminalWrapsGracefully(t *testing.T) {
	h40 := components.NewHelpModal(keys.DefaultKeyMap(), 40, 24)
	for _, line := range strings.Split(h40.View(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 40, "modal line must fit within width 40: %q", plain(line))
		assert.True(t, utf8.ValidString(line), "must not slice mid-rune")
	}

	var all40 string
	for _, l := range h40.LinesForTest() {
		all40 += plain(l) + "\n"
	}
	assert.Contains(t, all40, "ctrl+d/ctrl+u", "collapsed pair must not be cut on narrow terminal")
	assert.Contains(t, all40, "paste image", "long description must not be dropped")

	h24 := components.NewHelpModal(keys.DefaultKeyMap(), 24, 20)
	for _, line := range strings.Split(h24.View(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 24, "modal line must fit within width 24: %q", plain(line))
		assert.True(t, utf8.ValidString(line), "must not slice mid-rune")
	}

	var all24 string
	for _, l := range h24.LinesForTest() {
		all24 += plain(l) + "\n"
	}
	assert.Contains(t, all24, "ctrl+d/ctrl+u", "collapsed pair must be intact on very narrow terminal")
	assert.Contains(t, all24, "paste image", "wrapped description must be present")
}

func TestHelpModal_KeyColGrowsDynamically(t *testing.T) {
	km := keys.KeyMap{
		keys.ContextGlobal: map[string]keys.Action{
			"ctrl+shift+alt+x": keys.ActionQuit,
		},
	}
	h := components.NewHelpModal(km, 80, 24)
	assert.Equal(t, 16, h.KeyCol(), "key column must grow to widest key without 10-char clamp")

	var found bool
	for _, l := range h.LinesForTest() {
		if strings.Contains(plain(l), "ctrl+shift+alt+x") {
			found = true
		}
	}
	assert.True(t, found, "long binding must be displayed in full")
}

func TestHelpModal_SetSizeRebuildsLayout(t *testing.T) {
	h := components.NewHelpModal(keys.DefaultKeyMap(), 80, 24)
	lines80 := len(h.LinesForTest())

	h.SetSize(36, 24)
	lines36 := len(h.LinesForTest())
	assert.Greater(t, lines36, lines80, "narrower width should wrap descriptions into more lines")

	for _, line := range strings.Split(h.View(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 36, "line must fit in resized width")
	}
}
