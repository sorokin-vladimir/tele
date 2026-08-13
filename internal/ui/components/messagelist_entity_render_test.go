package components

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
)

// The bubble is a box: every row of it ends at the same column. Block padding
// injected by styling a run that spanned a newline made some rows wider than
// others, which is the ragged right border in #232.
func TestRenderMessage_EntityRowsAllEndAtTheSameColumn(t *testing.T) {
	text := "первый абзац этого сообщения довольно длинный\n\n" +
		"нашла две уязвимости в движке V8 из Chrome\n\n" +
		"более 400 проблем повышения привилегий в ядре одной из ОС"
	msg := domain.Message{
		ID: 1, Date: time.Now(), Text: text,
		Entities: []domain.MessageEntity{
			{Type: "bold", Offset: 52, Length: 14},
			{Type: "bold", Offset: 96, Length: 16},
		},
	}

	for _, w := range []int{60, 80, 100, 140} {
		ml := NewMessageList(20, w)
		ml.SetMessages([]domain.Message{msg})

		lines := ml.renderMessage(msg, false)

		want := lipgloss.Width(lines[0])
		for i, line := range lines {
			assert.Equalf(t, want, lipgloss.Width(line),
				"w=%d row %d is a different width\nrow 0: %q\nrow %d: %q",
				w, i, xansi.Strip(lines[0]), i, xansi.Strip(line))
		}
	}
}

// The wrap must break where the plain text breaks. Padding inside a styled run
// pushed tokens onto rows of their own, which is how a lone word ended up
// below the line it belonged to.
func TestRenderMessage_EntityTextWrapsLikePlainText(t *testing.T) {
	text := "первый абзац этого сообщения довольно длинный\n\n" +
		"нашла две уязвимости в движке V8 из Chrome"
	plain := domain.Message{ID: 1, Date: time.Now(), Text: text}
	styled := domain.Message{
		ID: 1, Date: time.Now(), Text: text,
		Entities: []domain.MessageEntity{{Type: "bold", Offset: 52, Length: 14}},
	}

	for _, w := range []int{60, 80, 100, 140} {
		ml := NewMessageList(20, w)
		ml.SetMessages([]domain.Message{plain})
		wantRows := make([]string, 0)
		for _, l := range ml.renderMessage(plain, false) {
			wantRows = append(wantRows, xansi.Strip(l))
		}

		ml.SetMessages([]domain.Message{styled})
		gotRows := make([]string, 0)
		for _, l := range ml.renderMessage(styled, false) {
			gotRows = append(gotRows, xansi.Strip(l))
		}

		assert.Equalf(t, wantRows, gotRows, "w=%d: styling changed the layout", w)
	}
}
