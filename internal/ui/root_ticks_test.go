package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// idleMainModel returns a model on the main screen with chats loaded and a chat
// open showing text messages: nothing is animating (no active spinner, no idle
// logo), so both the spinner and logo tick loops should go to sleep.
func idleMainModel() RootModel {
	m := NewRootModel(50, false)
	m.screen = ScreenMain
	m.chat.SetSize(80, 12)
	m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	m.chatList.SetWindow(0, 1, []project.ChatRow{{ID: 1}})
	return m
}

func TestRoot_SpinnerTick_IdleMain_StopsTicking(t *testing.T) {
	m := idleMainModel()
	_, cmd := m.Update(components.SpinnerTickMsg{})
	assert.Nil(t, cmd, "idle main screen must not reschedule the spinner tick")
}

func TestRoot_SpinnerTick_LoadingChats_KeepsTicking(t *testing.T) {
	m := NewRootModel(50, false)
	m.screen = ScreenMain
	m.chat.SetSize(80, 12)
	m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Text: "hi", Date: time.Now()}})
	// No chats set: the chat list shows its "Loading chats..." spinner, so the
	// spinner loop must stay alive.
	_, cmd := m.Update(components.SpinnerTickMsg{})
	assert.NotNil(t, cmd, "while loading chats the spinner must keep ticking")
}

func TestRoot_LogoTick_ChatOpen_StopsTicking(t *testing.T) {
	m := idleMainModel()
	_, cmd := m.Update(components.LogoTickMsg{})
	assert.Nil(t, cmd, "with a chat open the idle logo is hidden, so the logo tick must stop")
}

func TestRoot_LogoTick_LoginScreen_KeepsTicking(t *testing.T) {
	m := NewRootModel(50, false)
	m.screen = ScreenLogin
	_, cmd := m.Update(components.LogoTickMsg{})
	assert.NotNil(t, cmd, "the login splash logo animates, so the logo tick must keep going")
}

// When an idle loop has gone to sleep, a state change that needs animation must
// re-arm it. Opening the app on the main screen with no chats yet loaded shows
// the idle logo in the chat pane: the logo loop must (re)start.
func TestRoot_EnsureAnimation_RestartsLogoWhenIdleLogoVisible(t *testing.T) {
	m := NewRootModel(50, false)
	m.screen = ScreenMain
	m.chat.SetSize(80, 12)
	// No messages and no chat open: the chat pane renders the idle logo.
	_, cmd := m.Update(components.SpinnerTickMsg{})
	assert.NotNil(t, cmd, "idle logo visible: the logo tick must be (re)started")
}
