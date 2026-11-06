package components

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

func TestAttachmentAndSenderRowsStayInsideViewport(t *testing.T) {
	for _, width := range []int{30, 40, 80} {
		for _, kind := range []string{"file", "local-file", "sender", "reactions", "voice", "file-album"} {
			t.Run(fmt.Sprintf("%s/%d", kind, width), func(t *testing.T) {
				ml := NewMessageList(20, width)
				ml.SetIsGroup(true)
				msg := domain.Message{ID: 1, ChatID: 1, SenderName: "Sender", Text: "Caption", Date: time.Unix(0, 0)}
				filename := strings.Repeat("long_filename_", 20) + ".pdf"
				switch kind {
				case "file", "file-album":
					msg.Media = &domain.MediaRef{Kind: domain.MediaFile, FileName: filename, Size: 2048}
					msg.Document = &domain.DocumentRef{ID: 1, FileName: filename}
				case "local-file":
					msg.LocalMedia = &domain.LocalMedia{Kind: domain.MediaFile, FileName: filename}
				case "voice":
					msg.Media = &domain.MediaRef{Kind: domain.MediaVoice, Duration: 60, Waveform: []byte(strings.Repeat("\xff", 50))}
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
					require.Equal(t, len(lines), ml.groupHeight([]domain.Message{msg, second}), "album height matches rendered rows")
				} else {
					require.Equal(t, len(lines), ml.msgHeight(msg), "height must match the rendered rows")
				}
				require.Contains(t, strings.Join(lines, "\n"), msg.Date.Format("15:04"), "timestamp remains visible")
				for row, line := range lines {
					require.LessOrEqual(t, lipgloss.Width(line), ml.maxContentWidth()+bubbleFrameWidth, "row %d crosses the pane", row)
				}
			})
		}
	}
}

func TestMetadataWithKeycapsStaysWithinPane(t *testing.T) {
	for _, width := range []int{30, 40, 80} {
		ml := NewMessageList(20, width)
		ml.SetIsGroup(true)
		msg := domain.Message{ID: 1, SenderName: strings.Repeat("2\ufe0f\u20e3", 30), Date: time.Unix(0, 0), Media: &domain.MediaRef{Kind: domain.MediaFile, FileName: strings.Repeat("#\ufe0f\u20e3", 40)}}
		for i := 0; i < 12; i++ {
			msg.Reactions = append(msg.Reactions, domain.Reaction{Emoji: "2\ufe0f\u20e3", Count: i + 1, IsChosen: true})
		}
		for _, line := range ml.renderMessage(msg, true) {
			require.LessOrEqual(t, lipgloss.Width(line), ml.maxContentWidth()+bubbleFrameWidth)
		}
	}
}

func TestTruncateMetadataPreservesStylesAndClusters(t *testing.T) {
	text := "\x1b[31m2\ufe0f\u20e3abc\x1b[m"
	require.Equal(t, "\x1b[31m2\ufe0f\u20e3…\x1b[m", TruncateText(text, 3))
	require.Equal(t, text, TruncateText(text, 10))
	require.Empty(t, TruncateText(text, 0))
	const open = "\x1b]8;;https://example.org\x1b\\"
	const close = "\x1b]8;;\x1b\\"
	require.Equal(t, open+"ab…"+close, TruncateText(open+"abcdef"+close, 3))
}

func TestBareMediaMetadataUsesContentBudget(t *testing.T) {
	for _, kind := range []domain.MediaKind{domain.MediaSticker, domain.MediaVideoNote} {
		for _, width := range []int{30, 40, 80} {
			ml := NewMessageList(20, width)
			ml.SetIsGroup(true)
			ml.SetImageMode(media.ModeKitty)
			msg := domain.Message{ID: 1, SenderName: strings.Repeat("Sender ", 30), Date: time.Unix(0, 0), Media: &domain.MediaRef{Kind: kind, Duration: 12}, Document: &domain.DocumentRef{ID: 42, MimeType: "image/webp", ThumbSize: "m"}}
			for i := 0; i < 12; i++ {
				msg.Reactions = append(msg.Reactions, domain.Reaction{Emoji: "2\ufe0f\u20e3", Count: 123456 + i})
			}
			ml.imageCache.Add(42, image.NewRGBA(image.Rect(0, 0, 50, 50)))
			require.True(t, ml.isBareMedia(msg))
			lines := ml.renderBareMedia(msg, false)
			require.Len(t, lines, ml.msgHeight(msg))
			for _, line := range lines {
				require.LessOrEqual(t, lipgloss.Width(line), ml.maxContentWidth())
			}
			require.Contains(t, strings.Join(lines, "\n"), msg.Date.Format("15:04"))
		}
	}
}
