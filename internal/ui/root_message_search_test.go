package ui_test

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

func key(m ui.RootModel, text string) (ui.RootModel, tea.Cmd) {
	r := []rune(text)[0]
	nm, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: text})
	return nm.(ui.RootModel), cmd
}

// windowTitle is the message search window's title row as a person reads it.
func windowTitle(t *testing.T, m ui.RootModel) string {
	t.Helper()
	require.NotNil(t, m.MessageSearch(), "the message search window is open")
	return ansi.Strip(m.MessageSearch().View())
}

// typeAndAsk types query into the open window and presses Enter, returning the
// command the window's question became.
func typeAndAsk(t *testing.T, m ui.RootModel, query string) (ui.RootModel, tea.Cmd) {
	t.Helper()
	for _, r := range query {
		m, _ = key(m, string(r))
	}
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = nm.(ui.RootModel)
	require.NotNil(t, cmd, "Enter asks")
	// The window hands its question over; the root turns it into the call.
	nm, cmd = m.Update(cmd())
	return nm.(ui.RootModel), cmd
}

// The window's question is the owner's search, a page of 30 from where it
// says; the answer is shown in the window (#349).
func TestRoot_MessageSearch_AsksTheOwner(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	o := ownerOf(t, m)
	o.foundPage = project.SearchPage{Found: []project.FoundMessage{
		{Chat: "Alice", Message: domain.Message{ID: 9, ChatID: 1, SenderName: "Bob", Text: "the deploy failed", Date: time.Now()}},
	}}
	m, _ = key(m, "/")

	m, cmd := typeAndAsk(t, m, "deploy")
	m = runQuery(t, m, cmd)

	require.Len(t, o.messageSearches, 1)
	asked := o.messageSearches[0]
	assert.Equal(t, project.SearchScope{ChatID: 1}, asked.scope)
	assert.Equal(t, "deploy", asked.text)
	assert.Equal(t, project.SearchPosition{}, asked.after)
	assert.Equal(t, 30, asked.limit)
	assert.Contains(t, windowTitle(t, m), "the deploy failed")
}

// answerOf runs the call cmd makes and hands its answer to the model,
// returning what the model does with it.
func answerOf(t *testing.T, m ui.RootModel, cmd tea.Cmd) (ui.RootModel, tea.Cmd) {
	t.Helper()
	require.NotNil(t, cmd)
	nm, next := m.Update(cmd())
	return nm.(ui.RootModel), next
}

// A search Telegram refuses is a toast naming why, a wait included; a
// question cancelled by closing the window is nothing to report (#349).
func TestRoot_MessageSearch_AFailureIsAToast(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	o := ownerOf(t, m)
	o.foundErr = &telerr.Error{Kind: telerr.RateLimited, RetryAfter: 30 * time.Second}
	m, _ = key(m, "/")
	m, cmd := typeAndAsk(t, m, "deploy")

	_, next := answerOf(t, m, cmd)

	require.NotNil(t, next)
	toast, ok := next().(ui.StatusErrMsg)
	require.True(t, ok)
	assert.Contains(t, toast.Text, "30s")

	o.foundErr = context.Canceled
	m, cmd = typeAndAsk(t, m, "x")
	_, next = answerOf(t, m, cmd)
	assert.Nil(t, next, "a cancelled question is not reported")
}

// jumpTo opens found from the window, the way Enter on it does.
func jumpTo(t *testing.T, m ui.RootModel, found project.FoundMessage) ui.RootModel {
	t.Helper()
	nm, _ := m.Update(screens.MessageSearchJump{Found: found})
	return drainOwner(t, nm.(ui.RootModel))
}

// A result opens where it was found, at the message, highlighted once the
// window holds it: in the open chat the window moves onto it, another chat
// is opened on it, and a forum opens the topic (#349).
func TestRoot_MessageSearch_AResultOpensAtTheMessage(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	st.SetChat(domain.Chat{ID: 20, Title: "Go Club", IsForum: true, Peer: domain.Peer{ID: 20, Type: domain.PeerSuperGroup}})
	o := ownerOf(t, m)
	m, _ = key(m, "/")

	here := jumpTo(t, m, project.FoundMessage{Chat: "Alice", Message: domain.Message{ID: 500, ChatID: 1}})
	assert.Nil(t, here.MessageSearch(), "the window closes")
	w, ok := o.lastChatWindow()
	require.True(t, ok)
	assert.Equal(t, project.Anchor{Kind: project.AnchorMessage, MsgID: 500}, w.Anchor)

	there := jumpTo(t, m, project.FoundMessage{Chat: "Bob", Private: true, Message: domain.Message{ID: 77, ChatID: 2}})
	assert.Equal(t, int64(2), there.CurrentChatID())
	w, _ = o.lastChatWindow()
	assert.Equal(t, domain.HistoryKey{ChatID: 2}, domain.HistoryKey{ChatID: w.ChatID, TopicID: w.TopicID})
	assert.Equal(t, project.Anchor{Kind: project.AnchorMessage, MsgID: 77}, w.Anchor)

	topic := jumpTo(t, m, project.FoundMessage{Chat: "Go Club", Topic: "Releases", Message: domain.Message{ID: 90, ChatID: 20, TopicID: 12}})
	assert.Equal(t, int64(20), topic.CurrentChatID())
	w, _ = o.lastChatWindow()
	assert.Equal(t, domain.HistoryKey{ChatID: 20, TopicID: 12}, domain.HistoryKey{ChatID: w.ChatID, TopicID: w.TopicID})
	assert.Equal(t, project.Anchor{Kind: project.AnchorMessage, MsgID: 90}, w.Anchor)
}

