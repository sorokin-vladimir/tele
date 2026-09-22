package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// The terminal's own paste - cmd+v on macOS, ctrl+v or ctrl+shift+v elsewhere -
// arrives as a bracketed paste, and on the login screen it used to be dropped
// before it reached the field (#284).
func TestLogin_TerminalPasteReachesTheField(t *testing.T) {
	m := withLoginStep(t, sizedLoginRoot(t), screens.AuthRequestMsg{Step: internaltg.AuthStepPhone})

	view := withLoginStep(t, m, tea.PasteMsg{Content: "+447700900123"}).View().Content

	assert.Contains(t, view, "+447700900123")
}

// Pasted into the password field, it is masked like typed text.
func TestLogin_PastedPasswordIsMasked(t *testing.T) {
	m := withLoginStep(t, sizedLoginRoot(t), screens.AuthRequestMsg{Step: internaltg.AuthStepPassword})

	view := withLoginStep(t, m, tea.PasteMsg{Content: "hunter2secret"}).View().Content

	assert.NotContains(t, view, "hunter2secret")
}
