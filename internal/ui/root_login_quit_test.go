package ui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// newLoginRoot is the root as the app builds it before anything has connected:
// on the login screen, with a live login model.
func newLoginRoot() ui.RootModel {
	m := ui.NewRootModel(nil, 50, false)
	m.SetLoginModel(screens.NewLoginModel(internaltg.NewAuthFlow()))
	return m
}

// quits reports whether cmd, or any command batched with it, quits the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	for _, msg := range drainMsgs(cmd()) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return true
		}
	}
	return false
}

func withLoginStep(t *testing.T, m ui.RootModel, msg tea.Msg) ui.RootModel {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(ui.RootModel)
}

// The login screen is where a stuck connection leaves a person, so every step
// of it has to let them out (#283).
func TestLogin_ModifierQuitBindingQuitsOnEveryStep(t *testing.T) {
	steps := map[string]tea.Msg{
		"connecting": nil,
		"phone":      screens.AuthRequestMsg{Step: internaltg.AuthStepPhone},
		"code":       screens.AuthRequestMsg{Step: internaltg.AuthStepCode},
		"password":   screens.AuthRequestMsg{Step: internaltg.AuthStepPassword},
		"error":      screens.AuthErrorMsg{Text: "boom"},
	}
	presses := map[string]tea.KeyPressMsg{
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
		"ctrl+q": {Code: 'q', Mod: tea.ModCtrl},
	}
	for stepName, stepMsg := range steps {
		for keyName, press := range presses {
			t.Run(stepName+"/"+keyName, func(t *testing.T) {
				m := newLoginRoot()
				if stepMsg != nil {
					m = withLoginStep(t, m, stepMsg)
				}
				_, cmd := m.Update(press)
				assert.True(t, quits(cmd))
			})
		}
	}
}

// A bare q is a character someone may be typing into the phone or password
// field, so on this screen it is text and never a quit.
func TestLogin_BareQIsTypedNotQuit(t *testing.T) {
	m := withLoginStep(t, newLoginRoot(), tea.WindowSizeMsg{Width: 100, Height: 40})
	m = withLoginStep(t, m, screens.AuthRequestMsg{Step: internaltg.AuthStepPhone})

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})

	assert.False(t, quits(cmd))
	// The placeholder is "+1234567890", so a q on screen is the one just typed.
	assert.Contains(t, next.(ui.RootModel).View().Content, "q")
}

func TestLogin_QuitReboundToBareKeyDoesNotQuit(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"x"}},
	})
	require.Empty(t, warns)
	m := newLoginRoot().WithKeyMap(km)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

	assert.False(t, quits(cmd))
}

func TestLogin_QuitReboundToModifierKeyQuits(t *testing.T) {
	km, warns := keys.MergeOverrides(keys.DefaultKeyMap(), map[string]map[string][]string{
		"global": {"quit": {"alt+x"}},
	})
	require.Empty(t, warns)
	m := newLoginRoot().WithKeyMap(km)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModAlt})

	assert.True(t, quits(cmd))
}
