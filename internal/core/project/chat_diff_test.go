package project_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

func chatContents(m []domain.Message) project.ChatContents {
	c := project.ChatContents{ChatID: 1, Messages: m}
	if len(m) > 0 {
		c.AnchorMsgID = m[len(m)-1].ID
	}
	return c
}

func kinds(ds []project.ChatDelta) []project.ChatDeltaKind {
	out := make([]project.ChatDeltaKind, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Kind)
	}
	return out
}

func TestDiffChat_IdenticalContentsProduceNothing(t *testing.T) {
	c := chatContents(msgs(3))

	assert.Empty(t, project.DiffChat(c, c))
}

func TestDiffChat_FirstContentsAreAReset(t *testing.T) {
	got := project.DiffChat(project.ChatContents{}, chatContents(msgs(3)))

	require.NotEmpty(t, got)
	assert.Equal(t, project.ChatReset, got[0].Kind)
	assert.Len(t, got[0].Contents.Messages, 3)
}

func TestDiffChat_NewMessageAtTheTailIsAnAppend(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(msgs(4))

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatAppend, got[0].Kind)
	assert.Equal(t, 4, got[0].Message.ID)
}

func TestDiffChat_NewMessageWhileScrolledIntoThePastIsNotAppended(t *testing.T) {
	// The window holds 1..3 of a history that now has 4; HasNewer flips instead.
	prev := chatContents(msgs(3))
	next := chatContents(msgs(3))
	next.HasNewer = true

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatNewer, got[0].Kind, "no Append: the message is outside the window")
	assert.True(t, got[0].HasNewer)
	assert.Empty(t, got[0].Messages, "nothing entered the window")
}

func TestDiffChat_OlderMessagesPrependedAreAnOlderDelta(t *testing.T) {
	all := msgs(6)
	prev := chatContents(all[3:]) // 4,5,6
	prev.HasOlder = true
	next := chatContents(all) // 1..6

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatOlder, got[0].Kind)
	assert.Equal(t, []int{1, 2, 3}, ids(got[0].Messages))
	assert.False(t, got[0].HasOlder)
}

func TestDiffChat_NewerMessagesAppendedInBulkAreANewerDelta(t *testing.T) {
	all := msgs(6)
	prev := chatContents(all[:3]) // 1,2,3
	prev.HasNewer = true
	next := chatContents(all)

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatNewer, got[0].Kind)
	assert.Equal(t, []int{4, 5, 6}, ids(got[0].Messages))
}

func TestDiffChat_EditInsideTheWindowIsAnUpdate(t *testing.T) {
	prev := chatContents(msgs(3))
	edited := msgs(3)
	edited[1].Text = "edited"
	next := chatContents(edited)

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatUpdate, got[0].Kind)
	assert.Equal(t, 2, got[0].Message.ID)
	assert.Equal(t, "edited", got[0].Message.Text)
}

func TestDiffChat_ReactionChangeInsideTheWindowIsAnUpdate(t *testing.T) {
	prev := chatContents(msgs(3))
	reacted := msgs(3)
	reacted[0].Reactions = []domain.Reaction{{Emoji: "👍", Count: 1}}
	next := chatContents(reacted)

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatUpdate, got[0].Kind)
	assert.Equal(t, 1, got[0].Message.ID)
}

func TestDiffChat_DeletionInsideTheWindowIsARemove(t *testing.T) {
	all := msgs(3)
	prev := chatContents(all)
	next := chatContents([]domain.Message{all[0], all[2]})

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatRemove, got[0].Kind)
	assert.Equal(t, []int{2}, got[0].MsgIDs)
}

func TestDiffChat_ReadPointerChangeIsAReadDelta(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(msgs(3))
	next.ReadInboxMaxID = 3
	next.ReadOutboxMaxID = 2

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatRead, got[0].Kind)
	assert.Equal(t, 3, got[0].ReadInboxMaxID)
	assert.Equal(t, 2, got[0].ReadOutboxMaxID)
}

func TestDiffChat_DraftChangeIsADraftDelta(t *testing.T) {
	prev := chatContents(msgs(1))
	next := chatContents(msgs(1))
	next.Draft = "typed elsewhere"

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatDraft}, kinds(got))
	assert.Equal(t, "typed elsewhere", got[0].Draft)
}

// A contact coming online must not disturb the message window: re-seating the
// list re-anchors the viewport, which would scroll a chat being read back to
// the bottom.
func TestDiffChat_PresenceChangeUpdatesOnlyTheHeader(t *testing.T) {
	prev := chatContents(msgs(1))
	next := chatContents(msgs(1))
	next.Online = true

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatHeaderUpdate}, kinds(got))
	assert.True(t, got[0].Contents.Online)
}

func TestDiffChat_UnreadReactionsAreHeaderState(t *testing.T) {
	// The reaction may have landed on a message far outside the window, so the
	// count is carried by the header rather than by any message.
	prev := chatContents(msgs(3))
	next := chatContents(msgs(3))
	next.UnreadReactions = 1

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatHeaderUpdate}, kinds(got))
	assert.Equal(t, 1, got[0].Contents.UnreadReactions)
}

