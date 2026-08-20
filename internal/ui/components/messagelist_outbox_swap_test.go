package components

import (
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sentText = "ЭТОТ-ТЕКСТ-РОВНО-ОДИН-РАЗ"

// sentEntry is a queued send Telegram has already confirmed: the request came
// back with ids, and the entry lives on until the messages land in state.
func sentEntry(ref string, ids ...int) domain.OutboxEntry {
	return domain.OutboxEntry{
		Ref: ref, ChatID: 1, State: domain.OutboxSending,
		Message:    &domain.OutboxMessage{Text: sentText},
		CreatedAt:  time.Unix(10, 0),
		SentMsgIDs: ids,
	}
}

func sentMessage(id int) domain.Message {
	return domain.Message{ID: id, ChatID: 1, Date: time.Unix(10, 0), IsOut: true, Text: sentText}
}

func countIn(view, s string) int {
	return strings.Count(xansi.Strip(view), s)
}

// The frame between ChatAppend and ChatOutbox holds the message and the entry
// that produced it. Drawing both doubles the text and shifts the pane by a
// bubble, then the next frame shifts it back - the blink of #226.
func TestOutbox_DeliveredEntryIsNotDrawnBesideItsMessage(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"}})
	ml.SetOutbox([]domain.OutboxEntry{sentEntry("r1", 10)})
	require.Equal(t, 1, countIn(ml.View(), sentText), "the queued bubble is what shows the send")

	// The message lands: the window grows, the entry has not been cleared yet.
	ml.SetMessagesKeepScroll([]domain.Message{
		{ID: 1, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"},
		sentMessage(10),
	})

	assert.Equal(t, 1, countIn(ml.View(), sentText), "the send is drawn twice")

	// The owner then publishes the queue without it. Nothing may move.
	before := ml.View()
	ml.SetOutbox(nil)
	assert.Equal(t, before, ml.View(), "clearing a hidden entry changed the frame")
}

// The entry is state, not a drawing decision: the pane keeps publishing what
// the owner sent it, so a retry or an error on that entry still has something
// to act on.
func TestOutbox_DeliveredEntryStaysInTheQueue(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{sentMessage(10)})
	ml.SetOutbox([]domain.OutboxEntry{sentEntry("r1", 10)})

	require.Len(t, ml.Outbox(), 1)
	assert.Equal(t, "r1", ml.Outbox()[0].Ref)
}

// Only the entry whose message is on screen goes. A send confirmed while the
// user reads history is still the only sign of itself.
func TestOutbox_EntryOutsideTheWindowIsStillDrawn(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"}})
	ml.SetOutbox([]domain.OutboxEntry{sentEntry("r1", 10)})

	assert.Equal(t, 1, countIn(ml.View(), sentText))
	assert.Equal(t, "r1", ml.SelectedOutboxRef(), "nothing has replaced it yet")
}

// An album's parts are applied one at a time, so the window can hold the first
// while the rest are still in flight. One landed part is enough: the entry
// draws the whole group as one bubble, and keeping it would double that part.
func TestOutbox_PartlyLandedAlbumDropsItsEntry(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{sentMessage(10)})
	ml.SetOutbox([]domain.OutboxEntry{sentEntry("r1", 10, 11, 12)})

	assert.Equal(t, 1, countIn(ml.View(), sentText))
}

// The cursor is parked on the entry the moment it is submitted. When the entry
// stops being drawn it has to follow the send into the window, or the selection
// indicator blinks out for exactly the frames the guard covers.
func TestOutbox_CursorFollowsTheSendIntoTheWindow(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetShowIndicator(true)
	ml.SetMessages([]domain.Message{{ID: 1, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"}})
	ml.SetOutbox([]domain.OutboxEntry{sentEntry("r1", 10)})
	require.Equal(t, "r1", ml.SelectedOutboxRef())

	ml.SetMessagesKeepScroll([]domain.Message{
		{ID: 1, ChatID: 1, Date: time.Unix(1, 0), Text: "раньше"},
		sentMessage(10),
	})
	ml.View()

	assert.Empty(t, ml.SelectedOutboxRef(), "the cursor still points at a bubble nobody draws")
	assert.Equal(t, 10, ml.SelectedMessageID(), "the cursor did not follow the send")
}

// A queued send that Telegram has not confirmed carries no ids and can never be
// hidden by mistake.
func TestOutbox_UnconfirmedEntryIsAlwaysDrawn(t *testing.T) {
	ml := NewMessageList(20, 60)
	ml.SetMessages([]domain.Message{sentMessage(10)})
	ml.SetOutbox([]domain.OutboxEntry{{
		Ref: "r1", ChatID: 1, State: domain.OutboxQueued,
		Message: &domain.OutboxMessage{Text: sentText}, CreatedAt: time.Unix(11, 0),
	}})

	assert.Equal(t, 2, countIn(ml.View(), sentText), "a message and an unrelated queued send")
}
