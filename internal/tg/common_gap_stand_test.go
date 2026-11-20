package tg

import (
	"context"
	"fmt"
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

// The stand for #267. A bot rewrites one message thirty times, a pts gap opens
// in the middle of the stream, and Telegram answers the catch-up the way it
// really does: with the edits condensed into the single final one. Everything
// below the wire is real - the gotd manager, our dispatcher, the store - so
// what it measures is what a person would read in the chat afterwards.
//
// It exists because this class of defect is invisible from the inside. The
// account's position ends up exactly where the server says it should, every
// counter agrees, and the only thing that is wrong is the text on screen.
//
// tele used to deliver the updates of a recovered difference itself, because
// gotd fed them back through the sequence the gap was stuck in and then
// discarded them as outdated. gotd v0.164.0 applies them directly
// (gotd/td#1853), so the manager now talks to the API straight, and this stand
// holds that behaviour against any later gotd. The outbox stand reuses its API.
const (
	standChatID  = int64(10)
	standMsgID   = 5
	standStartTS = 100 // the pts the account starts from
	standEndPts  = 130 // where the server's state ends up
)

type standAPI struct {
	mu sync.Mutex
	// armed switches the difference from "nothing pending", which is what the
	// manager's startup catch-up must see, to the one recovering the gap.
	armed bool
	diff  tg.UpdatesDifferenceClass
	asked []int
}

func (a *standAPI) arm() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.armed = true
}

func (a *standAPI) asks() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.asked)
}

// askedFrom is the pts each catch-up started from, which is how a chase after
// the gap is told from the one the manager runs at startup.
func (a *standAPI) askedFrom() []int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]int(nil), a.asked...)
}

func (a *standAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{Pts: standStartTS, Qts: 0, Seq: 0, Date: int(time.Now().Unix())}, nil
}

func (a *standAPI) UpdatesGetDifference(
	_ context.Context, req *tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.asked = append(a.asked, req.Pts)
	if !a.armed || req.Pts >= standEndPts {
		return &tg.UpdatesDifferenceEmpty{Date: int(time.Now().Unix())}, nil
	}
	return a.diff, nil
}

func (a *standAPI) UpdatesGetChannelDifference(
	context.Context, *tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{Final: true}, nil
}

// standUser is carried by every envelope: the manager refuses to apply a
// message update whose peer it cannot resolve, and fetches a difference instead.
func standUser() []tg.UserClass {
	return []tg.UserClass{&tg.User{ID: standChatID, AccessHash: 42}}
}

func standEdit(pts int, text string) *tg.Updates {
	msg := &tg.Message{
		ID:       standMsgID,
		Message:  text,
		PeerID:   &tg.PeerUser{UserID: standChatID},
		Date:     int(time.Now().Unix()),
		EditDate: int(time.Now().Unix()),
		EditHide: true, // a bot's own rewrite carries no "edited" label
	}
	msg.FromID = &tg.PeerUser{UserID: standChatID}
	return &tg.Updates{
		Updates: []tg.UpdateClass{&tg.UpdateEditMessage{Message: msg, Pts: pts, PtsCount: 1}},
		Users:   standUser(),
		Date:    int(time.Now().Unix()),
	}
}

func editUpdate(msgID, pts int, text string) *tg.UpdateEditMessage {
	msg := &tg.Message{ID: msgID, Message: text, PeerID: &tg.PeerUser{UserID: 10}}
	msg.FromID = &tg.PeerUser{UserID: 10}
	return &tg.UpdateEditMessage{Message: msg, Pts: pts, PtsCount: 1}
}

// stand wires the real manager to the real dispatcher and store, with the API
// faked at the wire.
func stand(t *testing.T) (*updates.Manager, *standAPI, store.Store) {
	t.Helper()

	events := make(chan store.Event, 256)
	droppable := make(chan store.Event, 64)
	dispatcher := tg.NewUpdateDispatcher()
	setupDispatcher(&dispatcher, events, droppable, zap.NewNop(),
		func(int) bool { return false }, newNameCache())

	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: standChatID, Peer: domain.Peer{ID: standChatID, Type: domain.PeerUser}})
	st.AppendMessage(domain.Message{ID: standMsgID, ChatID: standChatID, Text: "thinking"})

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

	api := &standAPI{diff: &tg.UpdatesDifference{
		// Telegram condenses a difference: thirty rewrites of one message come
		// back as the one edit that says what it holds now.
		OtherUpdates: []tg.UpdateClass{
			editUpdate(standMsgID, standEndPts, "the whole answer"),
		},
		Users: standUser(),
		State: tg.UpdatesState{Pts: standEndPts, Date: int(time.Now().Unix())},
	}}

	manager := updates.New(updates.Config{Handler: dispatcher})
	started := make(chan struct{})
	go func() {
		_ = manager.Run(ctx, api, 1, updates.AuthOptions{
			OnStart: func(context.Context) { close(started) },
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the updates manager never started")
	}
	// The manager chases a difference the moment it starts, before any of this
	// has happened. Wait it out unarmed, so the only difference that carries
	// anything is the one the gap asks for.
	require.Eventually(t, func() bool { return api.asks() > 0 }, 5*time.Second, 10*time.Millisecond,
		"the manager never ran its startup catch-up")
	return manager, api, st
}

// streamWithAGap feeds the first edit, then jumps the stream forward: the
// middle of it never arrives on the wire, which is what opens the gap.
func streamWithAGap(t *testing.T, manager *updates.Manager) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, manager.Handle(ctx, standEdit(standStartTS+1, "th")))
	for pts := standStartTS + 11; pts <= standEndPts; pts++ {
		require.NoError(t, manager.Handle(ctx, standEdit(pts, fmt.Sprintf("part %d", pts))))
	}
}

func storedText(st store.Store) string {
	msgs := st.Messages(domain.HistoryKey{ChatID: standChatID})
	if len(msgs) == 0 {
		return ""
	}
	return msgs[0].Text
}

func TestStand_TheFinalTextSurvivesACommonPtsGap(t *testing.T) {
	manager, api, st := stand(t)
	api.arm()

	streamWithAGap(t, manager)

	require.Eventually(t, func() bool {
		return storedText(st) == "the whole answer"
	}, 5*time.Second, 20*time.Millisecond,
		"the text recovered by the difference must reach the chat, got %q", storedText(st))

	assert.Contains(t, api.askedFrom(), standStartTS+1,
		"the text must come from the catch-up the gap asked for, not the one at startup")
	assert.False(t, st.Messages(domain.HistoryKey{ChatID: standChatID})[0].ShowsEdited(),
		"and a bot's rewrite still carries no edited label")
}
