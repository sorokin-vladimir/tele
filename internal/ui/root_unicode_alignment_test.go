package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// A keycap starts with ASCII but occupies two grapheme cells. The old styled
// string decoder consumed the digit alone and dropped its combining suffix,
// moving every cell after the title, including the adjacent chat pane.
func TestUnicodeChatTitlesKeepPaneBordersAligned(t *testing.T) {
	for _, width := range []int{80, 120, 140} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			const height = 30
			m := NewRootModel(false).WithScreen(ScreenMain)
			m.folderBar.SetFolders([]domain.FolderFilter{{ID: 1, Title: "2\ufe0f\u20e3 Folder"}})
			m.chatList.SetWindow(0, 4, []project.ChatRow{
				{ID: 1, Title: "Plain title", Unread: 5},
				{ID: 2, Title: "2\ufe0f\u20e3 Channel", Unread: 99},
				{ID: 3, Title: "Cafe\u0301", Unread: 5},
				{ID: 4, Title: "Team \U0001f441\u200d\U0001f5e8", Unread: 5},
			})
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
			m = next.(RootModel)
			frame := m.View().Content
			buf := uv.NewScreenBuffer(width, height)
			buf.Method = ansi.GraphemeWidth
			uv.NewStyledString(frame).Draw(buf, uv.Rect(0, 0, width, height))
			lay := computeLayout(width, height, m.chat.ComposerHeight(), true)
			edges := []int{0, foldersSidebarW - 1, foldersSidebarW, lay.messages.Left - 2, lay.messages.Left - 1, width - 1}
			for y := 1; y < height-2; y++ {
				for _, x := range edges {
					cell := buf.CellAt(x, y)
					require.NotNil(t, cell, "border at (%d,%d)", x, y)
					require.True(t, strings.ContainsAny(cell.Content, "│║█"), "border at (%d,%d) moved: %+q", x, y, cell.Content)
				}
			}
		})
	}
}

func TestRenderedTitlePreservesASCIIBasedGraphemes(t *testing.T) {
	for _, text := range []string{"2\ufe0f\u20e3", "e\u0301", "#\ufe0f\u20e3"} {
		buf := uv.NewScreenBuffer(12, 1)
		buf.Method = ansi.GraphemeWidth
		uv.NewStyledString(text+"|").Draw(buf, uv.Rect(0, 0, 12, 1))
		require.Equal(t, text, buf.CellAt(0, 0).Content)
		require.Equal(t, "|", buf.CellAt(ansi.StringWidth(text), 0).Content)
	}
}

func TestUnicodeModeReplyRepaintsAnOtherwiseUnchangedView(t *testing.T) {
	m := NewRootModel(false).WithScreen(ScreenMain)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(RootModel)
	before := m.View().Content
	next, cmd := m.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeReset})
	m = next.(RootModel)
	require.NotNil(t, cmd)
	require.Equal(t, before, m.View().Content, "wait for the clear before invalidating the frame")
	next, _ = m.Update(widthRepaintMsg{})
	m = next.(RootModel)
	after := m.View().Content
	require.NotEqual(t, before, after, "force a flush after the width method changes")
	require.Equal(t, ansi.Strip(before), ansi.Strip(after), "redraw adds no visible cells")
	require.Equal(t, after, m.View().Content, "idle frames stay identical")
	_, cmd = m.Update(tea.ModeReportMsg{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeReset})
	require.Nil(t, cmd, "a repeated mode report must not clear the screen again")
}

func TestUnsupportedUnicodeModeDoesNotRequestRepaint(t *testing.T) {
	for _, report := range []tea.ModeReportMsg{
		{Mode: ansi.ModeUnicodeCore, Value: ansi.ModeNotRecognized},
		{Mode: ansi.ModeSynchronizedOutput, Value: ansi.ModeReset},
	} {
		m := NewRootModel(false)
		next, cmd := m.updateInner(report)
		require.Nil(t, cmd)
		require.False(t, next.(RootModel).widthModeReported)
	}
}

func TestViewReplacesNarrowJoinedSymbolsWithoutChangingChatData(t *testing.T) {
	const title = "Team \U0001f441\u200d\U0001f5e8"
	m := NewRootModel(false).WithScreen(ScreenMain)
	m.chatList.SetWindow(0, 1, []project.ChatRow{{ID: 1, Title: title}})
	m.chat.SetHeader(screens.ChatHeader{ChatID: 1, Title: title})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(RootModel)
	content := ansi.Strip(m.View().Content)
	require.Contains(t, content, "Team ?")
	require.NotContains(t, content, "\U0001f441\u200d\U0001f5e8")
	require.Equal(t, title, m.chat.HeaderTitle())
	row, ok := m.chatList.ChatAtViewportRow(0)
	require.True(t, ok)
	require.Equal(t, title, row.Title)
}
