package components

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

type Composer struct {
	input textinput.Model
}

func NewComposer(width int) *Composer {
	ti := textinput.New()
	ti.Placeholder = "Write a message..."
	ti.SetWidth(width)
	return &Composer{input: ti}
}

func (c *Composer) SetWidth(w int)    { c.input.SetWidth(w) }
func (c *Composer) Focus()            { c.input.Focus() }
func (c *Composer) Blur()             { c.input.Blur() }
func (c *Composer) Value() string     { return c.input.Value() }
func (c *Composer) SetValue(v string) { c.input.SetValue(v) }
func (c *Composer) Reset()            { c.input.Reset() }
func (c *Composer) View() string      { return c.input.View() }

func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

func (c *Composer) Init() tea.Cmd {
	return textinput.Blink
}
