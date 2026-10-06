package components_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func TestConfirmModal_ConfirmKeys(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{
		{Code: 'y', Text: "y"},
		{Code: tea.KeyEnter},
	} {
		m := components.NewConfirmModal("Quit", "Quit tele?", 80, 24)
		_, res := m.Update(msg)
		assert.Equal(t, components.ConfirmYes, res)
	}
}

func TestConfirmModal_CancelKeys(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{
		{Code: 'n', Text: "n"},
		{Code: tea.KeyEscape},
		{Code: 'q', Text: "q"},
	} {
		m := components.NewConfirmModal("Quit", "Quit tele?", 80, 24)
		_, res := m.Update(msg)
		assert.Equal(t, components.ConfirmNo, res)
	}
}

func TestConfirmModal_OtherKeyStaysOpen(t *testing.T) {
	m := components.NewConfirmModal("Quit", "Quit tele?", 80, 24)
	next, res := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Equal(t, components.ConfirmNone, res)
	assert.NotNil(t, next)
}

func TestConfirmModal_ViewShowsQuestion(t *testing.T) {
	m := components.NewConfirmModal("Quit", "Quit tele?", 80, 24)
	out := plain(m.View())
	assert.Contains(t, out, "Quit")
	assert.Contains(t, out, "Quit tele?")
	assert.Contains(t, out, "y")
	assert.Contains(t, out, "n")
}
