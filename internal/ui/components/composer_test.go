package components_test

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func TestComposer_Value(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("hello")
	assert.Equal(t, "hello", c.Value())
}

func TestComposer_Reset(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("text")
	c.Reset()
	assert.Empty(t, c.Value())
}

func TestComposer_View_NotEmpty(t *testing.T) {
	c := components.NewComposer(60)
	assert.NotEmpty(t, c.View())
}

func TestComposer_Update_AcceptsInput(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	newC, _ := c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	assert.Equal(t, "a", newC.Value())
}
