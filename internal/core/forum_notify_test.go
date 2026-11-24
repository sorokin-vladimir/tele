package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// showing builds the focus the policy takes: the histories some client shows.
func showing(hs ...domain.HistoryKey) func(domain.HistoryKey) bool {
	return func(h domain.HistoryKey) bool {
		for _, want := range hs {
			if h == want {
				return true
			}
		}
		return false
	}
}

func TestDecideNotification_InAForum(t *testing.T) {
	now := time.Now()
	forum := func(muted bool) domain.Chat {
		c := forumDialog()
		c.IsMuted = muted
		return c
	}
	topics := []domain.Topic{
		{ChatID: forumChat, ID: 12, Title: "Releases"},
		{ChatID: forumChat, ID: 30, Title: "Design", Mute: domain.TopicMuted},
		{ChatID: forumChat, ID: 31, Title: "Ops", Mute: domain.TopicUnmuted},
		{ChatID: forumChat, ID: domain.GeneralTopicID, Title: "General"},
	}
	msgIn := func(topic int) store.Event {
		return store.Event{Kind: store.EventNewMessage, Message: domain.Message{
			ID: 90, ChatID: forumChat, TopicID: topic, Text: "hi", Date: now.Add(-time.Second),
		}}
	}
	in := func(topic int) domain.HistoryKey { return domain.HistoryKey{ChatID: forumChat, TopicID: topic} }

	tests := []struct {
		name      string
		forumMute bool
		evt       store.Event
		focused   func(domain.HistoryKey) bool
		wantOK    bool
		wantTitle string
		wantTopic int
	}{
		{name: "a topic is titled under its forum", evt: msgIn(12), focused: showing(), wantOK: true, wantTitle: "Dev › Releases", wantTopic: 12},
		{name: "another topic of the forum on screen does not silence this one", evt: msgIn(12), focused: showing(in(31)), wantOK: true, wantTitle: "Dev › Releases", wantTopic: 12},
		{name: "the topic on screen is silent", evt: msgIn(12), focused: showing(in(12)), wantOK: false},
		{name: "a topic muted on its own is silent in an unmuted forum", evt: msgIn(30), focused: showing(), wantOK: false},
		{name: "a topic unmuted on its own speaks in a muted forum", forumMute: true, evt: msgIn(31), focused: showing(), wantOK: true, wantTitle: "Dev › Ops", wantTopic: 31},
		{name: "a topic following a muted forum is silent", forumMute: true, evt: msgIn(12), focused: showing(), wantOK: false},
		{name: "a message naming no topic is General's", evt: msgIn(0), focused: showing(), wantOK: true, wantTitle: "Dev › General", wantTopic: domain.GeneralTopicID},
		{name: "General on screen silences it", evt: msgIn(0), focused: showing(in(domain.GeneralTopicID)), wantOK: false},
		{name: "a topic nothing is known about is titled by its forum", evt: msgIn(77), focused: showing(), wantOK: true, wantTitle: "Dev", wantTopic: 77},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := store.NewMemory()
			st.SetChat(forum(tt.forumMute))
			st.SetTopicsPage(forumChat, topics)

			n, ok := decideNotification(st, tt.evt, tt.focused, true, now)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantTitle, n.Title)
				assert.Equal(t, tt.wantTopic, n.TopicID)
			}
		})
	}
}

// A reaction in a forum belongs to the topic of the message it is on.
func TestDecideNotification_AReactionInAForumIsItsMessagesTopics(t *testing.T) {
	now := time.Now()
	st := store.NewMemory()
	st.SetChat(forumDialog())
	st.SetTopicsPage(forumChat, []domain.Topic{{ChatID: forumChat, ID: 12, Title: "Releases"}})
	st.AppendMessage(domain.Message{ID: 40, ChatID: forumChat, TopicID: 12, IsOut: true, Date: now})
	evt := store.Event{Kind: store.EventReactionsUpdate, ChatID: forumChat, MsgID: 40,
		ReactionsUnread: new(true), ReactionEmoji: "👍", ReactionDate: now.Add(-time.Second)}

	_, onScreen := decideNotification(st, evt, showing(domain.HistoryKey{ChatID: forumChat, TopicID: 12}), true, now)
	n, ok := decideNotification(st, evt, showing(), true, now)

	assert.False(t, onScreen)
	assert.True(t, ok)
	assert.Equal(t, "Dev › Releases", n.Title)
}
