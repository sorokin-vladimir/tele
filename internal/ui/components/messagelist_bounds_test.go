package components

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAttachmentAndSenderRowsStayInsideViewport(t *testing.T) {
	for _, width := range []int{30, 40, 80} {
		for _, kind := range []string{"file", "local-file", "sender", "reactions", "voice", "file-album"} {
			t.Run(fmt.Sprintf("%s/%d", kind, width), func(t *testing.T) {
				ml := NewMessageList(20, width)
				ml.SetIsGroup(true)
				msg := domain.Message{ID: 1, ChatID: 1, SenderName: "Sender", Text: "Caption", Date: time.Unix(0, 0)}
				filename := strings.Repeat("2\ufe0f\u20e3_filename_", 20) + ".pdf"
				switch kind {
				case "file", "file-album":
					msg.Media = &domain.MediaRef{Kind: domain.MediaFile, FileName: filename, Size: 2048}
					msg.Document = &domain.DocumentRef{ID: 1, FileName: filename}
				case "local-file":
					msg.LocalMedia = &domain.LocalMedia{Kind: domain.MediaFile, FileName: filename}
				case "voice":
					msg.Media = &domain.MediaRef{Kind: domain.MediaVoice, Duration: 60, Waveform: []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66}}
					msg.Document = &domain.DocumentRef{ID: 1}
					ml.playingVoiceID = 1
					ml.voicePosition = 12
				case "reactions":
					for i := 0; i < 12; i++ {
						msg.Reactions = append(msg.Reactions, domain.Reaction{Emoji: "\U0001f44d", Count: i + 1})
					}
				case "sender":
					msg.SenderName = strings.Repeat("Long sender ", 20)
				}
				ml.SetMessages([]domain.Message{msg})
				lines := ml.renderMessage(msg, true)
				if kind == "file-album" {
					second := msg
					second.ID = 2
					second.Document = &domain.DocumentRef{ID: 2, FileName: filename}
					lines = ml.renderGroupBubble([]domain.Message{msg, second}, true)
				} else if kind != "local-file" {
					require.Equal(t, len(lines), ml.msgHeight(msg), "height must match the rendered rows")
				}
				require.Contains(t, strings.Join(lines, "\n"), msg.Date.Format("15:04"), "timestamp remains visible")
				for row, line := range lines {
					require.LessOrEqual(t, lipgloss.Width(line), width, "row %d crosses the pane", row)
				}
			})
		}
	}
}
