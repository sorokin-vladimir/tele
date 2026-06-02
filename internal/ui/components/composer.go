package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const maxComposerLines = 5

type Composer struct {
	ta              textarea.Model
	replyPreview    string
	focused         bool
	width           int
	visualLines     int
	hasDarkBackground bool
}

func NewComposer(width int) *Composer {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "> "
	ta.MaxHeight = maxComposerLines
	// Modifier+Enter combos (shift+enter, alt+enter) require a terminal that supports an extended
	// key protocol (Kitty keyboard protocol, or XTerm's modifyOtherKeys). Legacy terminals such as
	// macOS Terminal.app and MinTTY (Git for Windows) silently drop these keys, so neither binding
	// fires there. Both alternatives are registered so that whichever the terminal forwards is caught.
	// Lazygit has the same limitation and handles it identically — document the requirement and list
	// multiple fallbacks. Recommended terminals: Ghostty / iTerm2 (macOS), Windows Terminal (Windows),
	// kitty, WezTerm, Alacritty. tmux users need: set -g extended-keys on
	// See: https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Custom_Keybindings.md#terminal-compatibility
	// Issue: https://github.com/sorokin-vladimir/tele/issues/9#issuecomment-4600787928
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
	ta.KeyMap.Paste = key.NewBinding() // handled at root level via readClipboardCmd → tea.PasteMsg
	ta.CharLimit = 4096
	ta.SetHeight(1)
	ta.SetWidth(width - 2)
	return &Composer{ta: ta, width: width, visualLines: 1}
}

func (c *Composer) SetWidth(w int) {
	c.width = w
	c.ta.SetWidth(w - 2)
	c.recomputeHeight()
}

// Focus activates the composer cursor. Returns a blink Cmd that must be
// returned from the parent Update.
func (c *Composer) Focus() tea.Cmd {
	c.focused = true
	return c.ta.Focus()
}

func (c *Composer) Blur() {
	c.focused = false
	c.ta.Blur()
}

func (c *Composer) SetDarkBackground(isDark bool) { c.hasDarkBackground = isDark }

func (c *Composer) Value() string { return c.ta.Value() }

func (c *Composer) SetValue(v string) {
	c.ta.SetValue(v)
	c.recomputeHeight()
}

func (c *Composer) Reset() {
	c.ta.Reset()
	c.replyPreview = ""
	c.visualLines = 1
	c.ta.SetHeight(1)
}

func (c *Composer) SetReplyPreview(preview string) { c.replyPreview = preview }
func (c *Composer) ClearReplyPreview()             { c.replyPreview = "" }

// VisualHeight returns the total number of terminal rows that View() occupies:
// textarea lines + 2 border rows + preview lines (0 if no preview).
func (c *Composer) VisualHeight() int {
	h := c.visualLines + 2
	if c.replyPreview != "" {
		h += strings.Count(c.replyPreview, "\n") + 2
	}
	return h
}

// recomputeHeight counts visual lines in the current value (accounting for
// word-wrap at the available typing width) and calls ta.SetHeight accordingly.
func (c *Composer) recomputeHeight() {
	available := c.width - 4 // -2 lipgloss border chars, -2 prompt "> "
	if available < 1 {
		available = 1
	}
	val := c.ta.Value()
	if val == "" {
		c.visualLines = 1
		c.ta.SetHeight(1)
		return
	}
	logicalLines := strings.Split(val, "\n")
	total := 0
	for _, line := range logicalLines {
		n := lipgloss.Width(line)
		if n == 0 {
			total++
		} else {
			total += (n + available - 1) / available
		}
	}
	if total < 1 {
		total = 1
	}
	if total > maxComposerLines {
		total = maxComposerLines
	}
	c.visualLines = total
	c.ta.SetHeight(total)
}

func (c *Composer) View() string {
	var content string
	if c.replyPreview != "" {
		content = c.replyPreview + "\n\n" + c.ta.View()
	} else {
		content = c.ta.View()
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(c.width - 2)
	if c.focused {
		fg := lipgloss.LightDark(c.hasDarkBackground)(lipgloss.Color("19"), lipgloss.Color("12"))
		style = style.BorderForeground(fg)
	}
	return style.Render(content)
}

func (c *Composer) Init() tea.Cmd { return nil }

func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	c.recomputeHeight()
	return c, cmd
}