func TestDiffChat_UnreadMentionsAreHeaderState(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(msgs(3))
	next.UnreadMentions = 2

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatHeaderUpdate}, kinds(got))
	assert.Equal(t, 2, got[0].Contents.UnreadMentions)
}

func TestDiffChat_HeaderAndWindowChangeTogether(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(msgs(4))
	next.Online = true

	got := project.DiffChat(prev, next)

	assert.Equal(t, []project.ChatDeltaKind{project.ChatAppend, project.ChatHeaderUpdate}, kinds(got))
}

func TestDiffChat_SwitchingChatIsAReset(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(msgs(3))
	next.ChatID = 2

	got := project.DiffChat(prev, next)

	require.NotEmpty(t, got)
	assert.Equal(t, project.ChatReset, got[0].Kind)
}

func TestDiffChat_AnchorMoveIsAReset(t *testing.T) {
	all := msgs(10)
	prev := chatContents(all[7:])
	next := chatContents(all[2:5])
	next.AnchorMsgID = 4
	next.HasOlder, next.HasNewer = true, true

	got := project.DiffChat(prev, next)

	require.NotEmpty(t, got)
	assert.Equal(t, project.ChatReset, got[0].Kind,
		"a jump to a quoted message replaces the window, it does not extend it")
}

func TestDiffChat_AnchorMoveWithinTheSameWindowIsAReset(t *testing.T) {
	prev := chatContents(msgs(5))
	next := chatContents(msgs(5))
	next.AnchorMsgID = 2 // jumped to a quoted message the window already held

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatReset, got[0].Kind)
	assert.Equal(t, 2, got[0].Contents.AnchorMsgID)
}

func TestDiffChat_EmptyingTheWindowRemovesEveryMessage(t *testing.T) {
	prev := chatContents(msgs(3))
	next := chatContents(nil)

	got := project.DiffChat(prev, next)

	require.NotEmpty(t, got)
	assert.Equal(t, project.ChatRemove, got[0].Kind, "every message in the window went away")
	assert.Equal(t, []int{1, 2, 3}, got[0].MsgIDs)
}

// A window of fixed size anchored on the newest message slides as messages
// arrive. Calling that a Reset re-seats the message list and scrolls a chat
// being read back to the bottom.
func TestDiffChat_SlidingWindowIsARemovePlusAnAppend(t *testing.T) {
	all := msgs(10)
	prev := chatContents(all[2:7]) // 3,4,5,6,7
	next := chatContents(all[3:8]) // 4,5,6,7,8

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatRemove, project.ChatAppend}, kinds(got))
	assert.Equal(t, []int{3}, got[0].MsgIDs)
	assert.Equal(t, 8, got[1].Message.ID)
}

func TestDiffChat_SlidingByMoreThanOne(t *testing.T) {
	all := msgs(12)
	prev := chatContents(all[0:5]) // 1..5
	next := chatContents(all[3:8]) // 4..8

	got := project.DiffChat(prev, next)

	require.Equal(t, []project.ChatDeltaKind{project.ChatRemove, project.ChatNewer}, kinds(got))
	assert.Equal(t, []int{1, 2, 3}, got[0].MsgIDs)
	assert.Equal(t, []int{6, 7, 8}, ids(got[1].Messages))
}

func TestDiffChat_EmitsAnOutboxDeltaWhenTheQueueChanges(t *testing.T) {
	prev := project.ChatContents{ChatID: 1, Messages: []domain.Message{{ID: 10}}}
	next := prev
	next.Outbox = []domain.OutboxEntry{{Ref: "r1", State: domain.OutboxQueued}}

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatOutbox, got[0].Kind)
	require.Len(t, got[0].Outbox, 1)
	assert.Equal(t, "r1", got[0].Outbox[0].Ref)
}

func TestDiffChat_SaysNothingWhenTheQueueIsUnchanged(t *testing.T) {
	prev := project.ChatContents{
		ChatID:   1,
		Messages: []domain.Message{{ID: 10}},
		Outbox:   []domain.OutboxEntry{{Ref: "r1", State: domain.OutboxQueued}},
	}

	got := project.DiffChat(prev, prev)

	assert.Empty(t, got)
}

func TestDiffChat_AnEntryChangingStateIsAnOutboxDelta(t *testing.T) {
	prev := project.ChatContents{
		ChatID:   1,
		Messages: []domain.Message{{ID: 10}},
		Outbox:   []domain.OutboxEntry{{Ref: "r1", State: domain.OutboxQueued}},
	}
	next := prev
	next.Outbox = []domain.OutboxEntry{{Ref: "r1", State: domain.OutboxFailed}}

	got := project.DiffChat(prev, next)

	require.Len(t, got, 1)
	assert.Equal(t, project.ChatOutbox, got[0].Kind)
	assert.Equal(t, domain.OutboxFailed, got[0].Outbox[0].State)
}
