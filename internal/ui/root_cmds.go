package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func (m RootModel) markReadCmd() tea.Cmd {
	if m.st == nil || m.tgClient == nil || m.currentChatID == 0 || m.focus != FocusChat {
		return nil
	}
	chat, ok := m.st.GetChat(m.currentChatID)
	if !ok {
		return nil
	}
	maxID := m.chat.VisibleReadMaxID()
	if maxID <= 0 || maxID <= chat.ReadInboxMaxID {
		return nil
	}
	ctx := m.ctx
	client := m.tgClient
	peer := chat.Peer
	chatID := chat.ID
	return func() tea.Msg {
		if err := client.MarkRead(ctx, peer, maxID); err != nil {
			return StatusErrMsg{Text: "mark read failed: " + err.Error(), Sev: components.SeverityInfo}
		}
		return markReadDoneMsg{chatID: chatID, maxID: maxID}
	}
}

type readReactionsDoneMsg struct{ chatID int64 }

// readReactionsCmd sends messages.readReactions for a chat, then reports done so
// the store count can be reconciled. Nil when prerequisites are missing.
func (m RootModel) readReactionsCmd(chatID int64) tea.Cmd {
	if m.st == nil || m.tgClient == nil {
		return nil
	}
	chat, ok := m.st.GetChat(chatID)
	if !ok {
		return nil
	}
	ctx := m.ctx
	client := m.tgClient
	peer := chat.Peer
	return func() tea.Msg {
		if err := client.ReadReactions(ctx, peer); err != nil {
			return StatusErrMsg{Text: "read reactions failed: " + err.Error(), Sev: components.SeverityInfo}
		}
		return readReactionsDoneMsg{chatID: chatID}
	}
}

func logoTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return components.LogoTickMsg{}
	})
}

func requestBGColorCmd() tea.Cmd {
	return func() tea.Msg { return tea.RequestBackgroundColor() }
}

// enableColorSchemeReportsCmd turns on DEC private mode 2031 so a supporting
// terminal sends an unsolicited report whenever the OS light/dark color scheme
// changes (issue #148). Replaces the 2s background-color poll; terminals that
// do not support it ignore the sequence and rely on the focus-regain re-read.
func enableColorSchemeReportsCmd() tea.Cmd {
	return tea.Raw(ansi.SetModeLightDark)
}

func spinnerTickCmd() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
		return components.SpinnerTickMsg{}
	})
}

func typingDotsTickCmd() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return components.TypingDotsTickMsg{}
	})
}

func msgHighlightFadeCmd(serial int) tea.Cmd {
	return tea.Tick(components.HighlightFadeInterval, func(time.Time) tea.Msg {
		return msgHighlightFadeMsg{serial: serial}
	})
}

func chatHighlightFadeCmd(serial int) tea.Cmd {
	return tea.Tick(components.HighlightFadeInterval, func(time.Time) tea.Msg {
		return chatHighlightFadeMsg{serial: serial}
	})
}

func voiceTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return voiceTickMsg{} })
}

func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		str, err := clipboard.ReadAll()
		if err != nil || str == "" {
			return nil
		}
		return tea.PasteMsg{Content: str}
	}
}

// clipboardWrite writes text to the system clipboard. It is a variable so tests
// can stub out the external clipboard.
var clipboardWrite = clipboard.WriteAll

// messageCopiedMsg reports that a message's text was copied to the clipboard,
// so the status bar can confirm it.
type messageCopiedMsg struct{ ok bool }

// copyToClipboardCmd writes text to the clipboard off the update loop.
func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return messageCopiedMsg{ok: clipboardWrite(text) == nil}
	}
}

// SetClipboardWriterForTest swaps the clipboard writer and returns a restore
// func, so tests can capture copied text without touching the real clipboard.
func SetClipboardWriterForTest(fn func(string) error) func() {
	prev := clipboardWrite
	clipboardWrite = fn
	return func() { clipboardWrite = prev }
}
