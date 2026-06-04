package screens

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

type AuthRequestMsg struct {
	Step internaltg.AuthStep
	Hint string
}
type AuthErrorMsg struct{ Text string }
type ConnectedMsg struct{}
type TransitionToMainMsg struct{}

// WaitForAuthRequest returns a Cmd that blocks until AuthFlow sends a request, an error, or ready closes.
func WaitForAuthRequest(af *internaltg.AuthFlow, ready <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case req := <-af.Requests:
			return AuthRequestMsg{Step: req.Step, Hint: req.Hint}
		case <-ready:
			return ConnectedMsg{}
		case text := <-af.Errors:
			return AuthErrorMsg{Text: text}
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
			m.prompt = msg.Hint
			if m.prompt == "" {
				m.prompt = "Enter the login code:"
			}
			m.input.Placeholder = "12345"
		case internaltg.AuthStepPassword:
			m.prompt = "Enter 2FA password:"
			m.input.EchoMode = textinput.EchoPassword
			m.input.Placeholder = "password"
		}
		m.input.SetValue("")
		return m, nil

	case AuthErrorMsg:
		m.err = msg.Text
		m.step = -2
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
	switch {
	case m.step == -2:
		s = fmt.Sprintf("Login error:\n\n%s\n\n(Press Ctrl+C to exit)", m.err)
	case m.step < 0:
		s = "Connecting...\n"
	default:
		s = fmt.Sprintf("%s\n\n%s\n\n%s", m.prompt, m.input.View(), m.err)
	}
	return tea.NewView(s)
}
