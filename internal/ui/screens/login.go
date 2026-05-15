package screens

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

type AuthRequestMsg struct{ Step internaltg.AuthStep }
type ConnectedMsg struct{}
type TransitionToMainMsg struct{}

// WaitForAuthRequest returns a Cmd that blocks until AuthFlow sends a request or ready closes.
func WaitForAuthRequest(af *internaltg.AuthFlow, ready <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case req := <-af.Requests:
			return AuthRequestMsg{Step: req.Step}
		case <-ready:
			return ConnectedMsg{}
		}
	}
}

type LoginModel struct {
	af     *internaltg.AuthFlow
	input  textinput.Model
	step   internaltg.AuthStep
	prompt string
	err    string
}

func NewLoginModel(af *internaltg.AuthFlow) LoginModel {
	ti := textinput.New()
	ti.Focus()
	ti.SetWidth(40)
	return LoginModel{
		af:    af,
		input: ti,
		step:  -1,
	}
}

func (m LoginModel) CurrentStep() internaltg.AuthStep { return m.step }

func (m LoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case AuthRequestMsg:
		m.step = msg.Step
		switch msg.Step {
		case internaltg.AuthStepPhone:
			m.prompt = "Enter phone number:"
			m.input.Placeholder = "+1234567890"
		case internaltg.AuthStepCode:
			m.prompt = "Enter SMS code:"
			m.input.Placeholder = "12345"
		case internaltg.AuthStepPassword:
			m.prompt = "Enter 2FA password:"
			m.input.EchoMode = textinput.EchoPassword
			m.input.Placeholder = "password"
		}
		m.input.SetValue("")
		return m, nil

	case ConnectedMsg:
		return m, func() tea.Msg { return TransitionToMainMsg{} }

	case tea.KeyPressMsg:
		if msg.Code == tea.KeyEnter && m.step >= 0 {
			val := m.input.Value()
			af := m.af
			go func() {
				af.Responses <- internaltg.AuthResponse{Value: val}
			}()
			m.input.Reset()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m LoginModel) View() tea.View {
	var s string
	if m.step < 0 {
		s = "Connecting...\n"
	} else {
		s = fmt.Sprintf("%s\n\n%s\n\n%s", m.prompt, m.input.View(), m.err)
	}
	return tea.NewView(s)
}
