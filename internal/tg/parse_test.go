package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReactionsHaveUnread(t *testing.T) {
	none := tg.MessageReactions{
		RecentReactions: []tg.MessagePeerReaction{
			{Unread: false, Reaction: &tg.ReactionEmoji{Emoticon: "👍"}},
		},
	}
	assert.False(t, reactionsHaveUnread(none))

	some := tg.MessageReactions{
		RecentReactions: []tg.MessagePeerReaction{
			{Unread: false, Reaction: &tg.ReactionEmoji{Emoticon: "👍"}},
			{Unread: true, Reaction: &tg.ReactionEmoji{Emoticon: "❤"}},
		},
	}
	assert.True(t, reactionsHaveUnread(some))

	assert.False(t, reactionsHaveUnread(tg.MessageReactions{}))
}

func TestConvertMessage_HasUnreadReactions(t *testing.T) {
	raw := &tg.Message{
		ID: 5, Date: 1700000000, Out: true, Message: "hi",
		Reactions: tg.MessageReactions{
			Results: []tg.ReactionCount{
				{Reaction: &tg.ReactionEmoji{Emoticon: "❤"}, Count: 1},
			},
			RecentReactions: []tg.MessagePeerReaction{
				{Unread: true, Reaction: &tg.ReactionEmoji{Emoticon: "❤"}},
			},
		},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.True(t, msg.HasUnreadReactions)

	plain := &tg.Message{ID: 6, Date: 1700000000, Message: "no reactions"}
	msg2, ok := convertMessage(plain, 10)
	require.True(t, ok)
	assert.False(t, msg2.HasUnreadReactions)
}
