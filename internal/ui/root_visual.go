package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/transcript"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

func (m RootModel) setVisualMode() {
	m.vimState.Mode = keys.ModeVisual
	m.statusBar.SetMode(keys.ModeVisual)
	m.statusBar.SetVisualHasGaps(m.chat.VisualHasGaps())
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
		return
	}
	m.statusBar.SetVisualHasGaps(m.chat.VisualHasGaps())
}

func visualIgnores(action keys.Action) bool {
	switch action {
	case keys.ActionQuit, keys.ActionDismissToast, keys.ActionShowHelp, keys.ActionShowSettings,
		keys.ActionLogOut, keys.ActionReloadConfig, keys.ActionReloadThemes, keys.ActionChatSearch,
		keys.ActionSearch, keys.ActionMessageSearch, keys.ActionMessageSearchAll:
		return true
	}
	return false
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
		m.chat.FixOrStartVisualRange()
	case keys.ActionVisualMark:
		m.chat.FixRangeOrToggleMark()
	case keys.ActionCopyMessage:
		next, cmd := m.copySelection(false)
		return next, cmd, true
	case keys.ActionCopyWithNames:
		next, cmd := m.copySelection(true)
		return next, cmd, true
	case keys.ActionOpenContextMenu:
		m.contextMenu = components.NewSelectionContextMenu(!m.chat.VisualHasGaps(), m.keyMap)
	case keys.ActionNormal:
		m.endVisual()
	}
	m.syncVisual()
	return m, nil, true
}

func (m RootModel) copySelection(withNames bool) (RootModel, tea.Cmd) {
	msgs := m.chat.VisualSelection()
	withNames = withNames || m.chat.VisualHasGaps()
	m.endVisual()
	text := m.selectionText(msgs, withNames)
	if text == "" {
		return m, nil
	}
	return m, copyToClipboardCmd(text)
}

func (m RootModel) selectionText(msgs []domain.Message, withNames bool) string {
	if !withNames {
		return joinedTexts(msgs)
	}
	f := transcript.Format{
		Location:     time.Local,
		Self:         m.account.LoggedInUser.Name,
		Quotable:     m.chatMsgs,
		OmitUnquoted: true,
	}
	if m.cfg != nil {
		f.NameWidth = m.cfg.CLI.NameWidth
	}
	return transcript.Text(msgs, f)
}

func joinedTexts(msgs []domain.Message) string {
	var texts []string
	for _, msg := range msgs {
		if msg.Text != "" {
			texts = append(texts, msg.Text)
		}
	}
	return strings.Join(texts, "\n\n")
}
