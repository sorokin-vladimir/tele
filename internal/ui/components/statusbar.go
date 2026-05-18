package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

var (
	barBg     = lipgloss.Color("236")
	modeStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1).Background(barBg)
	barStyle  = lipgloss.NewStyle().Background(barBg)
)

type StatusBar struct {
	width      int
	mode       keys.VimMode
	status     string
	verbose    bool
	lastKey    string
	activePane string
	keyMap     keys.KeyMap
}

func NewStatusBar(width int) *StatusBar {
	return &StatusBar{width: width, mode: keys.ModeNormal}
}

func (sb *StatusBar) SetWidth(w int)           { sb.width = w }
func (sb *StatusBar) SetMode(m keys.VimMode)   { sb.mode = m }
func (sb *StatusBar) SetStatus(s string)       { sb.status = s }
func (sb *StatusBar) SetVerbose(v bool)        { sb.verbose = v }
func (sb *StatusBar) SetLastKey(k string)      { sb.lastKey = k }
func (sb *StatusBar) SetActivePane(p string)   { sb.activePane = p }
func (sb *StatusBar) SetKeyMap(km keys.KeyMap) { sb.keyMap = km }

func (sb *StatusBar) View() string {
	label := "NORMAL"
	if sb.mode == keys.ModeInsert {
		label = "INSERT"
	}
	left := modeStyle.Render(label)
	line := fmt.Sprintf("%s  %s", left, sb.status)
	if h := sb.hints(); h != "" {
		line += " | " + h
	}
	if sb.verbose {
		line += fmt.Sprintf("  [pane:%s key:%s]", sb.activePane, sb.lastKey)
	}
	return barStyle.Width(sb.width).Render(line)
}

func (sb *StatusBar) hints() string {
	if sb.keyMap == nil {
		return ""
	}
	switch {
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert:
		send := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionConfirm)
		normal := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionNormal)
		return joinHints(hintKey(send, "send"), hintKey(normal, "normal"))
	case sb.activePane == "chat":
		down := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionUp)
		menu := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionOpenContextMenu)
		write := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionInsert)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "scroll"),
			hintKey(menu, "menu"),
			hintKey(write, "write"),
			hintKey(quit, "quit"),
		)
	case sb.activePane == "chatlist":
		down := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionUp)
		open := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionConfirm)
		search := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionSearch)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "move"),
			hintKey(open, "open"),
			hintKey(search, "search"),
			hintKey(quit, "quit"),
		)
	}
	return ""
}

func hintKey(key, desc string) string {
	if key == "" {
		return ""
	}
	return key + " -> " + desc
}

func hintNav(downKey, upKey, desc string) string {
	if downKey == "" && upKey == "" {
		return ""
	}
	return downKey + "/" + upKey + " -> " + desc
}

func joinHints(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " | ")
}
