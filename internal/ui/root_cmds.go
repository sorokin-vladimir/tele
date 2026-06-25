package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

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

func logoTickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg {
		return components.LogoTickMsg{}
	})
}

func requestBGColorCmd() tea.Cmd {
	return func() tea.Msg { return tea.RequestBackgroundColor() }
}

func bgColorPollCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tea.RequestBackgroundColor()
	})
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
