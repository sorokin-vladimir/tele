package state_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func newState(t *testing.T) (*state.State, store.Store) {
	t.Helper()
	st := store.NewMemory()
	return state.New(st), st
}

func TestApplyIncoming_AppendsCountsAndReportsChange(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1, Title: "A"})

	chg, ok := s.ApplyIncoming(domain.Message{ID: 5, ChatID: 1, Text: "hi"})

	require.True(t, ok)
	assert.Equal(t, state.ChangeNewMessage, chg.Kind)
	assert.Equal(t, int64(1), chg.ChatID)
	assert.Equal(t, 5, chg.Message.ID)
	assert.True(t, chg.UnreadChanged)
	require.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 1)
	c, _ := st.GetChat(1)
	assert.Equal(t, 1, c.UnreadCount)
}

// A message already covered by the read pointer is still appended to history,
// but no counter moved, so the folder bar must not be recomputed.
func TestApplyIncoming_ReadElsewhereReportsNoUnreadChange(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1, ReadInboxMaxID: 10})

	chg, ok := s.ApplyIncoming(domain.Message{ID: 5, ChatID: 1})

	require.True(t, ok, "the message is still news to the client")
	assert.False(t, chg.UnreadChanged)
	require.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 1)
}

// The same message can be delivered twice: the wire and a recovered difference
// both carry it. The second copy is not news, and anything that fires once per
// arrival must not fire again (ADR 0016).
func TestApplyIncoming_SecondCopyIsNotNews(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1, Title: "A"})
	msg := domain.Message{ID: 5, ChatID: 1, Text: "hi"}

	_, ok := s.ApplyIncoming(msg)
	require.True(t, ok)

	_, ok = s.ApplyIncoming(msg)
	assert.False(t, ok, "the client already heard about this one")
	require.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 1, "and it is stored once")
	c, _ := st.GetChat(1)
	assert.Equal(t, 1, c.UnreadCount, "the counter did not move twice")
}

func TestApplyIncoming_OutgoingDoesNotCount(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})

	chg, ok := s.ApplyIncoming(domain.Message{ID: 5, ChatID: 1, IsOut: true})

	require.True(t, ok)
	assert.False(t, chg.UnreadChanged)
	c, _ := st.GetChat(1)
	assert.Equal(t, 0, c.UnreadCount)
}

// A real content edit carries an EditDate and rewrites the text.
func TestApplyEdit_RealEditUpdatesText(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "before"})
	when := time.Now()

	chg, ok := s.ApplyEdit(domain.Message{ID: 5, ChatID: 1, Text: "after", EditDate: &when})

	require.True(t, ok)
	assert.Equal(t, state.ChangeMessageEdited, chg.Kind)
	assert.Equal(t, "after", st.Messages(domain.HistoryKey{ChatID: 1})[0].Text)
}

// A reaction on a message that was genuinely edited earlier arrives with a
// non-nil EditDate: edit_date still carries the original edit time and
// edit_hide is false, because the "edited" label genuinely should show. The
// reactions ride along in the same payload and must not be dropped (#199).
func TestApplyEdit_AlreadyEditedMessageStillAppliesReactions(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	edited := time.Now().Add(-time.Hour)
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "fixed typo", EditDate: &edited})

	chg, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "fixed typo", EditDate: &edited,
		Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
		HasUnreadReactions: true,
	})

	require.True(t, ok)
	got := st.Messages(domain.HistoryKey{ChatID: 1})[0]
	require.Len(t, got.Reactions, 1, "the reaction carried by the edit must be applied")
	assert.Equal(t, "👍", got.Reactions[0].Emoji)
	assert.Equal(t, "fixed typo", got.Text, "the text edit still applies")
	assert.NotNil(t, got.EditDate, "the message stays marked as edited")
	assert.True(t, chg.UnreadReactionChanged, "the chat's unread-reaction count moved")
}

// A bot streaming a long reply rewrites one message over and over, and Telegram
// hides the label on every one of those edits. The new text must still land:
// the flag is about the label and says nothing about the content (#269).
func TestApplyEdit_HiddenEditUpdatesTheText(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "thinking"})
	when := time.Now()

	chg, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "the whole answer", EditDate: &when, EditHidden: true,
	})

	require.True(t, ok)
	assert.Equal(t, state.ChangeMessageEdited, chg.Kind)
	got := st.Messages(domain.HistoryKey{ChatID: 1})[0]
	assert.Equal(t, "the whole answer", got.Text)
	assert.NotNil(t, got.EditDate, "the edit happened and the time is recorded")
	assert.False(t, got.ShowsEdited(), "Telegram asked for no label on this one")
}

