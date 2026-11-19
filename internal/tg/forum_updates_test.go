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

func topicsEvent(t *testing.T, upd tg.UpdateClass) store.Event {
	t.Helper()
	d, mustDeliver, _ := newTestDispatcher(t, noSuppress)
	require.NoError(t, d.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}}))
	select {
	case evt := <-mustDeliver:
		return evt
	case <-time.After(time.Second):
		t.Fatal("no event received")
		return store.Event{}
	}
}

func TestDispatcher_ATopicCreatedIsReadAgain(t *testing.T) {
	evt := topicsEvent(t, &tg.UpdateNewChannelMessage{Pts: 1, PtsCount: 1, Message: &tg.MessageService{
		ID: 77, PeerID: &tg.PeerChannel{ChannelID: 50},
		Action: &tg.MessageActionTopicCreate{Title: "Design"},
	}})

	assert.Equal(t, store.EventTopicsChanged, evt.Kind)
	assert.Equal(t, int64(50), evt.ChatID)
	assert.Equal(t, []int{77}, evt.MsgIDs, "a topic is named by the message that opened it")
}

func TestDispatcher_ATopicEditedIsReadAgain(t *testing.T) {
	evt := topicsEvent(t, &tg.UpdateNewChannelMessage{Pts: 1, PtsCount: 1, Message: &tg.MessageService{
		ID: 78, PeerID: &tg.PeerChannel{ChannelID: 50},
		ReplyTo: &tg.MessageReplyHeader{ForumTopic: true, ReplyToMsgID: 12},
		Action:  &tg.MessageActionTopicEdit{Title: "Releases 2"},
	}})

	assert.Equal(t, store.EventTopicsChanged, evt.Kind)
	assert.Equal(t, []int{12}, evt.MsgIDs)
}

func TestDispatcher_ATopicReadIsReadAgain(t *testing.T) {
	in := topicsEvent(t, &tg.UpdateReadChannelDiscussionInbox{ChannelID: 50, TopMsgID: 12, ReadMaxID: 90})
	assert.Equal(t, store.EventTopicsChanged, in.Kind)
	assert.Equal(t, int64(50), in.ChatID)
	assert.Equal(t, []int{12}, in.MsgIDs)

	out := topicsEvent(t, &tg.UpdateReadChannelDiscussionOutbox{ChannelID: 50, TopMsgID: 12, ReadMaxID: 91})
	assert.Equal(t, store.EventTopicsChanged, out.Kind)
	assert.Equal(t, []int{12}, out.MsgIDs)
}

// A topic's notification setting changed on another device is read again with
// the topic, like everything else about it.
func TestDispatcher_ATopicsMuteIsReadAgain(t *testing.T) {
	evt := topicsEvent(t, &tg.UpdateNotifySettings{
		Peer: &tg.NotifyForumTopic{Peer: &tg.PeerChannel{ChannelID: 50}, TopMsgID: 12},
	})

	assert.Equal(t, store.EventTopicsChanged, evt.Kind)
	assert.Equal(t, int64(50), evt.ChatID)
	assert.Equal(t, []int{12}, evt.MsgIDs)
}

// A draft is kept per topic, and Telegram says which.
func TestDispatcher_ADraftNamesItsTopic(t *testing.T) {
	evt := topicsEventOn(t, &tg.UpdateDraftMessage{
		Peer: &tg.PeerChannel{ChannelID: 50}, TopMsgID: 12, Draft: &tg.DraftMessage{Message: "half"},
	}, false)

	assert.Equal(t, store.EventDraftMessage, evt.Kind)
	assert.Equal(t, 12, evt.TopicID)
	assert.Equal(t, "half", evt.Draft)
}

// Somebody typing in a forum is typing in one topic of it.
func TestDispatcher_TypingNamesItsTopic(t *testing.T) {
	evt := topicsEventOn(t, &tg.UpdateChannelUserTyping{
		ChannelID: 50, TopMsgID: 12, FromID: &tg.PeerUser{UserID: 7}, Action: &tg.SendMessageTypingAction{},
	}, true)

	assert.Equal(t, store.EventTyping, evt.Kind)
	assert.Equal(t, 12, evt.TopicID)
}

// topicsEventOn dispatches one update and returns the event it produced on the
// channel it belongs to: typing is droppable, everything else must arrive.
func topicsEventOn(t *testing.T, upd tg.UpdateClass, droppableChan bool) store.Event {
	t.Helper()
	d, mustDeliver, droppable := newTestDispatcher(t, noSuppress)
	require.NoError(t, d.Handle(context.Background(), &tg.Updates{Updates: []tg.UpdateClass{upd}}))
	ch := mustDeliver
	if droppableChan {
		ch = droppable
	}
	select {
	case evt := <-ch:
		return evt
	case <-time.After(time.Second):
		t.Fatal("no event received")
		return store.Event{}
	}
}

// Only the first page says in what order topics are pinned, so a change to
// what is pinned reads that page again rather than the topics it names.
func TestDispatcher_APinChangeReadsTheFirstPageAgain(t *testing.T) {
	one := topicsEvent(t, &tg.UpdatePinnedForumTopic{Peer: &tg.PeerChannel{ChannelID: 50}, TopicID: 12, Pinned: true})
	assert.Equal(t, store.EventTopicsChanged, one.Kind)
	assert.Equal(t, int64(50), one.ChatID)
	assert.True(t, one.TopicsPage)

	all := topicsEvent(t, &tg.UpdatePinnedForumTopics{Peer: &tg.PeerChannel{ChannelID: 50}, Order: []int{12, 1}})
	assert.True(t, all.TopicsPage)
}

func TestDispatcher_BotForumInboxReadTargetsTopic(t *testing.T) {
	evt := topicsEventOn(t, &tg.UpdateReadHistoryInbox{
		Peer:     &tg.PeerUser{UserID: 42},
		MaxID:    80,
		TopMsgID: 12,
	}, false)

	assert.Equal(t, store.EventTopicsChanged, evt.Kind)
	assert.Equal(t, int64(42), evt.ChatID)
	assert.Equal(t, []int{12}, evt.MsgIDs)
}

func TestDispatcher_BotForumTypingNamesTopic(t *testing.T) {
	evt := topicsEventOn(t, &tg.UpdateUserTyping{
		UserID:   42,
		TopMsgID: 12,
		Action:   &tg.SendMessageTypingAction{},
	}, true)

	assert.Equal(t, store.EventTyping, evt.Kind)
	assert.Equal(t, int64(42), evt.ChatID)
	assert.Equal(t, 12, evt.TopicID)
}
