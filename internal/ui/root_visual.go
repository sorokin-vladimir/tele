package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func (m RootModel) setVisualMode() {
	m.vimState.Mode = keys.ModeVisual
	m.statusBar.SetMode(keys.ModeVisual)
}

func (m RootModel) endVisual() {
	if m.chat != nil {
		m.chat.ClearVisual()
	}
	if m.vimState.Mode == keys.ModeVisual {
		m.vimState.Mode = keys.ModeNormal
		m.statusBar.SetMode(keys.ModeNormal)
	}
}

func (m RootModel) syncVisual() {
	if m.vimState.Mode != keys.ModeVisual {
		return
	}
	if m.chat == nil || !m.chat.InVisual() {
		m.endVisual()
	}
}

func (m RootModel) visualAction(action keys.Action) (tea.Model, tea.Cmd, bool) {
	if m.vimState.Mode != keys.ModeVisual {
		switch action {
		case keys.ActionVisualSelect:
			if m.chat.StartVisualRange() {
				m.setVisualMode()
			}
			return m, nil, true
		case keys.ActionVisualMark:
			if m.chat.StartVisualMark() {
				m.setVisualMode()
			}
			return m, nil, true
		}
		return m, nil, false
	}
	switch action {
	case keys.ActionCursorUp, keys.ActionCursorDown, keys.ActionUp, keys.ActionDown,
		keys.ActionGoTop, keys.ActionGoBottom, keys.ActionScrollHalfUp, keys.ActionScrollHalfDown:
		return m, nil, false
	case keys.ActionVisualSelect:
		m.chat.VisualRange()
	case keys.ActionVisualMark:
		m.chat.VisualMark()
	case keys.ActionCopyMessage:
		next, cmd := m.copySelection()
		return next, cmd, true
	case keys.ActionNormal:
		m.endVisual()
	}
	m.syncVisual()
	return m, nil, true
}

func (m RootModel) copySelection() (RootModel, tea.Cmd) {
	msgs := m.chat.VisualSelection()
	m.endVisual()
	text := selectionTexts(msgs)
	if text == "" {
		return m, nil
	}
	return m, copyToClipboardCmd(text)
}

func selectionTexts(msgs []domain.Message) string {
	var texts []string
	for _, msg := range msgs {
		if msg.Text != "" {
			texts = append(texts, msg.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}