// Delivery is at least once, so the same edit can arrive twice and a late copy
// can arrive after a newer one. Position is what separates them: an edit at or
// behind what the message has already applied changes nothing and tells the
// client nothing (ADR 0016).
func TestApplyEdit_LateCopyOfAnEditIsRefused(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "first"})
	when := time.Now()

	_, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "second", EditDate: &when, AppliedPosition: 20,
	})
	require.True(t, ok)

	// The same one again, and then one from before it.
	_, ok = s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "second", EditDate: &when, AppliedPosition: 20,
	})
	assert.False(t, ok, "a duplicate changes nothing")

	_, ok = s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "stale", EditDate: &when, AppliedPosition: 19,
	})
	assert.False(t, ok, "a copy from before the stored one changes nothing")
	assert.Equal(t, "second", st.Messages(domain.HistoryKey{ChatID: 1})[0].Text, "the newer text stands")

	// And the stream carries on from where it was.
	_, ok = s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "third", EditDate: &when, AppliedPosition: 21,
	})
	require.True(t, ok)
	assert.Equal(t, "third", st.Messages(domain.HistoryKey{ChatID: 1})[0].Text)
}

// Not every source carries a position: our own optimistic edit has none until
// the server answers, and a refetched history page has none at all. They apply
// unconditionally and leave the stored position alone.
func TestApplyEdit_WithoutAPositionAlwaysApplies(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "first"})
	when := time.Now()

	_, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "second", EditDate: &when, AppliedPosition: 20,
	})
	require.True(t, ok)

	_, ok = s.ApplyEdit(domain.Message{ID: 5, ChatID: 1, Text: "from a page", EditDate: &when})
	require.True(t, ok, "a copy with no position is not a late copy")
	assert.Equal(t, "from a page", st.Messages(domain.HistoryKey{ChatID: 1})[0].Text)

	// The recorded position did not move, so the stream still picks up at 21.
	_, ok = s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "third", EditDate: &when, AppliedPosition: 21,
	})
	require.True(t, ok)
	assert.Equal(t, "third", st.Messages(domain.HistoryKey{ChatID: 1})[0].Text)
}

// Telegram delivers a 1:1 peer reaction as a hidden edit carrying the message's
// whole current state. The reactions must be applied and the message must not
// be labelled edited (#118, #160).
func TestApplyEdit_HiddenEditAppliesReactionsWithoutTheLabel(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "hi"})
	when := time.Now()

	chg, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "hi", EditDate: &when, EditHidden: true,
		Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
		HasUnreadReactions: true,
	})

	require.True(t, ok)
	assert.Equal(t, 5, chg.MsgID)
	assert.True(t, chg.ReactionsUnread)
	assert.True(t, chg.UnreadReactionChanged)
	got := st.Messages(domain.HistoryKey{ChatID: 1})[0]
	require.Len(t, got.Reactions, 1, "the reaction carried by the hidden edit must be applied")
	assert.False(t, got.ShowsEdited(), "a hidden edit must not label the message edited")
}

func TestApplyEdit_ClearsUnreadWhenReadRemotely(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "hi"})
	when := time.Now()

	chg, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "hi", EditDate: &when, EditHidden: true,
		Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
		HasUnreadReactions: true,
	})
	require.True(t, ok)
	assert.True(t, chg.UnreadReactionChanged)
	c, _ := st.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)

	// Reactions read on another client: delivered as an edit with HasUnreadReactions false.
	chg, ok = s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "hi", EditDate: &when, EditHidden: true,
		Reactions:          []domain.Reaction{{Emoji: "👍", Count: 1}},
		HasUnreadReactions: false,
	})
	require.True(t, ok)
	assert.True(t, chg.UnreadReactionChanged)
	c, _ = st.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)
}

// An edit whose message has no reactions field says nothing about reactions,
// so the set stays; one whose field is there but empty says there are none
// (#248). A nil set is the first, an empty one the second.
func TestApplyEdit_ReactionsOnlyWhenTheEditCarriesThem(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	liked := []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "hi", Reactions: liked})

	_, ok := s.ApplyEdit(domain.Message{ID: 5, ChatID: 1, Text: "hi", AppliedPosition: 10})
	require.True(t, ok)
	assert.Equal(t, liked, st.Messages(domain.HistoryKey{ChatID: 1})[0].Reactions, "an edit without the field must leave the set alone")

	_, ok = s.ApplyEdit(domain.Message{ID: 5, ChatID: 1, Text: "hi", AppliedPosition: 11,
		Reactions: []domain.Reaction{}})
	require.True(t, ok)
	assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1})[0].Reactions, "an edit with an empty set clears it")
}

