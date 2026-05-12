package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

var (
	modeStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	barStyle  = lipgloss.NewStyle().Background(lipgloss.Color("236"))
)

type StatusBar struct {
	width     int
	mode      keys.VimMode
	status    string
	verbose   bool
	lastKey   string
	activePane string
}

func NewStatusBar(width int) *StatusBar {
	return &StatusBar{width: width, mode: keys.ModeNormal}
}

func (sb *StatusBar) SetWidth(w int)          { sb.width = w }
func (sb *StatusBar) SetMode(m keys.VimMode)  { sb.mode = m }
func (sb *StatusBar) SetStatus(s string)      { sb.status = s }
func (sb *StatusBar) SetVerbose(v bool)       { sb.verbose = v }
func (sb *StatusBar) SetLastKey(k string)     { sb.lastKey = k }
func (sb *StatusBar) SetActivePane(p string)  { sb.activePane = p }

func (sb *StatusBar) View() string {
	label := "NORMAL"
	if sb.mode == keys.ModeInsert {
		label = "INSERT"
	}
	left := modeStyle.Render(label)
	line := fmt.Sprintf("%s  %s", left, sb.status)
	if sb.verbose {
		line += fmt.Sprintf("  [pane:%s key:%s]", sb.activePane, sb.lastKey)
	}
	return barStyle.Width(sb.width).Render(line)
}
