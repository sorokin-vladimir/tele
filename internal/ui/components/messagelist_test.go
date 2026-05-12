package components_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
)

func makeMessages(n int) []store.Message {
	msgs := make([]store.Message, n)
	now := time.Now()
	for i := range msgs {
		msgs[i] = store.Message{ID: i + 1, ChatID: 1, Text: fmt.Sprintf("msg %d", i+1), Date: now}
	}
	return msgs
}

func TestMessageList_ShowsAll_WhenFewMessages(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(3))
	view := ml.View()
	assert.Contains(t, view, "msg 1")
	assert.Contains(t, view, "msg 3")
}

func TestMessageList_VirtualViewport(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(10))
	view := ml.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 3)
}

func TestMessageList_Count(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(5))
	assert.Equal(t, 5, ml.Count())
}

func TestMessageList_ScrollUp(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6))
	// after SetMessages viewStart is at tail; scroll up should move it back
	ml.ScrollUp()
	assert.Equal(t, 2, ml.ViewStart()) // was 3, now 2
}

func TestMessageList_AtTop_TrueWhenAtStart(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(2)) // fewer than height → viewStart = 0
	assert.True(t, ml.AtTop())
}

func TestMessageList_AtTop_FalseAfterScroll(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(6)) // viewStart = 3 after SetMessages
	assert.False(t, ml.AtTop())
}

func TestMessageList_AtTop_TrueAfterScrollingToStart(t *testing.T) {
	ml := components.NewMessageList(3, 40)
	ml.SetMessages(makeMessages(4)) // viewStart = 1
	ml.ScrollUp()                   // viewStart = 0
	assert.True(t, ml.AtTop())
}

func TestMessageList_OldestID_ReturnsFirstMessage(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	ml.SetMessages(makeMessages(5)) // IDs 1..5
	assert.Equal(t, 1, ml.OldestID())
}

func TestMessageList_OldestID_ZeroWhenEmpty(t *testing.T) {
	ml := components.NewMessageList(10, 40)
	assert.Equal(t, 0, ml.OldestID())
}

func TestMessageList_View_RendersEntityStyledText(t *testing.T) {
	ml := components.NewMessageList(5, 80)
	msgs := []store.Message{
		{
			ID:     1,
			ChatID: 1,
			Text:   "hello",
			Date:   time.Now(),
			Entities: []store.MessageEntity{
				{Type: "bold", Offset: 0, Length: 5},
			},
		},
	}
	ml.SetMessages(msgs)
	view := ml.View()
	assert.Contains(t, stripANSI(view), "hello")
}
