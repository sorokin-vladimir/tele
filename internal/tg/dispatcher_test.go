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

func TestDispatcher_PrivateMessage_NilFromID_AssignsSenderID(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      101,
		PeerID:  &tg.PeerUser{UserID: 12345},
		Message: "incoming without from_id",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users:   []tg.UserClass{&tg.User{ID: 12345, FirstName: "Alice", LastName: "Smith"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, int64(12345), evt.Message.SenderID)
		assert.Equal(t, "Alice Smith", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestDispatcher_ChannelMessage_NilFromID_AssignsSenderID(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      102,
		PeerID:  &tg.PeerChannel{ChannelID: 789},
		Message: "channel post without from_id",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewChannelMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Chats:   []tg.ChatClass{&tg.Channel{ID: 789, Title: "Golang News"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, int64(789), evt.Message.SenderID)
		assert.Equal(t, "Golang News", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestDispatcher_GroupChatMessage_NilFromID_AssignsSenderID(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      103,
		PeerID:  &tg.PeerChat{ChatID: 456},
		Message: "group message without from_id",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Chats:   []tg.ChatClass{&tg.Chat{ID: 456, Title: "Team Chat"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, int64(456), evt.Message.SenderID)
		assert.Equal(t, "Team Chat", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestDispatcher_OutgoingMessage_NilFromID_SenderIDNotOverridden(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      104,
		PeerID:  &tg.PeerUser{UserID: 12345},
		Message: "outgoing message",
		Out:     true,
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users:   []tg.UserClass{&tg.User{ID: 12345, FirstName: "Alice"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, int64(0), evt.Message.SenderID)
		assert.Equal(t, "", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}
