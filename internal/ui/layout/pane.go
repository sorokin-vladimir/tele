package layout

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// Pane is implemented by each screen panel (chatlist, chat).
type Pane interface {
	Init() tea.Cmd
	Update(tea.Msg) (Pane, tea.Cmd)
	View() string
	SetSize(width, height int)
	Context() keys.Context
	Focused() bool
	SetFocused(bool)
}
