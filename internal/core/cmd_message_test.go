package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (s *stubClient) EditMessage(_ context.Context, _ domain.Peer, _ int, text string, ents []domain.MessageEntity) error {
	s.editedTo = text
	s.sentEntities = ents
	return s.err
}

func TestEditMessage_ShowsTheNewTextBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "before", Date: time.Unix(1, 0)}})

	require.NoError(t, o.EditMessage(context.Background(), 1, 5, "after", nil))

	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 1)
	assert.Equal(t, "after", got[0].Text)
	assert.Equal(t, "after", c.editedTo)
}

// A refused edit must leave no trace: neither the new text, nor the "edited"
// marker the optimistic version set (#118).
func TestEditMessage_RestoresTheOldTextOnFailure(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "before", Date: time.Unix(1, 0)}})

	err := o.EditMessage(context.Background(), 1, 5, "after", nil)

	require.Error(t, err)
	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 1)
	assert.Equal(t, "before", got[0].Text)
	assert.Nil(t, got[0].EditDate, "a refused edit must not leave an edited marker")
}

func TestEditMessage_UnknownMessageIsNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	err := o.EditMessage(context.Background(), 1, 404, "after", nil)

	assert.Equal(t, telerr.NotFound, telerr.Of(err))
}

func (s *stubClient) DeleteMessages(_ context.Context, _ domain.Peer, ids []int, revoke bool) error {
	s.deletedIDs, s.revoked = ids, revoke
	s.deleteCalls = append(s.deleteCalls, append([]int(nil), ids...))
	if s.deleteFailsOn != 0 && s.deleteFailsOn != len(s.deleteCalls) {
		return nil
	}
	return s.err
}

// seedNumbered stores messages 1..n in chat 1 and returns their ids in order.
func seedNumbered(st store.Store, n int) []int {
	msgs := make([]domain.Message, n)
	ids := make([]int, n)
	for i := range msgs {
		ids[i] = i + 1
		msgs[i] = domain.Message{ID: i + 1, ChatID: 1, Text: "m", Date: time.Unix(int64(i+1), 0)}
	}
	st.SetMessages(1, msgs)
	return ids
}

func TestDeleteMessages_SendsAtMostOneHundredIDsPerCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		sizes []int
	}{
		{"one", 1, []int{1}},
		{"exactly a batch", 100, []int{100}},
		{"one over", 101, []int{100, 1}},
		{"two and a half batches", 250, []int{100, 100, 50}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &stubClient{}
			o, st := newCmdOwner(t, c)
			ids := seedNumbered(st, tc.count)

			require.NoError(t, o.DeleteMessages(context.Background(), 1, ids, true))

			require.Len(t, c.deleteCalls, len(tc.sizes))
			var sent []int
			for i, call := range c.deleteCalls {
				assert.Len(t, call, tc.sizes[i])
				sent = append(sent, call...)
			}
			assert.Equal(t, ids, sent, "every id goes out once, in the order given")
			assert.True(t, c.revoked)
			assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1}))
		})
	}
}

// The window shows all of them gone at once; only the refused batch and the
// ones after it are put back.
func TestDeleteMessages_StopsAtTheFirstRefusedBatch(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}, deleteFailsOn: 2}
	o, st := newCmdOwner(t, c)
	ids := seedNumbered(st, 250)

	err := o.DeleteMessages(context.Background(), 1, ids, false)

	require.Error(t, err)
	assert.Equal(t, telerr.Forbidden, telerr.Of(err))
	require.Len(t, c.deleteCalls, 2, "nothing is sent after the refusal")
	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 150)
	assert.Equal(t, 101, got[0].ID, "the first batch stays deleted")
	assert.Equal(t, 250, got[len(got)-1].ID)
}

func TestDeleteMessages_RefusalOfTheFirstBatchRestoresEverything(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}}
	o, st := newCmdOwner(t, c)
	ids := seedNumbered(st, 150)

	require.Error(t, o.DeleteMessages(context.Background(), 1, ids, true))

	assert.Len(t, c.deleteCalls, 1)
	assert.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 150)
}

