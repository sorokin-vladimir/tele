package screens

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	internaltg "github.com/sorokin-vladimir/tele/internal/tg"
)

type AuthRequestMsg struct {
	Step internaltg.AuthStep
	Hint string
	// Err is why the step is asked again, shown under the field (#285).
	Err string
}
type AuthErrorMsg struct{ Text string }
type ConnectedMsg struct{}
type TransitionToMainMsg struct{}

// SlowConnectMsg arrives SlowConnectAfter into the login screen. If Telegram
// has not answered by then, the screen says what is worth checking (#283).
type SlowConnectMsg struct{}

// SlowConnectAfter is how long a connection may take before the screen stops
// saying only "connecting...". Nothing is given up when it passes: gotd keeps
// reconnecting on its own, and a network that comes back or a clock set right
// while the screen is up lets the same attempt through. A constant rather than a setting, since all it
// changes is a line of text.
const SlowConnectAfter = 10 * time.Second

// SlowConnectTick schedules the SlowConnectMsg.
func SlowConnectTick() tea.Cmd {
	return tea.Tick(SlowConnectAfter, func(time.Time) tea.Msg { return SlowConnectMsg{} })
}

// WaitForAuthRequest returns a Cmd that blocks until AuthFlow sends a request, an error, or ready closes.
func WaitForAuthRequest(af *internaltg.AuthFlow, ready <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		select {
		case req := <-af.Requests:
			return AuthRequestMsg{Step: req.Step, Hint: req.Hint, Err: req.Err}
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
	// slow is set when SlowConnectMsg arrives while still connecting.
	slow bool
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

// Connecting reports whether Telegram has not answered yet: no prompt has been
// asked for and nothing has failed. The error step is negative too, which is why
// this is not a sign test on the step.
func (m LoginModel) Connecting() bool { return m.step == -1 }

// Slow reports whether the connection is still being waited for past
// SlowConnectAfter.
func (m LoginModel) Slow() bool { return m.slow && m.Connecting() }

func (m LoginModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m LoginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SlowConnectMsg:
		if m.Connecting() {
			m.slow = true
		}
		return m, nil

	case AuthRequestMsg:
		m.step = msg.Step
		m.err = msg.Err
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
		// The text arrives complete, with the log and the quit key: which key
		// quits is the root's to know, not this screen's.
		s = m.err
	case m.step < 0:
		s = "Connecting...\n"
	default:
		s = fmt.Sprintf("%s\n\n%s\n\n%s", m.prompt, m.input.View(), m.err)
	}
	return tea.NewView(s)
}
