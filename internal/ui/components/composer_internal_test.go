package components

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/stretchr/testify/assert"
)

func TestComposer_RefreshesTextareaStylesForThemeChange(t *testing.T) {
	t.Cleanup(func() { theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight}) })
	theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})

	c := NewComposer(60)
	c.Focus()

	theme.Apply(false)
	c.View()
	assert.Equal(t, textarea.DefaultLightStyles().Focused.CursorLine.GetBackground(), c.ta.Styles().Focused.CursorLine.GetBackground())

	theme.Apply(true)
	c.View()
	assert.Equal(t, textarea.DefaultDarkStyles().Focused.CursorLine.GetBackground(), c.ta.Styles().Focused.CursorLine.GetBackground())
}
