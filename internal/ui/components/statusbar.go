package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

var (
	barBg    = lipgloss.Color("236")
	barFg    = lipgloss.Color("252")
	barStyle = lipgloss.NewStyle().Background(barBg).Foreground(barFg)

	barSepStyle = lipgloss.NewStyle().Background(barBg).Foreground(lipgloss.Color("240"))

	modeBase   = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("231"))
	normalMode = modeBase.Background(lipgloss.Color("33")) // blue
	insertMode = modeBase.Background(lipgloss.Color("35")) // green
)

// Severity classifies a transient status-bar message.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

type StatusBar struct {
	width      int
	mode       keys.VimMode
	status     string
	verbose    bool
	lastKey    string
	activePane string
	keyMap     keys.KeyMap
	errText    string
	errSev     Severity
	errSerial  int
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

// SetError shows a transient, severity-tagged message and returns the serial
// identifying it, so a later ClearError only clears this exact message.
func (sb *StatusBar) SetError(text string, sev Severity) int {
	sb.errSerial++
	sb.errText = text
	sb.errSev = sev
	return sb.errSerial
}

// ClearError clears the error only when serial matches the current one, so a
// stale auto-clear timer cannot wipe a newer error.
func (sb *StatusBar) ClearError(serial int) {
	if serial == sb.errSerial {
		sb.errText = ""
	}
}

func (sb *StatusBar) View() string {
	modeStyle := normalMode
	label := "NORMAL"
	if sb.mode == keys.ModeInsert {
		modeStyle = insertMode
		label = "INSERT"
	}

	segs := []string{modeStyle.Render(label)}

	if sb.errText != "" {
		segs = append(segs, errStyle(sb.errSev).Render(sb.errText))
	} else if sb.status != "" {
		segs = append(segs, barStyle.Render(sb.status))
	}
	if h := sb.hints(); h != "" {
		segs = append(segs, barStyle.Render(h))
	}
	if sb.verbose {
		segs = append(segs, barStyle.Render(fmt.Sprintf("pane:%s key:%s", sb.activePane, sb.lastKey)))
	}

	sep := barSepStyle.Render(" │ ")
	return barStyle.Width(sb.width).Render(strings.Join(segs, sep))
}

func errStyle(sev Severity) lipgloss.Style {
	base := lipgloss.NewStyle().Background(barBg).Bold(true)
	switch sev {
	case SeverityError:
		return base.Foreground(lipgloss.Color("203")) // red
	case SeverityWarning:
		return base.Foreground(lipgloss.Color("214")) // amber
	default:
		return base.Foreground(lipgloss.Color("75")) // blue/info
	}
}

func (sb *StatusBar) hints() string {
	if sb.keyMap == nil {
		return ""
	}
	switch {
	case sb.activePane == "folders":
		down := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionUp)
		sel := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionConfirm)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "move"),
			hintKey(sel, "select"),
			hintKey(quit, "quit"),
		)
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert:
		send := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionConfirm)
		normal := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionNormal)
		return joinHints(hintKey(send, "send"), hintKey(normal, "normal"))
	case sb.activePane == "chat":
		down := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionUp)
		write := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionInsert)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "scroll"),
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
	return strings.Join(out, " · ")
}
