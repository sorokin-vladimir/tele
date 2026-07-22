package components_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
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
