package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
	"github.com/stretchr/testify/require"
)

func TestLongMessageMetadataKeepsFrameInsideWindow(t *testing.T) {
	for _, width := range []int{90, 120, 140} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := NewRootModel(false).WithScreen(ScreenMain)
			m.chatList.SetWindow(0, 1, []project.ChatRow{{ID: 1, Title: "2\ufe0f\u20e3 " + strings.Repeat("Channel ", 20)}})
			m.chat.SetHeader(screens.ChatHeader{ChatID: 1, Title: "Synthetic group", IsGroup: true})
			filename := strings.Repeat("long_filename_", 20) + ".pdf"
			m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1, SenderName: strings.Repeat("Sender ", 40), Text: strings.Repeat("unbroken", 30), Date: time.Unix(0, 0), Media: &domain.MediaRef{Kind: domain.MediaFile, FileName: filename, Size: 2048}, Document: &domain.DocumentRef{ID: 1, FileName: filename}}})
			next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m = next.(RootModel)
			frame := strings.Split(m.View().Content, "\n")
			for row, line := range frame {
				require.LessOrEqual(t, ansi.StringWidth(line), width, "row %d crosses the window", row)
			}
		})
	}
}
