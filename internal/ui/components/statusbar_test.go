package components_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func TestStatusBar_NormalMode(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetMode(keys.ModeNormal)
	assert.Contains(t, sb.View(), "NORMAL")
}

func TestStatusBar_InsertMode(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetMode(keys.ModeInsert)
	assert.Contains(t, sb.View(), "INSERT")
}

func TestStatusBar_StatusText(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetStatus("Loading...")
	assert.Contains(t, sb.View(), "Loading...")
}
