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
	ta           textarea.Model
	replyPreview string
	focused      bool
	width        int
	visualLines  int
}

func NewComposer(width int) *Composer {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = "> "
	ta.MaxHeight = maxComposerLines
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter"))
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
// textarea lines + 2 border rows + 1 if a reply preview is active.
func (c *Composer) VisualHeight() int {
	h := c.visualLines + 2
	if c.replyPreview != "" {
		h++
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
		content = c.replyPreview + "\n" + c.ta.View()
	} else {
		content = c.ta.View()
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(c.width - 2)
	if c.focused {
		style = style.BorderForeground(lipgloss.Color("12"))
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
