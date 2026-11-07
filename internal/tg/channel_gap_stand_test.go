package tg

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/core/state"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// The stand for #266. A busy supergroup: Telegram answers every channel
// difference with a five-minute timeout, and a pts gap opens right after the
// manager subscribes to the channel. Everything below the wire is real - the
// gotd manager, our dispatcher, the store - so what it measures is whether the
// message behind the gap reaches the chat while a person is still looking.
const (
	chanStandID      = int64(20)
	chanStandHash    = int64(77)
	chanStandPts     = 10  // the channel pts the subscribe catch-up reports
	chanStandTimeout = 300 // seconds, what Telegram sends for a busy supergroup
)

type chanStandAPI struct {
	mu    sync.Mutex
	asked []int
}

// asks is how many channel differences the manager has asked for. The first is
// the subscribe catch-up; any after it is the gap being chased.
func (a *chanStandAPI) asks() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.asked)
}

func (a *chanStandAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{Pts: 1, Date: int(time.Now().Unix())}, nil
}

func (a *chanStandAPI) UpdatesGetDifference(
	context.Context, *tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	return &tg.UpdatesDifferenceEmpty{Date: int(time.Now().Unix())}, nil
}

// UpdatesGetChannelDifference answers the subscribe with an empty difference
// and the gap with the two messages it missed. Both carry the timeout, as
// Telegram's answers for a busy supergroup do.
func (a *chanStandAPI) UpdatesGetChannelDifference(
	_ context.Context, req *tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked = append(a.asked, req.Pts)
	if req.Pts < chanStandPts+1 {
		d := &tg.UpdatesChannelDifferenceEmpty{Final: true, Pts: chanStandPts}
		d.SetTimeout(chanStandTimeout)
		return d, nil
	}
	d := &tg.UpdatesChannelDifference{
		Final:       true,
		Pts:         chanStandPts + 3,
		NewMessages: []tg.MessageClass{chanStandMsg(12), chanStandMsg(13)},
		Chats:       chanStandChats(),
	}
	d.SetTimeout(chanStandTimeout)
	return d, nil
}

// chanStandChats is the channel as the envelope carries it. The access hash is
// optional in the schema and the manager reads its flag, not the value, so it
// has to be set through the setter or the channel is never tracked.
func chanStandChats() []tg.ChatClass {
	ch := &tg.Channel{ID: chanStandID, Megagroup: true}
	ch.SetAccessHash(chanStandHash)
	return []tg.ChatClass{ch}
}

func chanStandMsg(id int) *tg.Message {
	return &tg.Message{
		ID:      id,
		Message: "message",
		PeerID:  &tg.PeerChannel{ChannelID: chanStandID},
		Date:    int(time.Now().Unix()),
	}
}

func chanStandUpdate(msgID, pts int) *tg.Updates {
	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateNewChannelMessage{
			Message: chanStandMsg(msgID), Pts: pts, PtsCount: 1,
		}},
		Chats: chanStandChats(),
		Date:  int(time.Now().Unix()),
	}
}

// chanStand wires the real manager to the real dispatcher and store, with the
// API faked at the wire. withWrapper says whether the manager talks to the API
// through the cooldown-stripping wrapper or straight to it.
func chanStand(t *testing.T, withWrapper bool) (*updates.Manager, *chanStandAPI, store.Store) {
	t.Helper()

	events := make(chan store.Event, 256)
	droppable := make(chan store.Event, 64)
	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, events, droppable, zap.NewNop(),
		func(int) bool { return false }, newNameCache())

	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: chanStandID, Peer: domain.Peer{
		ID: chanStandID, Type: domain.PeerChannel, AccessHash: chanStandHash,
	}})

	s := state.New(st)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		for {
			select {
			case evt := <-events:
				state.Apply(s, evt)
			case <-droppable:
			case <-ctx.Done():
				return
			}
		}
	}()

	api := &chanStandAPI{}
	var raw updates.API = api
	if withWrapper {
		raw = newChannelDiffAPI(api, zap.NewNop())
	}

	manager := updates.New(updates.Config{Handler: dispatcher})
	started := make(chan struct{})
	go func() {
		_ = manager.Run(ctx, raw, 1, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the updates manager never started")
	}
	return manager, api, st
}

// subscribeThenGap delivers the first message, which makes the manager
// subscribe to the channel, then one that skips a pts: message 12 never
// arrives on the wire.
func subscribeThenGap(t *testing.T, manager *updates.Manager, api *chanStandAPI) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, manager.Handle(ctx, chanStandUpdate(11, chanStandPts+1)))
	require.Eventually(t, func() bool { return api.asks() > 0 }, 5*time.Second, 10*time.Millisecond,
		"the manager never subscribed to the channel")
	require.NoError(t, manager.Handle(ctx, chanStandUpdate(13, chanStandPts+3)))
}

func holdsMessage(st store.Store, id int) bool {
	for _, m := range st.Messages(domain.HistoryKey{ChatID: chanStandID}) {
		if m.ID == id {
			return true
		}
	}
	return false
}

func TestStand_AChannelGapClosesDespiteTheCooldown(t *testing.T) {
	manager, api, st := chanStand(t, true)

	subscribeThenGap(t, manager, api)

	require.Eventually(t, func() bool { return holdsMessage(st, 12) }, 5*time.Second, 20*time.Millisecond,
		"the message behind the gap must reach the chat within the gap timer, not after the cooldown")
	require.Eventually(t, func() bool { return holdsMessage(st, 13) }, 5*time.Second, 20*time.Millisecond,
		"and so must the one that revealed the gap")
}

// The same stand with the manager talking straight to the API, which is what
// gotd does on its own. It reproduces the defect: the subscribe catch-up arms a
// five-minute ban, the gap timer fires inside it, and the manager sits the ban
// out instead of asking, discarding the channel's updates meanwhile.
//
// When this test starts failing, gotd/td#1852 has landed: delete the wrapper,
// this test, and the entry both have in docs/gotd-workarounds.md (#270).
func TestStand_WithoutTheWrapperAChannelGapWaitsOutTheCooldown(t *testing.T) {
	manager, api, st := chanStand(t, false)

	subscribeThenGap(t, manager, api)

	// Well past the manager's 500ms gap timer: with nothing holding it back the
	// gap would have been chased by now.
	time.Sleep(2 * time.Second)

	assert.Equal(t, 1, api.asks(),
		"upstream no longer treats the timeout as a ban; the wrapper and this test can go")
	assert.False(t, holdsMessage(st, 12), "the message behind the gap is still missing")
}
