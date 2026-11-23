package project_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
)

// recorder stands in for the client: it takes every delta the registry emits
// and hands them back in the order they arrived.
type recorder struct {
	mu  sync.Mutex
	got []project.Delta
}

func (r *recorder) emit(d project.Delta) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, d)
	return true
}

// take returns what arrived since the last take.
func (r *recorder) take() []project.Delta {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.got
	r.got = nil
	return out
}

func newRegistry(r project.Reader) (*project.Registry, *recorder) {
	rec := &recorder{}
	return project.NewRegistry(r, rec.emit), rec
}

// lastSub is the last id each registry was handed, so a test subscribes the
// way a client does: under ids it chooses and never reuses.
var (
	lastSubMu sync.Mutex
	lastSub   = map[*project.Registry]project.SubID{}
)

// subscribe registers w on g under the next id and returns it.
func subscribe(g *project.Registry, w project.Window) project.SubID {
	lastSubMu.Lock()
	lastSub[g]++
	id := lastSub[g]
	lastSubMu.Unlock()
	g.Subscribe(id, w)
	return id
}

func TestRegistry_SubscribeRepliesWithCurrentContents(t *testing.T) {
	r := &fakeReader{chats: chats(3)}
	g, rec := newRegistry(r)

	id := subscribe(g, project.ChatListWindow{Limit: 10})
	deltas := rec.take()

	require.NotZero(t, id)
	require.Len(t, deltas, 1)
	require.NotNil(t, deltas[0].ChatList)
	assert.Equal(t, project.ChatListReset, deltas[0].ChatList.Kind,
		"the first delta is a Reset, which is what makes resubscribing a resync")
	assert.Equal(t, id, deltas[0].Sub)
}

func TestRegistry_ResubscribeIsResync(t *testing.T) {
	r := &fakeReader{chats: chats(3)}
	g, rec := newRegistry(r)
	id1 := subscribe(g, project.ChatListWindow{Limit: 10})
	g.Unsubscribe(id1)
	r.chats = chats(5)
	rec.take()

	subscribe(g, project.ChatListWindow{Limit: 10})
	deltas := rec.take()

	require.Len(t, deltas, 1)
	assert.Len(t, deltas[0].ChatList.Rows, 5)
}

func TestRegistry_RefreshEmitsNothingWhenNothingChanged(t *testing.T) {
	r := &fakeReader{chats: chats(3)}
	g, rec := newRegistry(r)
	subscribe(g, project.ChatListWindow{Limit: 10})
	rec.take()

	g.Refresh()

	assert.Empty(t, rec.take())
}

func TestRegistry_RefreshEmitsNothingForAChangeOutsideTheWindow(t *testing.T) {
	all := chats(50)
	r := &fakeReader{chats: all}
	g, rec := newRegistry(r)
	subscribe(g, project.ChatListWindow{Offset: 0, Limit: 10})
	rec.take()

	all[40].Online = true // a presence update for a chat nowhere near the screen
	r.chats = all
	g.Refresh()

	assert.Empty(t, rec.take(),
		"presence streams continuously for every online contact; only the window pays")
}

func TestRegistry_RefreshEmitsARowForAnInWindowChange(t *testing.T) {
	all := chats(10)
	r := &fakeReader{chats: all}
	g, rec := newRegistry(r)
	id := subscribe(g, project.ChatListWindow{Offset: 0, Limit: 5})
	rec.take()

	all[2].Online = true
	r.chats = all

	g.Refresh()
	deltas := rec.take()

	require.Len(t, deltas, 1)
	assert.Equal(t, id, deltas[0].Sub)
	assert.Equal(t, project.ChatListRow, deltas[0].ChatList.Kind)
	assert.Equal(t, int64(3), deltas[0].ChatList.Row.ID)
}