// A message that was already gone has nothing to put back, whatever batch it
// was in.
func TestDeleteMessages_RefusalDoesNotResurrectMissingMessages(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}, deleteFailsOn: 2}
	o, st := newCmdOwner(t, c)
	ids := seedNumbered(st, 120)
	var kept []domain.Message
	for _, m := range st.Messages(domain.HistoryKey{ChatID: 1}) {
		if m.ID != 110 {
			kept = append(kept, m)
		}
	}
	st.SetMessages(1, kept)

	require.Error(t, o.DeleteMessages(context.Background(), 1, ids, true))

	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 19, "messages 101..120 come back, except the one that was gone")
	for _, m := range got {
		assert.NotEqual(t, 110, m.ID)
	}
}

func TestDeleteMessages_RemovesBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{
		{ID: 5, ChatID: 1, Text: "keep", Date: time.Unix(1, 0)},
		{ID: 6, ChatID: 1, Text: "drop", Date: time.Unix(2, 0)},
	})

	require.NoError(t, o.DeleteMessages(context.Background(), 1, []int{6}, true))

	require.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 1)
	assert.Equal(t, []int{6}, c.deletedIDs)
	assert.True(t, c.revoked)
}

func TestDeleteMessages_PutsTheMessageBackOnFailure(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{
		{ID: 5, ChatID: 1, Text: "keep", Date: time.Unix(1, 0)},
		{ID: 6, ChatID: 1, Text: "drop", Date: time.Unix(2, 0)},
	})

	err := o.DeleteMessages(context.Background(), 1, []int{6}, true)

	require.Error(t, err)
	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 2, "a refused delete must restore the message")
	assert.Equal(t, 6, got[1].ID)
	assert.Equal(t, "drop", got[1].Text)
}

// Restoring must not look like an arrival: an undone delete moves no counter.
func TestDeleteMessages_RestoreDoesNotCountAsUnread(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 6, ChatID: 1, Text: "drop", Date: time.Unix(2, 0)}})
	before, _ := st.GetChat(1)

	require.Error(t, o.DeleteMessages(context.Background(), 1, []int{6}, true))

	after, _ := st.GetChat(1)
	assert.Equal(t, before.UnreadCount, after.UnreadCount)
}

func (s *stubClient) SendReaction(_ context.Context, _ domain.Peer, _ int, emoji string) ([]domain.Reaction, error) {
	s.reactedWith = emoji
	s.reactionSent = true
	return s.reactionReply, s.err
}

func TestSendReaction_ShowsTheReactionBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})

	kept, err := o.SendReaction(context.Background(), 1, 5, "👍")
	require.NoError(t, err)
	assert.True(t, kept, "a reply that states no set is not judged")

	got := st.Messages(domain.HistoryKey{ChatID: 1})[0].Reactions
	require.Len(t, got, 1)
	assert.Equal(t, "👍", got[0].Emoji)
	assert.True(t, got[0].IsChosen)
	assert.Equal(t, "👍", c.reactedWith)
}

// Reacting again with the same emoji retracts it, which Telegram expresses as
// an empty reaction.
func TestSendReaction_SecondTimeRetracts(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1, 0),
		Reactions: []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}},
	}})

	_, err := o.SendReaction(context.Background(), 1, 5, "👍")
	require.NoError(t, err)

	require.True(t, c.reactionSent)
	assert.Empty(t, c.reactedWith, "retracting sends an empty reaction")
	assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1})[0].Reactions)
}

func TestSendReaction_RestoresThePreviousReactionsOnFailure(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.RateLimited}}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1, 0),
		Reactions: []domain.Reaction{{Emoji: "🔥", Count: 2}},
	}})

	_, err := o.SendReaction(context.Background(), 1, 5, "👍")

	require.Error(t, err)
	got := st.Messages(domain.HistoryKey{ChatID: 1})[0].Reactions
	require.Len(t, got, 1)
	assert.Equal(t, "🔥", got[0].Emoji)
	assert.Equal(t, 2, got[0].Count)
}

