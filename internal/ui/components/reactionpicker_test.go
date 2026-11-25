package components_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pressLeft() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyLeft} }
func pressRight() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyRight} }

func TestReactionPicker_Enter_EmitsConfirmed(t *testing.T) {
	p := components.NewReactionPicker("")
	newP, cmd := p.Update(pressEnter())
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg, ok := cmd().(components.ReactConfirmedMsg)
	require.True(t, ok)
	assert.Equal(t, "🤝", msg.Emoji) // top-left of grid
}

func TestReactionPicker_Esc_EmitsClose(t *testing.T) {
	p := components.NewReactionPicker("")
	newP, cmd := p.Update(pressEsc())
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	_, ok := cmd().(components.CloseReactionPickerMsg)
	assert.True(t, ok)
}

func TestReactionPicker_RightMoves(t *testing.T) {
	p := components.NewReactionPicker("")
	p, _ = p.Update(pressRight())
	require.NotNil(t, p)
	newP, cmd := p.Update(pressEnter())
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg := cmd().(components.ReactConfirmedMsg)
	assert.Equal(t, "🙏", msg.Emoji) // one step right from top-left
}

func TestReactionPicker_DownMoves(t *testing.T) {
	p := components.NewReactionPicker("")
	p, _ = p.Update(pressDown())
	require.NotNil(t, p)
	newP, cmd := p.Update(pressEnter())
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg := cmd().(components.ReactConfirmedMsg)
	assert.Equal(t, "💯", msg.Emoji) // one step down from top-left
}

func TestReactionPicker_LeftAtEdgeStays(t *testing.T) {
	p := components.NewReactionPicker("")
	p, _ = p.Update(pressLeft())
	require.NotNil(t, p)
	newP, cmd := p.Update(pressEnter())
	assert.Nil(t, newP)
	msg := cmd().(components.ReactConfirmedMsg)
	assert.Equal(t, "🤝", msg.Emoji) // stays at top-left
}

// pressRune builds a printable key press for a single rune, as a non-Latin
// keyboard layout would report it (e.g. Russian 'о' on the physical J key).
func pressRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestReactionPicker_RussianLayoutDownMoves(t *testing.T) {
	// 'о' is the Russian character on the physical J key (move down).
	p := components.NewReactionPicker("")
	p, _ = p.Update(pressRune('о'))
	require.NotNil(t, p)
	newP, cmd := p.Update(pressEnter())
	assert.Nil(t, newP)
	require.NotNil(t, cmd)
	msg := cmd().(components.ReactConfirmedMsg)
	assert.Equal(t, "💯", msg.Emoji) // one step down from top-left
}

func TestReactionPicker_ViewContainsEmoji(t *testing.T) {
	p := components.NewReactionPicker("")
	v := p.View()
	assert.Contains(t, v, "🤝")
	assert.Contains(t, v, "🤬")
}

// A reaction picked again is taken back only when it matches the set Telegram
// sent byte for byte. Every set from Telegram is spelled by
// domain.EmojiPresentation, so each emoji the picker sends must be what that
// spelling makes of the emoji as Telegram sends it, without U+FE0F (#294).
func TestReactionPicker_SendsEachEmojiAsTelegramsSetSpellsIt(t *testing.T) {
	seen := map[string]bool{}
	for row := 0; row < 4; row++ {
		for col := 0; col < 7; col++ {
			p := components.NewReactionPicker("")
			for range row {
				p, _ = p.Update(pressDown())
			}
			for range col {
				p, _ = p.Update(pressRight())
			}
			_, cmd := p.Update(pressEnter())
			require.NotNil(t, cmd)
			picked := cmd().(components.ReactConfirmedMsg).Emoji

			fromTelegram := strings.ReplaceAll(picked, "\U0000FE0F", "")
			assert.Equal(t, picked, domain.EmojiPresentation(fromTelegram),
				"%q as Telegram sends it is spelled %q", fromTelegram, domain.EmojiPresentation(fromTelegram))
			seen[picked] = true
		}
	}
	assert.Len(t, seen, 28, "every cell of the grid was picked")
}
