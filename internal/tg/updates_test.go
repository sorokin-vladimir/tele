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
	setupDispatcher(&dispatcher, events)

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
	setupDispatcher(&dispatcher, events)

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
