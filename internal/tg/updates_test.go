package tg

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func newTestDispatcher(t *testing.T, suppress func(int) bool) (*tg.UpdateDispatcher, chan store.Event, chan store.Event) {
	t.Helper()
	mustDeliver := make(chan store.Event, 4)
	droppable := make(chan store.Event, 4)
	d := tg.NewUpdateDispatcher()
	setupDispatcher(&d, mustDeliver, droppable, zap.NewNop(), suppress)
	return &d, mustDeliver, droppable
}

func noSuppress(int) bool { return false }

func TestSetupDispatcher_NewMessage(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      7,
		PeerID:  &tg.PeerUser{UserID: 99},
		Message: "test update",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users:   nil,
		Chats:   nil,
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, "test update", evt.Message.Text)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_ServiceMessageIgnored(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	svcMsg := &tg.MessageService{ID: 1}
	update := &tg.UpdateNewMessage{Message: svcMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
	})
	require.NoError(t, err)

	select {
	case <-mustDeliver:
		t.Fatal("unexpected event for service message")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSetupDispatcher_SuppressesID(t *testing.T) {
	suppressed := map[int]bool{7: true}
	dispatcher, mustDeliver, _ := newTestDispatcher(t, func(id int) bool {
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
	case <-mustDeliver:
		t.Fatal("suppressed message must not reach events channel")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSetupDispatcher_SuppressesID_ConsumeOnce(t *testing.T) {
	suppressed := map[int]bool{7: true}
	dispatcher, mustDeliver, _ := newTestDispatcher(t, func(id int) bool {
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
	case <-mustDeliver:
		t.Fatal("first delivery must be suppressed")
	case <-time.After(50 * time.Millisecond):
	}

	// Second delivery of the same ID: must pass through
	handle()
	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
	case <-time.After(time.Second):
		t.Fatal("second delivery must not be suppressed")
	}
}

func TestSetupDispatcher_UserStatusOnline_EmitsPresenceEvent(t *testing.T) {
	dispatcher, _, droppable := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	upd := &tg.UpdateUserStatus{
		UserID: 42,
		Status: &tg.UserStatusOnline{},
	}
	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{upd},
	})
	require.NoError(t, err)

	select {
	case evt := <-droppable:
		assert.Equal(t, store.EventUserPresence, evt.Kind)
		assert.Equal(t, int64(42), evt.ChatID)
		assert.True(t, evt.Online)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_UserStatusOffline_EmitsPresenceEvent(t *testing.T) {
	dispatcher, _, droppable := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	upd := &tg.UpdateUserStatus{
		UserID: 42,
		Status: &tg.UserStatusOffline{},
	}
	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{upd},
	})
	require.NoError(t, err)

	select {
	case evt := <-droppable:
		assert.Equal(t, store.EventUserPresence, evt.Kind)
		assert.Equal(t, int64(42), evt.ChatID)
		assert.False(t, evt.Online)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_PrivateMessage_NilFromID_SenderNameIsPeerName(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      20,
		PeerID:  &tg.PeerUser{UserID: 42},
		Message: "hello",
		Date:    int(time.Now().Unix()),
		// FromID is nil — incoming private message without explicit sender
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users:   []tg.UserClass{&tg.User{ID: 42, FirstName: "Alice"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, "Alice", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_ForwardedMessage_SetsForwardName(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromID(&tg.PeerUser{UserID: 77})
	rawMsg := &tg.Message{
		ID:      30,
		PeerID:  &tg.PeerUser{UserID: 42},
		FromID:  &tg.PeerUser{UserID: 42},
		Message: "forwarded",
		Date:    int(time.Now().Unix()),
	}
	rawMsg.SetFwdFrom(fwd)
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Users: []tg.UserClass{
			&tg.User{ID: 42, FirstName: "Alice"},
			&tg.User{ID: 77, FirstName: "Bob"},
		},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		require.NotNil(t, evt.Message.Forward)
		assert.Equal(t, "Bob", evt.Message.Forward.From)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_NewChannelMessage_SenderNameIsChannelTitle(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      10,
		PeerID:  &tg.PeerChannel{ChannelID: 500},
		Message: "breaking news",
		Date:    int(time.Now().Unix()),
		// FromID is nil — anonymous channel post
	}
	update := &tg.UpdateNewMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Chats:   []tg.ChatClass{&tg.Channel{ID: 500, Title: "Tech News"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, "Tech News", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_NewChannelMessage_EmitsEvent(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      11,
		PeerID:  &tg.PeerChannel{ChannelID: 600},
		Message: "supergroup message",
		Date:    int(time.Now().Unix()),
		// FromID is nil — anonymous channel post
	}
	update := &tg.UpdateNewChannelMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
		Chats:   []tg.ChatClass{&tg.Channel{ID: 600, Title: "Supergroup"}},
	})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventNewMessage, evt.Kind)
		assert.Equal(t, "supergroup message", evt.Message.Text)
		assert.Equal(t, int64(600), evt.Message.ChatID)
		assert.Equal(t, "Supergroup", evt.Message.SenderName)
	case <-time.After(time.Second):
		t.Fatal("no event received for channel message")
	}
}

func TestSetupDispatcher_NewChannelMessage_Suppressed(t *testing.T) {
	suppressed := map[int]bool{12: true}
	dispatcher, mustDeliver, _ := newTestDispatcher(t, func(id int) bool {
		return suppressed[id]
	})

	ctx := context.Background()
	rawMsg := &tg.Message{
		ID:      12,
		PeerID:  &tg.PeerChannel{ChannelID: 600},
		Message: "echo",
		Date:    int(time.Now().Unix()),
	}
	update := &tg.UpdateNewChannelMessage{Message: rawMsg, Pts: 1, PtsCount: 1}

	err := dispatcher.Handle(ctx, &tg.Updates{
		Updates: []tg.UpdateClass{update},
	})
	require.NoError(t, err)

	select {
	case <-mustDeliver:
		t.Fatal("suppressed channel message must not reach events channel")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestSetupDispatcher_NotifySettings_Muted_EmitsMuteEvent(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	var settings tg.PeerNotifySettings
	settings.SetMuteUntil(2147483647) // math.MaxInt32: muted indefinitely
	upd := &tg.UpdateNotifySettings{
		Peer:           &tg.NotifyPeer{Peer: &tg.PeerChannel{ChannelID: 500}},
		NotifySettings: settings,
	}
	err := dispatcher.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventMuteUpdate, evt.Kind)
		assert.Equal(t, int64(500), evt.ChatID)
		assert.True(t, evt.Muted)
	case <-time.After(time.Second):
		t.Fatal("no mute event received")
	}
}

func TestSetupDispatcher_NotifySettings_Unmuted_EmitsMuteEvent(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	var settings tg.PeerNotifySettings
	settings.SetMuteUntil(0)
	upd := &tg.UpdateNotifySettings{
		Peer:           &tg.NotifyPeer{Peer: &tg.PeerUser{UserID: 42}},
		NotifySettings: settings,
	}
	err := dispatcher.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}})
	require.NoError(t, err)

	select {
	case evt := <-mustDeliver:
		assert.Equal(t, store.EventMuteUpdate, evt.Kind)
		assert.Equal(t, int64(42), evt.ChatID)
		assert.False(t, evt.Muted)
	case <-time.After(time.Second):
		t.Fatal("no mute event received")
	}
}

func TestSetupDispatcher_NotifySettings_GlobalCategory_Ignored(t *testing.T) {
	dispatcher, mustDeliver, _ := newTestDispatcher(t, noSuppress)

	upd := &tg.UpdateNotifySettings{
		Peer:           &tg.NotifyUsers{},
		NotifySettings: tg.PeerNotifySettings{},
	}
	err := dispatcher.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}})
	require.NoError(t, err)

	select {
	case <-mustDeliver:
		t.Fatal("global category notify settings must not emit a per-chat mute event")
	case <-time.After(100 * time.Millisecond):
		// expected: no event
	}
}