func TestRegistry_RefreshFansOutToEverySubscription(t *testing.T) {
	all := chats(10)
	r := &fakeReader{chats: all, msgs: map[int64][]domain.Message{1: msgs(3)}}
	g, rec := newRegistry(r)
	listID := subscribe(g, project.ChatListWindow{Limit: 10})
	chatID := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10,
	})
	rec.take()

	all[0].Title = "renamed"
	r.chats = all
	r.msgs[1] = msgs(4)

	g.Refresh()
	deltas := rec.take()

	var sawList, sawChat bool
	for _, d := range deltas {
		if d.Sub == listID && d.ChatList != nil {
			sawList = true
		}
		if d.Sub == chatID && d.History != nil {
			sawChat = true
		}
	}
	assert.True(t, sawList)
	assert.True(t, sawChat)
}

func TestRegistry_MoveWindowReplacesTheWindowAndEmits(t *testing.T) {
	r := &fakeReader{chats: chats(50)}
	g, rec := newRegistry(r)
	id := subscribe(g, project.ChatListWindow{Offset: 0, Limit: 5})
	rec.take()

	g.MoveWindow(id, project.ChatListWindow{Offset: 20, Limit: 5})
	deltas := rec.take()

	require.Len(t, deltas, 1)
	require.Equal(t, project.ChatListReset, deltas[0].ChatList.Kind)
	assert.Equal(t, 20, deltas[0].ChatList.Offset)
	assert.Equal(t, int64(21), deltas[0].ChatList.Rows[0].ID)
}

func TestRegistry_MoveWindowToTheSameWindowEmitsNothing(t *testing.T) {
	r := &fakeReader{chats: chats(50)}
	g, rec := newRegistry(r)
	w := project.ChatListWindow{Offset: 0, Limit: 5}
	id := subscribe(g, w)
	rec.take()

	g.MoveWindow(id, w)

	assert.Empty(t, rec.take(), "MoveWindow is idempotent")
}

func TestRegistry_UnsubscribedSubscriptionStopsEmitting(t *testing.T) {
	all := chats(10)
	r := &fakeReader{chats: all}
	g, rec := newRegistry(r)
	id := subscribe(g, project.ChatListWindow{Limit: 10})
	g.Unsubscribe(id)
	rec.take()

	all[0].Online = true
	r.chats = all
	g.Refresh()

	assert.Empty(t, rec.take())
}

func TestRegistry_MoveWindowOnAnUnknownSubscriptionIsANoOp(t *testing.T) {
	g, rec := newRegistry(&fakeReader{chats: chats(1)})

	g.MoveWindow(project.SubID(99), project.ChatListWindow{Limit: 5})

	assert.Empty(t, rec.take())
}

func TestRegistry_WindowReportsTheCurrentWindow(t *testing.T) {
	g, _ := newRegistry(&fakeReader{chats: chats(3)})
	id := subscribe(g, project.ChatListWindow{Offset: 0, Limit: 5})
	g.MoveWindow(id, project.ChatListWindow{Offset: 1, Limit: 5})

	w, ok := g.Window(id)

	require.True(t, ok)
	assert.Equal(t, project.ChatListWindow{Offset: 1, Limit: 5}, w)
}

func TestRegistry_PresenceReachesOnlyTheSubscribedChat(t *testing.T) {
	all := chats(2)
	r := &fakeReader{
		chats: all,
		msgs:  map[int64][]domain.Message{1: msgs(2), 2: msgs(2)},
	}
	g, rec := newRegistry(r)
	sub1 := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 5,
	})
	subscribe(g, project.HistoryWindow{
		ChatID: 2, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 5,
	})
	// Drain the two opening Resets.
	g.Refresh()
	rec.take()

	all[0].Online = true
	r.chats = all
	g.Refresh()
	deltas := rec.take()

	require.Len(t, deltas, 1)
	assert.Equal(t, sub1, deltas[0].Sub)
	assert.Equal(t, project.HistoryHeaderUpdate, deltas[0].History.Kind)
}

