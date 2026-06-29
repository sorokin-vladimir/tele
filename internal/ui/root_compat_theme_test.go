package ui

import (
	"testing"

	"charm.land/lipgloss/v2/compat"
	"github.com/stretchr/testify/assert"
)

// compat.AdaptiveColor (used for context-menu / hint / reaction-picker
// backgrounds) resolves against the package-level compat.HasDarkBackground,
// which is detected once at init and never updated. setDarkBackground must keep
// it in sync so those explicit light/dark colors follow a runtime theme change.
func TestRoot_SetDarkBackground_SyncsCompatGlobal(t *testing.T) {
	m := idleMainModel()

	m.setDarkBackground(true)
	assert.True(t, compat.HasDarkBackground, "compat global must follow the theme (dark)")

	m.setDarkBackground(false)
	assert.False(t, compat.HasDarkBackground, "compat global must follow the theme (light)")
}