// An edit update for a message nobody ever edited carries no edit date at all.
// Applying it must not invent one.
func TestApplyEdit_WithoutAnEditDateLeavesTheMessageUnlabelled(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1, Text: "hi"})

	_, ok := s.ApplyEdit(domain.Message{
		ID: 5, ChatID: 1, Text: "hi",
		Reactions: []domain.Reaction{{Emoji: "👍", Count: 1}},
	})

	require.True(t, ok)
	got := st.Messages(domain.HistoryKey{ChatID: 1})[0]
	assert.Nil(t, got.EditDate, "nothing was edited, so nothing records an edit time")
	assert.False(t, got.ShowsEdited())
	require.Len(t, got.Reactions, 1)
}

func TestApplyReactions_TracksUnreadOnce(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1})

	chg, ok := s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 1}}, true, "reactions update")
	require.True(t, ok)
	assert.True(t, chg.UnreadReactionChanged)

	// Same message again: the count is already tracked.
	chg, ok = s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 2}}, true, "reactions update")
	require.True(t, ok)
	assert.False(t, chg.UnreadReactionChanged)
	c, _ := st.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)
}

func TestApplyReactions_ClearsUnreadWhenReadRemotely(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1})

	chg, ok := s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 1}}, true, "reactions update")
	require.True(t, ok)
	assert.True(t, chg.UnreadReactionChanged)
	assert.True(t, chg.ReactionsUnread)
	c, _ := st.GetChat(1)
	assert.Equal(t, 1, c.UnreadReactionsCount)

	// Reactions read on another client: unread is false.
	chg, ok = s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 1}}, false, "reactions update")
	require.True(t, ok)
	assert.True(t, chg.UnreadReactionChanged)
	assert.False(t, chg.ReactionsUnread)
	c, _ = st.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)

	// Idempotent: repeating the read reaction update does not decrement further.
	chg, ok = s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 1}}, false, "reactions update")
	require.True(t, ok)
	assert.False(t, chg.UnreadReactionChanged)
	c, _ = st.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)

	// An untracked message receiving a read reaction update changes nothing.
	chg, ok = s.ApplyReactions(1, 999, []domain.Reaction{{Emoji: "👍", Count: 1}}, false, "reactions update")
	require.True(t, ok)
	assert.False(t, chg.UnreadReactionChanged)
	c, _ = st.GetChat(1)
	assert.Equal(t, 0, c.UnreadReactionsCount)
}

// The reaction trace (#248) names where a set came from: an edit and a
// reactions update can describe the same change and disagree about it.
func TestReactionWrites_NameTheirSource(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	st, err := store.NewSQLite(":memory:", zap.New(core))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	s := state.New(st)
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1})

	s.ApplyReactions(1, 5, []domain.Reaction{{Emoji: "👍", Count: 1, IsChosen: true}}, false, "optimistic")
	s.ApplyEdit(domain.Message{ID: 5, ChatID: 1, AppliedPosition: 10,
		Reactions: []domain.Reaction{{Emoji: "👍", Count: 2}}})

	var vias []string
	for _, e := range logs.FilterMessage("reaction: store write").All() {
		vias = append(vias, e.ContextMap()["via"].(string))
	}
	assert.Equal(t, []string{"optimistic", "edit"}, vias)
}

func TestApplyDelete_WithChatID(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1})
	st.AppendMessage(domain.Message{ID: 6, ChatID: 1})

	chg, ok := s.ApplyDelete(1, []int{5})

	require.True(t, ok)
	assert.Equal(t, state.ChangeMessagesDeleted, chg.Kind)
	assert.Equal(t, int64(1), chg.ChatID)
	assert.Equal(t, []int{5}, chg.MsgIDs)
	require.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 1)
	assert.Equal(t, 6, st.Messages(domain.HistoryKey{ChatID: 1})[0].ID)
}

// A non-channel delete arrives with no peer context; the store resolves each ID
// to its owning chat through its index (#72).
func TestApplyDelete_WithoutChatIDResolvesThroughStore(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1, Peer: domain.Peer{ID: 1, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: 5, ChatID: 1})

	chg, ok := s.ApplyDelete(0, []int{5})

	require.True(t, ok)
	assert.Equal(t, int64(0), chg.ChatID)
	assert.Empty(t, st.Messages(domain.HistoryKey{ChatID: 1}))
}

