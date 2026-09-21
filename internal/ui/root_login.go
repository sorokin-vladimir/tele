package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// isLoginQuit reports whether a key pressed on the login screen quits. Only a
// quit binding that carries ctrl or alt does: a bare key such as q is a
// character someone may be typing into the phone or password field, and on this
// screen it goes to the field (#283).
func (m RootModel) isLoginQuit(msg tea.KeyPressMsg) bool {
	if !msg.Mod.Contains(tea.ModCtrl) && !msg.Mod.Contains(tea.ModAlt) {
		return false
	}
	return m.keyMap.Resolve(keys.ContextGlobal, msg.String()) == keys.ActionQuit
}
