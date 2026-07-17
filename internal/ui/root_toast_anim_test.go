package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWithToastStack(t *testing.T) RootModel {
	t.Helper()
	st := store.NewMemory()
	m := NewRootModel(nil, st, 50, false).WithScreen(ScreenMain)
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return newM.(RootModel)
}

func TestRoot_StatusErr_ArmsToastTicker(t *testing.T) {
	m := rootWithToastStack(t)
	newM, cmd := m.Update(StatusErrMsg{Text: "network down", Sev: components.SeverityError})
	root := newM.(RootModel)
	assert.True(t, root.toastAnimTicking, "adding a toast should arm the toast ticker")
	require.NotNil(t, cmd)
}

func TestRoot_ToastAnimTick_AdvancesAndSettles(t *testing.T) {
	m := rootWithToastStack(t)
	newM, _ := m.Update(StatusErrMsg{Text: "boom", Sev: components.SeverityError})
	m = newM.(RootModel)

	var cmd tea.Cmd
	for i := 0; i < 50; i++ {
		newM, cmd = m.Update(toastAnimTickMsg{})
		m = newM.(RootModel)
		if !m.toasts.Animating() {
			break
		}
	}
	assert.False(t, m.toasts.Animating(), "the entering toast should settle")
	assert.False(t, m.toastAnimTicking, "the ticker flag should clear once settled")
	assert.Nil(t, cmd, "no further tick scheduled once settled")
}

func TestRoot_ClearStatusErr_LeavesThenRemoves(t *testing.T) {
	m := rootWithToastStack(t)
	newM, _ := m.Update(StatusErrMsg{Text: "boom", Sev: components.SeverityError})
	m = newM.(RootModel)
	settleRoot(&m)

	serial := m.toasts.LastSerialForTest()
	newM, _ = m.Update(ClearStatusErrMsg{Serial: serial})
	m = newM.(RootModel)
	require.Equal(t, 1, m.toasts.CountForTest(), "toast should still be present while leaving")

	settleRoot(&m)
	assert.Equal(t, 0, m.toasts.CountForTest(), "toast should be gone after the leave animation")
}

// settleRoot drives toast ticks until the stack stops animating.
func settleRoot(m *RootModel) {
	for i := 0; i < 100 && m.toasts.Animating(); i++ {
		newM, _ := m.Update(toastAnimTickMsg{})
		*m = newM.(RootModel)
	}
}
