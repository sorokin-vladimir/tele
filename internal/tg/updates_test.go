package tg

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestSetupDispatcher_NewMessage(t *testing.T) {
	events := make(chan store.Event, 1)
	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, events, func(int) bool { return false })

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 99},
		Message: "test update",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	// Handle requires an UpdatesClass; wrap the update in tg.Updates.
	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users:   nil,
		Chats:   nil,
	})
	require.NoError(t, err)

	select {
	case evt := <-events:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, "test update", evt.Message.Text)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_ServiceMessageIgnored(t *testing.T) {
	events := make(chan store.Event, 1)
	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, events, func(int) bool { return false })

	ctx := context.Background()
	// UpdateNewMessage wrapping a service message should not emit an event.
	svcMsg := &tg.MessageService{ID: 1}
	update := &tg.UpdateNewMessage{Message: svcMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
	})
	require.NoError(t, err)

	select {
	case <-events:
		t.Fatal("unexpected event for service message")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSetupDispatcher_SuppressesID(t *testing.T) {
	events := make(chan store.Event, 1)
	dispatcher := tg.NewUpdateDispatcher()
	suppressed := map[int]bool{7: true}
	setupDispatcher(&dispatcher, events, func(id int) bool {
		return suppressed[id]
	})

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 1},
		Message: "echo",
		Date:    int(time.Now().Unix()),
	}
	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}},
	})
	require.NoError(t, err)

	select {
	case <-events:
		t.Fatal("suppressed message must not reach events channel")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSetupDispatcher_SuppressesID_ConsumeOnce(t *testing.T) {
	events := make(chan store.Event, 2)
	dispatcher := tg.NewUpdateDispatcher()
	suppressed := map[int]bool{7: true}
	setupDispatcher(&dispatcher, events, func(id int) bool {
		if suppressed[id] {
			delete(suppressed, id)
			return true
		}
		return false
	})

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 1},
		Message: "echo",
		Date:    int(time.Now().Unix()),
	}
	handle := func() {
		_ = dispatcher.Handle(ctx, &tg.Updates{
			Updates: []tg.UpdateClass{&tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}},
		})
	}

	// First delivery: suppressed
	handle()
	select {
	case <-events:
		t.Fatal("first delivery must be suppressed")
	case <-time.After(50 * time.Millisecond):
	}

	// Second delivery of the same ID: must pass through
	handle()
	select {
	case evt := <-events:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
	case <-time.After(time.Second):
		t.Fatal("second delivery must not be suppressed")
	}
}

func TestExtractPeerID(t *testing.T) {
	cases := []struct {
		name   string
		raw    tg.MessageClass
		wantID int64
	}{
		{
			name:   "PeerUser",
			raw:    &tg.Message{PeerID: &tg.PeerUser{UserID: 42}},
			wantID: 42,
		},
		{
			name:   "PeerChat",
			raw:    &tg.Message{PeerID: &tg.PeerChat{ChatID: 100}},
			wantID: 100,
		},
		{
			name:   "PeerChannel",
			raw:    &tg.Message{PeerID: &tg.PeerChannel{ChannelID: 200}},
			wantID: 200,
		},
		{
			name:   "ServiceMessage",
			raw:    &tg.MessageService{ID: 1},
			wantID: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPeerID(tc.raw)
			assert.Equal(t, tc.wantID, got)
		})
	}
}
