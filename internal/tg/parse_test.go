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

// A message without a reactions field says nothing about reactions and one
// with an empty field says there are none; an edit applies only the second
// (#248), so the two must stay apart: nil and an empty set.
func TestConvertMessage_AbsentReactionsAreNotAnEmptySet(t *testing.T) {
	absent, ok := convertMessage(&tg.Message{ID: 5, Date: 1700000000, Message: "hi"}, 10)
	require.True(t, ok)
	assert.Nil(t, absent.Reactions)

	raw := &tg.Message{ID: 5, Date: 1700000000, Message: "hi"}
	raw.SetReactions(tg.MessageReactions{Results: []tg.ReactionCount{}})
	empty, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.NotNil(t, empty.Reactions)
	assert.Empty(t, empty.Reactions)
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

func TestConvertMessage_Mentioned(t *testing.T) {
	raw := &tg.Message{ID: 7, Date: 1700000000, Mentioned: true, Message: "@you hi"}
	out, ok := convertMessage(raw, 1)
	require.True(t, ok)
	assert.True(t, out.Mentioned)

	plain := &tg.Message{ID: 8, Date: 1700000000, Message: "hi"}
	out2, ok := convertMessage(plain, 1)
	require.True(t, ok)
	assert.False(t, out2.Mentioned)
}

// A plain message in a topic names the topic in reply_to_msg_id and nothing in
// reply_to_top_id: it belongs to the topic and replies to nothing.
func TestConvertMessage_PlainMessageInATopic(t *testing.T) {
	raw := &tg.Message{ID: 40, Date: 1700000000, Message: "hi",
		ReplyTo: &tg.MessageReplyHeader{ForumTopic: true, ReplyToMsgID: 12}}
	out, ok := convertMessage(raw, 5)
	require.True(t, ok)
	assert.Equal(t, 12, out.TopicID)
	assert.Zero(t, out.ReplyToMsgID)
}

// A reply inside a topic names the topic in reply_to_top_id, which leaves
// reply_to_msg_id free to be the message replied to.
func TestConvertMessage_ReplyInsideATopic(t *testing.T) {
	raw := &tg.Message{ID: 41, Date: 1700000000, Message: "agreed",
		ReplyTo: &tg.MessageReplyHeader{ForumTopic: true, ReplyToMsgID: 40, ReplyToTopID: 12}}
	out, ok := convertMessage(raw, 5)
	require.True(t, ok)
	assert.Equal(t, 12, out.TopicID)
	assert.Equal(t, 40, out.ReplyToMsgID)
}

func TestConvertMessageGroupedID(t *testing.T) {
	raw := &tg.Message{ID: 10, Message: "part", Date: 1700000000}
	raw.SetGroupedID(9988776655)
	got, ok := convertMessage(raw, 42)
	if !ok {
		t.Fatalf("convertMessage returned ok=false")
	}
	if got.GroupedID != 9988776655 {
		t.Fatalf("GroupedID = %d, want 9988776655", got.GroupedID)
	}
}

func TestConvertMessageNoGroupedID(t *testing.T) {
	raw := &tg.Message{ID: 11, Message: "solo", Date: 1700000000}
	got, _ := convertMessage(raw, 42)
	if got.GroupedID != 0 {
		t.Fatalf("GroupedID = %d, want 0", got.GroupedID)
	}
}

// Every reaction set from Telegram enters the app through convertReactions,
// and leaves it in emoji presentation, the way the picker spells a pick: a
// text-default emoji, ZWJ sequences included, gains U+FE0F, the rest are kept
// as they are, and a reaction that is not an emoji is skipped (#294).
func TestConvertReactions_SpellsEveryEmojiInEmojiPresentation(t *testing.T) {
	mr := tg.MessageReactions{
		Results: []tg.ReactionCount{
			{Reaction: &tg.ReactionEmoji{Emoticon: "\U00002764"}, Count: 3},
			{Reaction: &tg.ReactionEmoji{Emoticon: "\U00002764\U0000200D\U0001F525"}, Count: 2},
			{Reaction: &tg.ReactionEmoji{Emoticon: "👍"}, Count: 5},
			{Reaction: &tg.ReactionPaid{}, Count: 1},
		},
	}

	got := convertReactions(mr)

	require.Len(t, got, 3)
	assert.Equal(t, "\U00002764\U0000FE0F", got[0].Emoji)
	assert.Equal(t, 3, got[0].Count)
	assert.Equal(t, "\U00002764\U0000FE0F\U0000200D\U0001F525", got[1].Emoji)
	assert.Equal(t, 2, got[1].Count)
	assert.Equal(t, "👍", got[2].Emoji)
	assert.Equal(t, 5, got[2].Count)
}

// The notification path reads the newest unread reaction on its own, and
// spells it the same way (#294).
func TestNewestUnreadReaction_SpellsTheEmojiInEmojiPresentation(t *testing.T) {
	mr := tg.MessageReactions{
		RecentReactions: []tg.MessagePeerReaction{
			{Unread: true, Date: 1700000000, Reaction: &tg.ReactionEmoji{Emoticon: "\U0001F54A"}},
		},
	}

	emoji, date, ok := newestUnreadReaction(mr)

	require.True(t, ok)
	assert.Equal(t, "\U0001F54A\U0000FE0F", emoji)
	assert.Equal(t, int64(1700000000), date.Unix())
}