// Reading through a long unread tail advances the read pointer while the chat
// stays open. The window must stay where it was opened: recomputing the anchor
// would walk it forward and drop the messages above out from under the reader.
func TestRegistry_FirstUnreadWindowHoldsItsAnchorAsMessagesAreRead(t *testing.T) {
	chat := domain.Chat{ID: 1, UnreadCount: 4, ReadInboxMaxID: 6}
	r := readerWith(chat, msgs(10))
	g, rec := newRegistry(r)
	id := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2,
	})
	require.Len(t, rec.take(), 1)

	// The first screen is marked read: the pointer moves, the history does not.
	chat.ReadInboxMaxID = 8
	chat.UnreadCount = 2
	r.chats = []domain.Chat{chat}

	g.Refresh()

	for _, d := range rec.take() {
		assert.NotEqual(t, project.HistoryRemove, d.History.Kind,
			"messages already on screen must not leave the window when they are read")
		assert.NotEqual(t, project.HistoryReset, d.History.Kind,
			"a moved read pointer must not re-seat the message list")
	}
	w, ok := g.Window(id)
	require.True(t, ok)
	assert.Equal(t, 7, w.(project.HistoryWindow).Anchor.MsgID,
		"the anchor is pinned to the message the window opened on")
}

// Pinning must not outlive the message: a pinned anchor that is deleted falls
// back to resolving the anchor again rather than emptying the window.
func TestRegistry_PinnedAnchorFallsBackWhenItsMessageIsGone(t *testing.T) {
	chat := domain.Chat{ID: 1, UnreadCount: 4, ReadInboxMaxID: 6}
	all := msgs(10)
	r := readerWith(chat, all)
	g, rec := newRegistry(r)
	subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2,
	})

	// Message 7, the anchor, is deleted elsewhere.
	kept := append(append([]domain.Message{}, all[:6]...), all[7:]...)
	r.msgs[1] = kept

	g.Refresh()
	rec.take()

	g.Refresh()
	assert.Empty(t, rec.take(), "the window settles instead of rebuilding itself every time")
}

