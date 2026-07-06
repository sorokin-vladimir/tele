package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenKey_OnSingleLinkMessage_OpensURL(t *testing.T) {
	var got string
	defer ui.SetURLOpenerForTest(func(u string) { got = u })()

	m, st := newRootOnChat(t, &mockTGClient{})
	st.AppendMessage(store.Message{ID: 3, ChatID: 1, Date: time.Now(),
		Text:     "see https://example.com now",
		Entities: []store.MessageEntity{{Type: "url", Offset: 4, Length: 19}}})
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	require.NotNil(t, cmd, "pressing o on a single-link message must open it")
	drainMsgs(cmd())
	assert.Equal(t, "https://example.com", got)
}

func TestOpenKey_MultipleTargets_OpensPickerThenTarget(t *testing.T) {
	var got string
	defer ui.SetURLOpenerForTest(func(u string) { got = u })()

	m, st := newRootOnChat(t, &mockTGClient{})
	// Photo with a caption link -> two open targets (Photo + link).
	st.AppendMessage(store.Message{ID: 5, ChatID: 1, Date: time.Now(),
		Text:     "pic https://example.com",
		Photo:    &store.PhotoRef{ID: 1, FullThumbSize: "y"},
		Entities: []store.MessageEntity{{Type: "url", Offset: 4, Length: 19}}})
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View()

	nm, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = nm.(ui.RootModel)
	require.True(t, m.OpenPickerOpen(), "two targets must open the picker")

	// Pick the second entry (the link) by digit; the picker emits a chosen msg
	// that the event loop routes back to open the target.
	nm, cmd := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m = nm.(ui.RootModel)
	assert.False(t, m.OpenPickerOpen(), "choosing closes the picker")
	require.NotNil(t, cmd)
	for _, msg := range drainMsgs(cmd()) {
		nm, cmd2 := m.Update(msg)
		m = nm.(ui.RootModel)
		if cmd2 != nil {
			drainMsgs(cmd2())
		}
	}
	assert.Equal(t, "https://example.com", got)
}

func TestOpenKey_OnPlainTextMessage_NoURLOpened(t *testing.T) {
	var called bool
	defer ui.SetURLOpenerForTest(func(string) { called = true })()

	m, st := newRootOnChat(t, &mockTGClient{})
	st.AppendMessage(store.Message{ID: 4, ChatID: 1, Date: time.Now(), Text: "just text"})
	nm, _ := m.Update(ui.ChatHistoryMsg{ChatID: 1, Messages: st.Messages(1)})
	m = nm.(ui.RootModel)
	m.View()

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	if cmd != nil {
		drainMsgs(cmd())
	}
	assert.False(t, called, "plain text has no link to open")
}