func TestSendReaction_OptimisticAndRollbackLeaveUnreadBadgeUntouched(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.RateLimited}}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 1})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})
	// Track an unread reaction on message 5.
	st.ApplyUnreadReaction(1, 5, true)
	chat, _ := st.GetChat(1)
	require.Equal(t, 1, chat.UnreadReactionsCount)

	// SendReaction fails and triggers rollback.
	_, err := o.SendReaction(context.Background(), 1, 5, "👍")
	require.Error(t, err)

	// The chat's unread reaction badge must still be 1.
	chat, _ = st.GetChat(1)
	assert.Equal(t, 1, chat.UnreadReactionsCount)
}

// The reply to messages.sendReaction states the set Telegram ended up with.
// A request that succeeded but left a set without our pick is reported, never
// silent (#248); a set with exactly our pick as ours is what was asked for.
func TestSendReaction_JudgesTheSetTheReplyStates(t *testing.T) {
	chosen := func(emoji ...string) []domain.Reaction {
		out := []domain.Reaction{{Emoji: "🔥", Count: 3}}
		for _, e := range emoji {
			out = append(out, domain.Reaction{Emoji: e, Count: 1, IsChosen: true})
		}
		return out
	}
	tests := []struct {
		name  string
		had   []domain.Reaction
		pick  string
		reply []domain.Reaction
		kept  bool
	}{
		{"set and kept", nil, "👍", chosen("👍"), true},
		{"set but not in the reply", nil, "👍", chosen(), false},
		{"set but another one is ours", nil, "👍", chosen("👌"), false},
		{"switched and kept", chosen("👌"), "👍", chosen("👍"), true},
		{"retracted and gone", chosen("👍"), "👍", chosen(), true},
		{"retracted but still ours", chosen("👍"), "👍", chosen("👍"), false},
		{"empty set stated", nil, "👍", []domain.Reaction{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &stubClient{reactionReply: tt.reply}
			o, st := newCmdOwner(t, c)
			st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0), Reactions: tt.had}})

			kept, err := o.SendReaction(context.Background(), 1, 5, tt.pick)

			require.NoError(t, err)
			assert.Equal(t, tt.kept, kept)
		})
	}
}

// optimisticReactions moved here from the UI, so its behaviour gets stated
// directly rather than only through SendReaction.
func TestOptimisticReactions(t *testing.T) {
	tests := []struct {
		name    string
		current []domain.Reaction
		emoji   string
		want    []domain.Reaction
	}{
		{
			name:  "first reaction on a message",
			emoji: "👍",
			want:  []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}},
		},
		{
			name:    "joining others",
			current: []domain.Reaction{{Emoji: "👍", Count: 2}},
			emoji:   "👍",
			want:    []domain.Reaction{{Emoji: "👍", Count: 3, IsChosen: true}},
		},
		{
			name:    "retracting the only reaction drops it",
			current: []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}},
			emoji:   "👍",
			want:    []domain.Reaction{},
		},
		{
			name:    "retracting leaves the others' count",
			current: []domain.Reaction{{Emoji: "👍", Count: 3, IsChosen: true}},
			emoji:   "👍",
			want:    []domain.Reaction{{Emoji: "👍", Count: 2}},
		},
		{
			name: "switching emoji releases the previous one",
			current: []domain.Reaction{
				{Emoji: "👍", Count: 1, IsChosen: true},
				{Emoji: "🔥", Count: 4},
			},
			emoji: "🔥",
			want:  []domain.Reaction{{Emoji: "🔥", Count: 5, IsChosen: true}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, optimisticReactions(tt.current, tt.emoji))
		})
	}
}

var bob = domain.Peer{ID: 2, Type: domain.PeerUser}

func (s *stubClient) ForwardMessages(_ context.Context, _ domain.Peer, to domain.Peer, toTopic int, ids []int) error {
	s.forwardedTo, s.forwardedPeer, s.forwardedIDs, s.forwardedTopic = to.ID, to, ids, toTopic
	return s.err
}