func TestConvertTypingAction_Typing(t *testing.T) {
	assert.Equal(t, store.TypingActionTyping, convertTypingAction(&tg.SendMessageTypingAction{}))
}

func TestConvertTypingAction_UploadPhoto(t *testing.T) {
	assert.Equal(t, store.TypingActionUploadPhoto, convertTypingAction(&tg.SendMessageUploadPhotoAction{}))
}

func TestConvertTypingAction_Cancel(t *testing.T) {
	assert.Equal(t, store.TypingActionCancel, convertTypingAction(&tg.SendMessageCancelAction{}))
}

func TestConvertTypingAction_Unknown(t *testing.T) {
	assert.Equal(t, store.TypingActionUnknown, convertTypingAction(&tg.SendMessageGamePlayAction{}))
}

func TestSetupDispatcher_UserTyping_EmitsTypingEvent(t *testing.T) {
	dispatcher, _, droppable := newTestDispatcher(t, noSuppress)

	upd := &tg.UpdateUserTyping{
		UserID: 55,
		Action: &tg.SendMessageTypingAction{},
	}
	err := dispatcher.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}})
	require.NoError(t, err)

	select {
	case evt := <-droppable:
		assert.Equal(t, store.EventTyping, evt.Kind)
		assert.Equal(t, int64(55), evt.ChatID)
		assert.Equal(t, store.TypingActionTyping, evt.TypingAction)
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSetupDispatcher_ChatUserTyping_EmitsTypingEvent(t *testing.T) {
	dispatcher, _, droppable := newTestDispatcher(t, noSuppress)

	upd := &tg.UpdateChatUserTyping{
		ChatID: 100,
		Action: &tg.SendMessageUploadPhotoAction{},
	}
	err := dispatcher.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}})
	require.NoError(t, err)

	select {
	case evt := <-droppable:
		assert.Equal(t, store.EventTyping, evt.Kind)
		assert.Equal(t, int64(100), evt.ChatID)
		assert.Equal(t, store.TypingActionUploadPhoto, evt.TypingAction)
	case <-time.After(time.Second):
		t.Fatal("no event received")
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
