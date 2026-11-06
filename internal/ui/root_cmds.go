package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

// markReadCmd advances the read pointer to what the viewport shows. It follows
// the cursor, so it runs on nearly every keypress — which is why its failures
// go through errStatusBackground: the reader did not ask for this and cannot
// act on it, and it is re-attempted as the viewport moves again.
func (m RootModel) markReadCmd() tea.Cmd {
	if m.owner == nil || m.currentChatID == 0 || m.focus != FocusChat {
		return nil
	}
	maxID := m.chat.VisibleReadMaxID()
	if maxID <= 0 || maxID <= m.chat.InboxReadMaxID() {
		return nil
	}
	ctx, owner, h := m.ctx, m.owner, m.currentHistory()
	return func() tea.Msg {
		if err := owner.MarkRead(ctx, h, maxID); err != nil {
			return errStatusBackground("mark read", err)
		}
		return nil
	}
}

// readReactionsCmd tells the owner to mark this chat's reactions read. The
// owner clears the badge itself once Telegram confirms.
func (m RootModel) readReactionsCmd(h domain.HistoryKey) tea.Cmd {
	if m.owner == nil {
		return nil
	}
	ctx, owner := m.ctx, m.owner
	return func() tea.Msg {
		if err := owner.ReadReactions(ctx, h); err != nil {
			return errStatusBackground("read reactions", err)
		}
		return nil
	}
}

// readMentionsCmd tells the owner to mark this chat's mentions read.
func (m RootModel) readMentionsCmd(h domain.HistoryKey) tea.Cmd {
	if m.owner == nil {
		return nil
	}
	ctx, owner := m.ctx, m.owner
	return func() tea.Msg {
		if err := owner.ReadMentions(ctx, h); err != nil {
			return errStatusBackground("read mentions", err)
		}
		return nil
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

// requestCellSizeCmd asks the terminal for its cell size in pixels (CSI 16 t).
// The reply arrives as a uv.CellSizeEvent. It is only needed for sixel: foot
// answers the query but leaves the pixel fields of TIOCGWINSZ empty, so the
// ioctl probe finds nothing there and a sixel sized from a guessed cell would
// not line up with the cells it covers.
func requestCellSizeCmd() tea.Cmd {
	return tea.Raw("\x1b[16t")
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

func toastAnimTickCmd() tea.Cmd {
	return tea.Tick(components.ToastAnimInterval, func(time.Time) tea.Msg {
		return toastAnimTickMsg{}
	})
}

func readClipboardCmd() tea.Cmd {
	return func() tea.Msg {
		str, err := clipboardRead()
		if err != nil || str == "" {
			return nil
		}
		return tea.PasteMsg{Content: str}
	}
}

// clipboardRead reads text from the system clipboard. It is a variable so tests
// can stub out the external clipboard.
var clipboardRead = clipboard.ReadAll

// clipboardWrite writes text to the system clipboard. It is a variable so tests
// can stub out the external clipboard.
var clipboardWrite = clipboard.WriteAll

// SetClipboardReaderForTest swaps the clipboard text reader and returns a restore
// func, so tests can drive the text-paste fallback without the real clipboard.
func SetClipboardReaderForTest(fn func() (string, error)) func() {
	prev := clipboardRead
	clipboardRead = fn
	return func() { clipboardRead = prev }
}

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