// escape presses Esc in the open window and hands over what it asks for.
func escape(t *testing.T, m ui.RootModel) ui.RootModel {
	t.Helper()
	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	nm, _ = nm.(ui.RootModel).Update(cmd())
	return nm.(ui.RootModel)
}

// The window for a chat is kept until another chat is opened; the window for
// every chat is kept for as long as tele runs (#349).
func TestRoot_MessageSearch_IsRemembered(t *testing.T) {
	m, st := newRootWithOpenChat(t)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: domain.Peer{ID: 2, Type: domain.PeerUser}})
	m, _ = key(m, "/")
	m, _ = typeAndAsk(t, m, "deploy")
	m = escape(t, m)

	again, _ := key(m, "/")
	assert.Equal(t, "deploy", again.MessageSearch().Query(), "the same chat, the same window")
	again = escape(t, again)

	m = openChat(t, again, 2, "Bob")
	m = openChat(t, m, 1, "Alice")
	m, _ = key(m, "/")
	assert.Empty(t, m.MessageSearch().Query(), "another chat was opened in between")
	m = escape(t, m)

	m, _ = key(m, "g")
	m, _ = key(m, "/")
	m, _ = typeAndAsk(t, m, "build")
	m = escape(t, m)
	m = openChat(t, m, 2, "Bob")
	m, _ = key(m, "g")
	m, _ = key(m, "/")
	assert.Equal(t, "build", m.MessageSearch().Query(), "every chat's window outlives the chat")
}

// The terminal's paste goes into the open window's query.
func TestRoot_MessageSearch_TakesAPaste(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	m, _ = key(m, "/")

	nm, _ := m.Update(tea.PasteMsg{Content: "deploy"})

	assert.Equal(t, "deploy", nm.(ui.RootModel).MessageSearch().Query())
}

// While the window waits for an answer the spinner keeps going, and it stops
// once there is nothing to wait for (#349).
func TestRoot_MessageSearch_TheSpinnerTicksWhileItWaits(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	// The chat list loaded, so nothing but the window has a spinner to keep.
	nm, _ := m.Update(screens.TransitionToMainMsg{})
	m = drainOwner(t, nm.(ui.RootModel))
	_, idle := m.Update(components.SpinnerTickMsg{})
	require.Nil(t, idle, "nothing else ticks")
	m, _ = key(m, "/")
	m, cmd := typeAndAsk(t, m, "deploy")
	before := windowTitle(t, m)

	nm, next := m.Update(components.SpinnerTickMsg{})
	m = nm.(ui.RootModel)
	assert.NotEqual(t, before, windowTitle(t, m), "the spinner moved")
	assert.NotNil(t, next, "the tick goes on while the window waits")

	m, _ = answerOf(t, m, cmd)
	_, next = m.Update(components.SpinnerTickMsg{})
	assert.Nil(t, next, "nothing to wait for, no tick")
}

// The window follows the screen's size, the one in memory too.
func TestRoot_MessageSearch_FollowsTheScreenSize(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	m, _ = key(m, "/")

	nm, _ := m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	m = nm.(ui.RootModel)

	lines := strings.Split(windowTitle(t, m), "\n")
	assert.Equal(t, 46, ansi.StringWidth(lines[0]), "four columns narrower than the screen")
}

// Esc closes the window and cancels a question in flight.
func TestRoot_MessageSearch_EscCancels(t *testing.T) {
	m, _ := newRootWithOpenChat(t)
	o := ownerOf(t, m)
	m, _ = key(m, "/")
	m, cmd := typeAndAsk(t, m, "deploy")
	require.NotNil(t, cmd)
	drainMsgs(cmd()) // the call is made; its answer is not back yet
	require.Len(t, o.messageSearches, 1)
	require.NoError(t, o.messageSearches[0].ctx.Err())

	nm, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = nm.(ui.RootModel)
	require.NotNil(t, cmd)
	nm, _ = m.Update(cmd())
	m = nm.(ui.RootModel)

	assert.Nil(t, m.MessageSearch())
	assert.Error(t, o.messageSearches[0].ctx.Err(), "the call's context is cancelled")
}

// / in the chat pane searches the open chat's messages; g / searches every
// chat's from anywhere; / in the chat list still finds a chat (#349).
func TestRoot_MessageSearch_Opens(t *testing.T) {
	m, _ := newRootWithOpenChat(t)

	inChat, _ := key(m, "/")
	assert.Contains(t, windowTitle(t, inChat), "Messages in Alice")
	assert.False(t, inChat.SearchActive(), "not the chat search")

	everywhere, _ := key(m, "g")
	everywhere, _ = key(everywhere, "/")
	assert.Contains(t, windowTitle(t, everywhere), "Messages in all chats")

	list, _ := key(m, "1")
	list, _ = key(list, "/")
	assert.Nil(t, list.MessageSearch())
	assert.True(t, list.SearchActive(), "/ in the chat list finds a chat")
}
