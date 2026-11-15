package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func (m RootModel) endVisual() {
	m.chat.ClearVisual()
	m.vimState.Mode = keys.ModeNormal
	m.statusBar.SetMode(keys.ModeNormal)
}

func (m RootModel) visualAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	inVisual := m.chat.InVisual()
	if !inVisual {
		if m.vimState.Mode == keys.ModeVisual {
			m.endVisual()
		}
		if action != keys.ActionVisualSelect {
			return m, nil, false
		}
		if m.chat.StartVisual() {
			m.vimState.Mode = keys.ModeVisual
			m.statusBar.SetMode(keys.ModeVisual)
		}
		return m, nil, true
	}
	switch action {
	case keys.ActionNone, keys.ActionCursorUp, keys.ActionCursorDown,
		keys.ActionUp, keys.ActionDown, keys.ActionGoTop, keys.ActionGoBottom,
		keys.ActionScrollHalfUp, keys.ActionScrollHalfDown:
		return m, nil, false
	case keys.ActionCopyMessage:
		text := m.chat.VisualText()
		m.endVisual()
		if text == "" {
			return m, nil, true
		}
		return m, copyToClipboardCmd(text), true
	case keys.ActionVisualSelect, keys.ActionNormal:
		m.endVisual()
		return m, nil, true
	}
	m.endVisual()
	return m, nil, false
}