func (s *stubClient) SendMessage(_ context.Context, peer domain.Peer, text string, replyTo, topic int, ents []domain.MessageEntity, randomID int64) (domain.Message, error) {
	s.sendMu.Lock()
	s.sendCount++
	s.sentText = text
	s.sentTopic = topic
	s.sentRandomID = randomID
	s.sentPeer = peer
	s.sentEntities = ents
	block := s.sendBlock
	sentID := s.sentID
	// Every send creates a distinct message, as Telegram does. Reusing one id
	// would let a bug that mixes entries up pass unnoticed.
	nth := s.sendCount - 1
	s.sendMu.Unlock()
	if block != nil {
		<-block
	}
	if s.err != nil {
		return domain.Message{}, s.err
	}
	switch sentID {
	case 0:
		sentID = 777 + nth
	case -1:
		sentID = 0 // stands in for a reply that named no message
	default:
		sentID += nth
	}
	// Telegram answers a send with the message it created; the stub does the
	// same, since that is what the caller records (#193).
	return domain.Message{
		ID: sentID, ChatID: peer.ID, Text: text, Entities: ents,
		ReplyToMsgID: replyTo, TopicID: topic, IsOut: true, Date: time.Unix(1700000000, 0),
	}, nil
}

func (s *stubClient) sendCalls() int {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.sendCount
}

func (s *stubClient) lastSentPeer() domain.Peer {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.sentPeer
}

func (s *stubClient) lastRandomID() int64 {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.sentRandomID
}

func TestForward_SendsToTheGivenTarget(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 2, Title: "Bob", Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "hi", Date: time.Unix(1, 0)}})

	require.NoError(t, o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, ""))

	assert.Equal(t, int64(2), c.forwardedTo)
	assert.Equal(t, []int{5}, c.forwardedIDs)
}

// A target the owner can neither find a dialog for nor has an address for is
// nobody Telegram can reach. A search hit is reachable because search left its
// address behind; see TestForward_ToAPersonFoundBySearch.
func TestForward_UnknownTargetIsPeerNotFound(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})

	err := o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: 99}, []int{5}, "")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.Empty(t, c.forwardedIDs)
}

func TestForward_UnknownSourceIsPeerNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	err := o.Forward(context.Background(), 404, domain.HistoryKey{ChatID: bob.ID}, []int{5}, "")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
}

func TestForward_SendsTheCommentFirst(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 2, Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)

	require.NoError(t, o.Forward(ctx, 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, "look"))

	waitFor(t, "the comment was never sent", func() bool { return c.sendCalls() == 1 })
	assert.Equal(t, "look", c.sentText)
}

// A known target bubbles up the list at once; the real message arrives later
// through the update stream.
func TestForward_BumpsTheTargetChat(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 2, Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "hi", Date: time.Unix(1, 0)}})

	require.NoError(t, o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, ""))

	target, _ := st.GetChat(2)
	require.NotNil(t, target.LastMessage)
	assert.Equal(t, "hi", target.LastMessage.Text)
	assert.True(t, target.LastMessage.IsOut)
}

func TestForward_DoesNotBumpWhenTheForwardFails(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.Forbidden}}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 2, Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "hi", Date: time.Unix(1, 0)}})

	require.Error(t, o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, ""))

	target, _ := st.GetChat(2)
	assert.Nil(t, target.LastMessage, "a refused forward must not surface the target")
}

// The comment is queued for the target chat like any other message. It used to
// be written into the store here by hand, because echo suppression hid it and no
// optimistic bubble would ever show it; with the comment on the queue and
// suppression gone for text, it arrives through the update path instead (#193).
func TestForward_QueuesTheCommentForTheTargetChat(t *testing.T) {
	o, st := newCmdOwner(t, &stubClient{})
	st.SetChat(domain.Chat{ID: 2, Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})
	q := newOutboxStore(t)
	o.SetOutbox(q)

	require.NoError(t, o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, "look"))

	queued := q.ForChat(2)
	require.Len(t, queued, 1, "the comment must be queued for the target, not the source")
	require.NotNil(t, queued[0].Message)
	assert.Equal(t, "look", queued[0].Message.Text)
}

// A target the owner cannot address stops the forward before anything is
// copied: the comment would be lost and the messages would arrive without it.
func TestForward_StopsWhenTheCommentCannotBeQueued(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})
	o.SetOutbox(newOutboxStore(t))

	err := o.Forward(context.Background(), 1, domain.HistoryKey{ChatID: bob.ID}, []int{5}, "look")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.Empty(t, c.forwardedIDs, "nothing may be copied when the comment could not be queued")
}