// ApplyHistory is how a page replaces a chat's history outright, and
// MergeHistory is how one joins what is already there. Both commit through
// state, so projections rebuild from one place rather than from a client's
// reply handler.
func TestApplyHistory_StoresTheWindowAndPublishesOneChange(t *testing.T) {
	s, st := newState(t)
	var seen []state.Change
	s.OnChange(func(c state.Change) { seen = append(seen, c) })
	st.SetChat(domain.Chat{ID: 1})

	chg, ok := s.ApplyHistory(1, []domain.Message{
		{ID: 1, ChatID: 1, Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 1, Date: time.Unix(2, 0)},
	})

	require.True(t, ok)
	assert.Equal(t, state.ChangeHistory, chg.Kind)
	assert.Equal(t, int64(1), chg.ChatID)
	assert.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 2)
	assert.Len(t, seen, 1)
}

func TestApplyHistory_ReplacesTheStoredMessages(t *testing.T) {
	s, st := newState(t)
	st.SetChat(domain.Chat{ID: 1})
	s.ApplyHistory(1, []domain.Message{{ID: 5, ChatID: 1, Date: time.Unix(5, 0)}})

	s.ApplyHistory(1, []domain.Message{
		{ID: 4, ChatID: 1, Date: time.Unix(4, 0)},
		{ID: 5, ChatID: 1, Date: time.Unix(5, 0)},
	})

	assert.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 2, "the caller merges; state stores what it is given")
}

func TestMergeHistory_JoinsThePageAndPublishesOneChange(t *testing.T) {
	s, st := newState(t)
	var seen []state.Change
	st.SetChat(domain.Chat{ID: 1})
	st.SetMessages(1, []domain.Message{{ID: 3, ChatID: 1, Date: time.Unix(3, 0)}})
	s.OnChange(func(c state.Change) { seen = append(seen, c) })

	chg, ok := s.MergeHistory(domain.HistoryKey{ChatID: 1}, []domain.Message{
		{ID: 1, ChatID: 1, Date: time.Unix(1, 0)},
		{ID: 2, ChatID: 1, Date: time.Unix(2, 0)},
	})

	require.True(t, ok)
	assert.Equal(t, state.ChangeHistory, chg.Kind)
	assert.Equal(t, int64(1), chg.ChatID)
	assert.Len(t, st.Messages(domain.HistoryKey{ChatID: 1}), 3, "the page joins what was held rather than replacing it")
	assert.Len(t, seen, 1)
}

func TestMergeHistory_PublishesNothingForAPageThatAddsNothing(t *testing.T) {
	s, st := newState(t)
	var seen []state.Change
	st.SetChat(domain.Chat{ID: 1})
	held := []domain.Message{{ID: 1, ChatID: 1, Date: time.Unix(1, 0)}}
	st.SetMessages(1, held)
	s.OnChange(func(c state.Change) { seen = append(seen, c) })

	_, ok := s.MergeHistory(domain.HistoryKey{ChatID: 1}, held)

	assert.False(t, ok, "reaching history the store already had is not news")
	assert.Empty(t, seen)
}

// A refreshed file reference is not an edit: it changes how the media is
// addressed, not what the message says, so it must not set an edited marker.
func TestApplyMediaRef_ReplacesTheReferenceAndPublishes(t *testing.T) {
	s, st := newState(t)
	st.SetMessages(1, []domain.Message{{
		ID: 5, ChatID: 1, Date: time.Unix(1, 0),
		Photo: &domain.PhotoRef{ID: 9, FileReference: []byte("stale")},
	}})
	var seen []state.Change
	s.OnChange(func(c state.Change) { seen = append(seen, c) })

	chg, ok := s.ApplyMediaRef(1, 5, &domain.PhotoRef{ID: 9, FileReference: []byte("fresh")}, nil)

	require.True(t, ok)
	assert.Equal(t, state.ChangeMediaRef, chg.Kind)
	assert.Equal(t, int64(1), chg.ChatID)
	assert.Equal(t, 5, chg.MsgID)
	require.Len(t, seen, 1)
	got := st.Messages(domain.HistoryKey{ChatID: 1})
	require.Len(t, got, 1)
	assert.Equal(t, []byte("fresh"), got[0].Photo.FileReference)
	assert.Nil(t, got[0].EditDate, "refreshing a reference must not mark the message edited")
}