// Every delta is stated against the one before it, so a client that receives
// two in the opposite order draws something the owner never had. Two goroutines
// changing state close together must not be able to swap their deltas on the
// way out: the second refresh waits until the first one's delta is taken.
func TestRegistry_ARefreshDoesNotOvertakeTheDeltaBeforeIt(t *testing.T) {
	all := chats(3)
	r := &fakeReader{chats: all}

	var (
		mu       sync.Mutex
		got      []project.Delta
		hold     bool
		entered  = make(chan struct{})
		released = make(chan struct{})
	)
	emit := func(d project.Delta) bool {
		mu.Lock()
		block := hold
		hold = false
		mu.Unlock()
		if block {
			close(entered)
			<-released
		}
		mu.Lock()
		got = append(got, d)
		mu.Unlock()
		return true
	}
	g := project.NewRegistry(r, emit)
	subscribe(g, project.ChatListWindow{Limit: 10})

	// The first change is computed and held on its way to the client.
	mu.Lock()
	got = nil
	hold = true
	mu.Unlock()
	first := append([]domain.Chat{}, all...)
	first[0].Online = true
	r.chats = first
	firstDone := make(chan struct{})
	go func() {
		g.Refresh()
		close(firstDone)
	}()
	<-entered

	// A second change lands while the first delta is still in flight.
	second := append([]domain.Chat{}, first...)
	second[0].Title = "renamed"
	r.chats = second
	secondDone := make(chan struct{})
	go func() {
		g.Refresh()
		close(secondDone)
	}()

	select {
	case <-secondDone:
		t.Fatal("the second refresh emitted while the first delta was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(released)
	<-firstDone
	<-secondDone

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 2)
	assert.Equal(t, "chat", got[0].ChatList.Row.Title)
	assert.True(t, got[0].ChatList.Row.Online)
	assert.Equal(t, "renamed", got[1].ChatList.Row.Title,
		"the client ends on the newest row, not on the one computed first")
}

// dropOnce takes deltas like recorder, except that it refuses the first one
// addressed to the named subscription after it is armed, as a full client
// queue would.
type dropOnce struct {
	recorder
	sub   project.SubID
	armed bool
}

func (d *dropOnce) emit(delta project.Delta) bool {
	if d.armed && delta.Sub == d.sub {
		d.armed = false
		return false
	}
	return d.recorder.emit(delta)
}

// A delta the client never took leaves it with a copy the next delta cannot be
// applied to. The rest of that refresh is withheld from the subscription, and
// its next rebuild is the resync a fresh subscription gets, even when nothing
// has changed since. Other subscriptions carry on as they were.
func TestRegistry_ADroppedChatDeltaMakesTheNextOneAResync(t *testing.T) {
	all := chats(2)
	r := &fakeReader{chats: all, msgs: map[int64][]domain.Message{1: msgs(3), 2: msgs(3)}}
	rec := &dropOnce{}
	g := project.NewRegistry(r, rec.emit)
	dropped := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10,
	})
	subscribe(g, project.HistoryWindow{
		ChatID: 2, Anchor: project.Anchor{Kind: project.AnchorNewest}, Before: 10,
	})
	rec.take()

	// One change that is two deltas for the first chat: an append and a header.
	all[0].Title = "renamed"
	all[1].Title = "renamed too"
	r.chats = all
	r.msgs[1] = msgs(4)
	rec.sub, rec.armed = dropped, true
	g.Refresh()

	for _, d := range rec.take() {
		assert.NotEqual(t, dropped, d.Sub,
			"what follows a dropped delta is meaningless without it and is withheld")
	}

	g.Refresh()
	deltas := rec.take()

	require.Len(t, deltas, 1, "only the subscription that lost a delta is resent")
	assert.Equal(t, dropped, deltas[0].Sub)
	assert.Equal(t, project.HistoryReset, deltas[0].History.Kind)
	assert.Equal(t, "renamed", deltas[0].History.Contents.Title)
	assert.Len(t, deltas[0].History.Contents.Messages, 4)
}

func TestRegistry_ADroppedChatListDeltaMakesTheNextOneAResync(t *testing.T) {
	all := chats(3)
	r := &fakeReader{chats: all}
	rec := &dropOnce{}
	g := project.NewRegistry(r, rec.emit)
	id := subscribe(g, project.ChatListWindow{Limit: 10})
	rec.take()

	all[0].Online = true
	r.chats = all
	rec.sub, rec.armed = id, true
	g.Refresh()
	require.Empty(t, rec.take())

	g.Refresh()
	deltas := rec.take()

	require.NotEmpty(t, deltas)
	assert.Equal(t, project.ChatListReset, deltas[0].ChatList.Kind)
	assert.True(t, deltas[0].ChatList.Rows[0].Online)
}

// Scrolling up widens the window by replacing it, carrying the same anchor
// description the client opened with. The pin must survive that, or asking for
// older history would re-anchor the window on whatever is unread by then.
func TestRegistry_WideningKeepsThePinnedAnchor(t *testing.T) {
	chat := domain.Chat{ID: 1, UnreadCount: 4, ReadInboxMaxID: 6}
	r := readerWith(chat, msgs(10))
	g, _ := newRegistry(r)
	id := subscribe(g, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 2,
	})

	chat.ReadInboxMaxID = 9
	chat.UnreadCount = 1
	r.chats = []domain.Chat{chat}
	g.Refresh()

	g.MoveWindow(id, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 4,
	})

	w, ok := g.Window(id)
	require.True(t, ok)
	assert.Equal(t, 7, w.(project.HistoryWindow).Anchor.MsgID, "the pin survives a widen")
	assert.Equal(t, 4, w.(project.HistoryWindow).Before, "and the widen still took effect")
}
