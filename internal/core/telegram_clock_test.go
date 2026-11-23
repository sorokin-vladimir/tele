package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// sentAt is when Telegram says the message in these tests was sent, by its own
// clock.
var sentAt = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// skewedOwner is an owner whose local clock reads skew off Telegram's, with the
// connection reporting the offset the way gotd does: server time minus local.
// It is one second after sentAt by Telegram's clock.
func skewedOwner(t *testing.T, skew time.Duration, knowsOffset bool) (*Owner, *mockNotifier) {
	t.Helper()
	n := &mockNotifier{}
	o, st := newTestOwnerNotified(t, n)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob"})
	o.now = func() time.Time { return sentAt.Add(time.Second + skew) }
	if knowsOffset {
		o.SetTelegramOffset(func() time.Duration { return -skew })
	}
	return o, n
}

func arriveAt(o *Owner, date time.Time) {
	testClient(o)
	o.handleEvent(store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ID: 1, ChatID: 2, Text: "hello there", Date: date}})
}

func TestTelegramClock_AMessageJustSentNotifiesWithTheLocalClockAhead(t *testing.T) {
	o, n := skewedOwner(t, 5*time.Minute, true)

	arriveAt(o, sentAt)

	assert.Len(t, n.calls, 1, "a message one second old by Telegram's clock was judged by the local one")
}

func TestTelegramClock_AnOldMessageStaysSilentWithTheLocalClockBehind(t *testing.T) {
	o, n := skewedOwner(t, -5*time.Minute, true)

	arriveAt(o, sentAt.Add(-time.Minute))

	assert.Empty(t, n.calls, "a message a minute old by Telegram's clock looked fresh by the local one")
}

func TestTelegramClock_WithNoOffsetTheLocalClockDecides(t *testing.T) {
	o, n := skewedOwner(t, 0, false)

	arriveAt(o, sentAt)

	assert.Len(t, n.calls, 1)
}
