package components

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
)

const newestMarker = "САМОЕ-НОВОЕ-СООБЩЕНИЕ"

// tailMessages is a screenful of history ending in a message that must always
// be reachable.
func tailMessages() []domain.Message {
	now := time.Now()
	dump := "tcp        0      0 127.0.0.1:6379          0.0.0.0:*               LISTEN\n" +
		"tcp        0      0 0.0.0.0:8443            0.0.0.0:*               LISTEN\n" +
		"tcp6       0      0 :::5432                 :::*                    LISTEN"
	urls := []domain.MessageEntity{
		{Type: "url", Offset: 20, Length: 14},
		{Type: "url", Offset: 95, Length: 12},
	}
	return []domain.Message{
		{ID: 1, Date: now, Text: "первое сообщение в этой переписке"},
		{ID: 2, Date: now, Text: dump, Entities: urls},
		{ID: 3, Date: now, Text: "короткий ответ", IsOut: true},
		{ID: 4, Date: now, Text: dump, Entities: urls},
		{ID: 5, Date: now, Text: "ещё одна реплика подлиннее, чтобы перенос сработал"},
		{ID: 6, Date: now, Text: "ок", IsOut: true},
		{ID: 7, Date: now, Text: newestMarker},
	}
}

// The estimate and the render must agree, because every scroll clamp is built
// on the estimate while the frame is built by the render. Where they disagree
// the viewport anchors to a bottom that is not the bottom (#231).
func TestItemHeightMatchesRenderedLines(t *testing.T) {
	now := time.Now()
	long := "один два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать"
	entityTypes := []string{
		"bold", "italic", "code", "pre", "underline", "strike",
		"text_url", "url", "email", "mention", "hashtag", "bot_command",
	}

	msgs := []domain.Message{
		{ID: 1, Date: now, Text: "ok"},
		{ID: 2, Date: now, Text: long},
		{ID: 3, Date: now, Text: "первый абзац\n\nвторой абзац этого сообщения\n\nтретий"},
		{ID: 4, Date: now, Text: long, IsOut: true},
		{ID: 5, Date: now, Text: long, SenderName: "Пользователь С Очень Длинным Именем Которое Шире Пузыря"},
		{ID: 6, Date: now, Text: long, Reactions: []domain.Reaction{
			{Emoji: "👍", Count: 12}, {Emoji: "🔥", Count: 8}, {Emoji: "💀", Count: 3},
			{Emoji: "🤖", Count: 2}, {Emoji: "✅", Count: 1},
		}},
		{ID: 7, Date: now, Text: long, EditDate: &now},
		{ID: 8, Date: now, Text: long, ReplyToMsgID: 2},
		{ID: 9, Date: now, Text: long, Forward: &domain.ForwardInfo{From: "Alice"}},
		{ID: 10, Date: now, Text: long, Media: &domain.MediaRef{Kind: domain.MediaFile}},
		{ID: 11, Date: now, Text: strings.Repeat("a", 200)},
		{ID: 12, Date: now, Text: "abcdefghijklmn opqrstuvwxyzab cdefghijklmnop qrstuvwxyzabcd"},
	}
	// One message per entity type, each spanning a wrap point, since the styling
	// is what used to move the breaks.
	for i, typ := range entityTypes {
		msgs = append(msgs, domain.Message{
			ID: 100 + i, Date: now, Text: long,
			Entities: []domain.MessageEntity{
				{Type: typ, Offset: 20, Length: 30, URL: "https://example.com"},
			},
		})
	}

	for _, group := range []bool{false, true} {
		for w := 20; w <= 200; w += 3 {
			ml := NewMessageList(30, w)
			ml.isGroup = group
			ml.SetMessages(msgs)
			for i := range ml.items {
				if ml.items[i].kind != itemMessage {
					continue
				}
				est := ml.itemHeight(i)
				got := len(ml.renderItem(i, false))
				if est != got {
					t.Fatalf("group=%v w=%d item %d (msg %d): itemHeight=%d rendered=%d",
						group, w, i, ml.items[i].msg.ID, est, got)
				}
			}
		}
	}
}

// The sweep from #231: scrolling to the bottom and then pressing down must
// leave the newest message on screen, at every terminal size.
func TestView_NewestMessageIsAlwaysReachable(t *testing.T) {
	msgs := tailMessages()

	for w := 80; w <= 200; w += 4 {
		for h := 16; h <= 48; h += 2 {
			ml := NewMessageList(h, w)
			ml.SetMessages(msgs)
			ml.ScrollToBottom()
			ml.ScrollDownBy(300)

			if !strings.Contains(xansi.Strip(ml.View()), newestMarker) {
				t.Fatalf("w=%d h=%d: the newest message is not on screen", w, h)
			}
		}
	}
}

// The estimates agree with the render today, and the guard above holds them
// there. This is about what happens when they do not: a frame that ran out of
// room before the last item, while the clamp believes it is at the bottom, used
// to be resolved by trimming from the bottom - dropping exactly the newest rows
// and leaving nothing on screen to say so, with every scroll key a no-op.
func TestView_KeepsTheNewestRowsWhenAnEstimateFallsShort(t *testing.T) {
	msgs := tailMessages()
	ml := NewMessageList(20, 100)
	ml.SetMessages(msgs)

	// Every item renders one line taller than the clamp believes.
	for i := range ml.items {
		ml.heightCache[i] = ml.itemHeight(i) - 1
	}
	ml.ScrollToBottom()

	out := xansi.Strip(ml.View())

	assert.Contains(t, out, newestMarker, "the newest message was trimmed off the frame")
}
