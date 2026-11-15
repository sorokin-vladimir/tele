package ui_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWithMessages(t *testing.T, msgs ...domain.Message) ui.RootModel {
	t.Helper()
	m, _ := newRootOnChat(t)
	m.Chat().SetMessages(msgs)
	m.View()
	return m
}

func press(t *testing.T, m ui.RootModel, msg tea.KeyPressMsg) (ui.RootModel, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(msg)
	return nm.(ui.RootModel), cmd
}

func textMessages(n int) []domain.Message {
	out := make([]domain.Message, n)
	for i := range out {
		out[i] = domain.Message{ID: i + 1, ChatID: 1, Text: "m" + string(rune('1'+i)), Date: time.Now().Add(time.Duration(i) * time.Minute)}
	}
	return out
}

func TestVisual_VThenJKThenYCopiesTheRangeOldestFirst(t *testing.T) {
	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()
	m := rootWithMessages(t, textMessages(4)...)

	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	assert.Equal(t, keys.ModeVisual, m.VimMode())
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	m, cmd := press(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.NotNil(t, cmd)
	drainMsgs(cmd())

	assert.Equal(t, "m2\n\nm3\n\nm4", got)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
}

func TestVisual_LowercaseVStartsTheSameMode(t *testing.T) {
	m := rootWithMessages(t, textMessages(3)...)
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'v', Text: "v"})
	assert.Equal(t, keys.ModeVisual, m.VimMode())
}

func TestVisual_EscLeavesVisualAndKeepsTheChatOpen(t *testing.T) {
	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()
	m := rootWithMessages(t, textMessages(3)...)

	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, keys.ModeNormal, m.VimMode())

	_, cmd := press(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.NotNil(t, cmd, "the chat is still open, so y copies the focused message")
	drainMsgs(cmd())
	assert.Equal(t, "m2", got)
}

func TestVisual_SecondVLeavesVisualWithoutCopying(t *testing.T) {
	called := false
	defer ui.SetClipboardWriterForTest(func(string) error { called = true; return nil })()
	m := rootWithMessages(t, textMessages(3)...)

	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	m, cmd := press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	assert.Nil(t, cmd)
	assert.Equal(t, keys.ModeNormal, m.VimMode())
	assert.False(t, called)
}

func TestVisual_AnotherActionLeavesVisualFirst(t *testing.T) {
	m := rootWithMessages(t, textMessages(3)...)
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'i', Text: "i"})
	assert.Equal(t, keys.ModeInsert, m.VimMode())
}

func TestVisual_YSkipsMediaOnlyMessagesInTheRange(t *testing.T) {
	var got string
	defer ui.SetClipboardWriterForTest(func(s string) error { got = s; return nil })()
	msgs := textMessages(3)
	msgs[1] = domain.Message{ID: 2, ChatID: 1, Date: msgs[1].Date, Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 1}}
	m := rootWithMessages(t, msgs...)

	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	m, _ = press(t, m, tea.KeyPressMsg{Code: 'k', Text: "k"})
	_, cmd := press(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	require.NotNil(t, cmd)
	drainMsgs(cmd())
	assert.Equal(t, "m1\n\nm3", got)
}

func TestVisual_StatusBarShowsVisualUntilItEnds(t *testing.T) {
	m := rootWithMessages(t, textMessages(3)...)
	assert.Contains(t, m.View().Content, "NORMAL")

	m, _ = press(t, m, tea.KeyPressMsg{Code: 'V', Text: "V"})
	assert.Contains(t, m.View().Content, "VISUAL")

	m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Contains(t, m.View().Content, "NORMAL")
}
