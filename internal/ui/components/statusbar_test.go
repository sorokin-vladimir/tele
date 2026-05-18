package components_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
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

func TestStatusBar_ChatlistHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chatlist")
	view := sb.View()
	assert.Contains(t, view, "j/k -> move")
	assert.Contains(t, view, "enter -> open")
	assert.Contains(t, view, "/ -> search")
	assert.Contains(t, view, "q -> quit")
}

func TestStatusBar_ChatNormalHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeNormal)
	view := sb.View()
	assert.Contains(t, view, "j/k -> scroll")
	assert.Contains(t, view, "space -> menu")
	assert.Contains(t, view, "-> write")
	assert.Contains(t, view, "q -> quit")
}

func TestStatusBar_ChatInsertHints(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chat")
	sb.SetMode(keys.ModeInsert)
	view := sb.View()
	assert.Contains(t, view, "enter -> send")
	assert.Contains(t, view, "esc -> normal")
}

func TestStatusBar_NoKeyMap_NoHints(t *testing.T) {
	sb := components.NewStatusBar(80)
	sb.SetActivePane("chatlist")
	view := sb.View()
	assert.NotContains(t, view, "->")
}

func TestStatusBar_HintsAppendedAfterStatus(t *testing.T) {
	km := keys.DefaultKeyMap()
	sb := components.NewStatusBar(120)
	sb.SetKeyMap(km)
	sb.SetActivePane("chatlist")
	sb.SetStatus("12 chats")
	view := sb.View()
	statusIdx := strings.Index(view, "12 chats")
	hintIdx := strings.Index(view, "j/k -> move")
	assert.Greater(t, hintIdx, statusIdx, "hints should appear after status text")
}
