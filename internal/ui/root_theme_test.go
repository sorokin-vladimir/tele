package ui

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/stretchr/testify/assert"
)

// The 2s background-color poll is replaced by event-driven theme detection
// (issue #148): a BackgroundColorMsg must update the theme but must NOT
// reschedule a timer.
func TestRoot_BackgroundColorMsg_UpdatesThemeWithoutPolling(t *testing.T) {
	m := idleMainModel()
	newM, cmd := m.Update(tea.BackgroundColorMsg{Color: color.Black})
	assert.Nil(t, cmd, "background color must not reschedule a poll")
	assert.True(t, newM.(RootModel).hasDarkBackground, "black background is dark")
}

// Unsolicited OS color-scheme reports (DEC mode 2031) arrive as raw ultraviolet
// events; they must flip the theme with no command.
func TestRoot_ColorSchemeEvents_UpdateTheme(t *testing.T) {
	m := idleMainModel()

	dark, cmd := m.Update(uv.DarkColorSchemeEvent{})
	assert.Nil(t, cmd)
	assert.True(t, dark.(RootModel).hasDarkBackground, "dark color-scheme event sets dark")

	light, cmd := dark.(RootModel).Update(uv.LightColorSchemeEvent{})
	assert.Nil(t, cmd)
	assert.False(t, light.(RootModel).hasDarkBackground, "light color-scheme event clears dark")
}

// Fallback for terminals without mode 2031: regaining focus re-reads the
// background color so a theme change made while away is picked up.
func TestRoot_Focus_RereadsBackgroundColor(t *testing.T) {
	m := idleMainModel()
	_, cmd := m.Update(tea.FocusMsg{})
	assert.NotNil(t, cmd, "regaining focus must re-request the background color")
}
