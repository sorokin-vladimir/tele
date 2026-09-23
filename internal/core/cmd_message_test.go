package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

func (s *stubClient) EditMessage(_ context.Context, _ domain.Peer, _ int, text string, _ []domain.MessageEntity) error {
	s.editedTo = text
	return s.err
}

func TestEditMessage_ShowsTheNewTextBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Text: "before", Date: time.Unix(1, 0)}})

	require.NoError(t, o.EditMessage(context.Background(), 1, 5, "after", nil))

	got := st.Messages(1)
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
	got := st.Messages(1)
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
	return s.err
}

func TestDeleteMessages_RemovesBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{
		{ID: 5, ChatID: 1, Text: "keep", Date: time.Unix(1, 0)},
		{ID: 6, ChatID: 1, Text: "drop", Date: time.Unix(2, 0)},
	})

	require.NoError(t, o.DeleteMessages(context.Background(), 1, []int{6}, true))

	require.Len(t, st.Messages(1), 1)
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
	got := st.Messages(1)
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

func (s *stubClient) SendReaction(_ context.Context, _ domain.Peer, _ int, emoji string) error {
	s.reactedWith = emoji
	s.reactionSent = true
	return s.err
}

func TestSendReaction_ShowsTheReactionBeforeTheServerAnswers(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})

	require.NoError(t, o.SendReaction(context.Background(), 1, 5, "👍"))

	got := st.Messages(1)[0].Reactions
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

	require.NoError(t, o.SendReaction(context.Background(), 1, 5, "👍"))

	require.True(t, c.reactionSent)
	assert.Empty(t, c.reactedWith, "retracting sends an empty reaction")
	assert.Empty(t, st.Messages(1)[0].Reactions)
}

func TestSendReaction_RestoresThePreviousReactionsOnFailure(t *testing.T) {
	c := &stubClient{err: &telerr.Error{Kind: telerr.RateLimited}}
	o, st := newCmdOwner(t, c)
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1, 0),
		Reactions: []domain.Reaction{{Emoji: "🔥", Count: 2}},
	}})

	err := o.SendReaction(context.Background(), 1, 5, "👍")

	require.Error(t, err)
	got := st.Messages(1)[0].Reactions
	require.Len(t, got, 1)
	assert.Equal(t, "🔥", got[0].Emoji)
	assert.Equal(t, 2, got[0].Count)
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

func (s *stubClient) ForwardMessages(_ context.Context, _ domain.Peer, to domain.Peer, ids []int) error {
	s.forwardedTo, s.forwardedPeer, s.forwardedIDs = to.ID, to, ids
	return s.err
}

func (s *stubClient) SendMessage(_ context.Context, peer domain.Peer, text string, replyTo int, ents []domain.MessageEntity, randomID int64) (domain.Message, error) {
	s.sendMu.Lock()
	s.sendCount++
	s.sentText = text
	s.sentRandomID = randomID
	s.sentPeer = peer
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
		ReplyToMsgID: replyTo, IsOut: true, Date: time.Unix(1700000000, 0),
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

	require.NoError(t, o.Forward(context.Background(), 1, bob.ID, []int{5}, ""))

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

	err := o.Forward(context.Background(), 1, 99, []int{5}, "")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.Empty(t, c.forwardedIDs)
}

func TestForward_UnknownSourceIsPeerNotFound(t *testing.T) {
	o, _ := newCmdOwner(t, &stubClient{})

	err := o.Forward(context.Background(), 404, bob.ID, []int{5}, "")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
}

func TestForward_SendsTheCommentFirst(t *testing.T) {
	c := &stubClient{}
	o, st := newCmdOwner(t, c)
	st.SetChat(domain.Chat{ID: 2, Peer: bob})
	st.SetMessages(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(1, 0)}})
	o.SetOutbox(newOutboxStore(t))
	ctx := runWorker(t, o)

	require.NoError(t, o.Forward(ctx, 1, bob.ID, []int{5}, "look"))

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

	require.NoError(t, o.Forward(context.Background(), 1, bob.ID, []int{5}, ""))

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

	require.Error(t, o.Forward(context.Background(), 1, bob.ID, []int{5}, ""))

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

	require.NoError(t, o.Forward(context.Background(), 1, bob.ID, []int{5}, "look"))

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

	err := o.Forward(context.Background(), 1, bob.ID, []int{5}, "look")

	assert.Equal(t, telerr.PeerNotFound, telerr.Of(err))
	assert.Empty(t, c.forwardedIDs, "nothing may be copied when the comment could not be queued")
}
