package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core"
	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

var chatOne = domain.HistoryKey{ChatID: 1}

func jumpMsgs(lo, hi int) []domain.Message {
	out := make([]domain.Message, 0, hi-lo+1)
	for id := lo; id <= hi; id++ {
		out = append(out, domain.Message{ID: id, ChatID: 1, Text: "m", Date: time.Unix(int64(id), 0)})
	}
	return out
}

// rootOnTail builds a root on chat 1 open at its tail, 1001..1020, with a
// history limit of 50.
func rootOnTail(t *testing.T) (RootModel, *ownerStub) {
	t.Helper()
	st := store.NewMemory()
	st.SetChat(domain.Chat{ID: 1, Title: "Alice", Peer: domain.Peer{ID: 1, Type: domain.PeerUser}, ReadInboxMaxID: 1020})
	st.SetMessages(1, jumpMsgs(1001, 1020))
	m := newRootInternal(st, 50)
	m = m.WithScreen(ScreenMain)
	// In a running client this arrives with the account's first contents.
	m.historyLimit = 50
	o := m.owner.(*ownerStub)
	next, _ := m.Update(screens.OpenChatMsg{ChatID: 1, Title: "Alice"})
	next, _ = o.drain(next.(RootModel))
	return next.(RootModel), o
}

// runDrained runs one message through the root and drains what the owner queued.
func runDrained(t *testing.T, m RootModel, o *ownerStub, msg tea.Msg) RootModel {
	t.Helper()
	next, _ := m.Update(msg)
	next, _ = o.drain(next.(RootModel))
	return next.(RootModel)
}

// jumpedFar is rootOnTail after a jump to 500, with the page around it fetched.
func jumpedFar(t *testing.T) (RootModel, *ownerStub) {
	t.Helper()
	m, o := rootOnTail(t)
	m = runDrained(t, m, o, components.JumpToMsgRequest{MsgID: 500})
	o.state.MergeAround(chatOne, jumpMsgs(475, 525))
	next, _ := o.drain(m)
	return next.(RootModel), o
}

// Reaching the bottom of a run far back asks for a page more below it, the
// way reaching the top asks for one above.
func TestRoot_TheBottomOfAFarRunWidensTheWindowDown(t *testing.T) {
	m, o := jumpedFar(t)

	m = runDrained(t, m, o, screens.LoadNewerMsg{ChatID: 1})

	assert.Equal(t, 75, m.chatWindow.After)
	assert.Equal(t, 25, m.chatWindow.Before)
}

// At the tail there is nothing newer to ask for.
func TestRoot_TheBottomOfTheTailRunAsksForNothing(t *testing.T) {
	m, o := rootOnTail(t)

	m = runDrained(t, m, o, screens.LoadNewerMsg{ChatID: 1})

	assert.Zero(t, m.chatWindow.After)
}

// A run far back that grows until it meets the tail run is the tail run: the
// window becomes the one opening the chat gives, pinned where it stands, so it
// follows what arrives and the scroll does not move.
func TestRoot_AWindowWhoseRunReachesTheTailFollowsIt(t *testing.T) {
	m, o := rootOnTail(t)
	m = runDrained(t, m, o, components.JumpToMsgRequest{MsgID: 990})
	o.state.MergeAround(chatOne, jumpMsgs(965, 1000))
	next, _ := o.drain(m)
	m = next.(RootModel)
	require.False(t, m.chatAtTail)

	o.state.MergeNewer(chatOne, 1000, jumpMsgs(1001, 1010))
	next, _ = o.drain(m)
	m = next.(RootModel)

	assert.True(t, m.chatAtTail)
	assert.Equal(t, project.Anchor{Kind: project.AnchorFirstUnread, MsgID: 990}, m.chatWindow.Anchor)
	assert.Equal(t, 25, m.chatWindow.Before)
}

// go_bottom away from the tail goes back to the newest messages: the window
// the chat opens with.
func TestRoot_GoBottomAwayFromTheTailReturnsToIt(t *testing.T) {
	m, o := jumpedFar(t)

	m = runDrained(t, m, o, screens.GoToLatestMsg{ChatID: 1})

	assert.Equal(t, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorFirstUnread}, Before: 50,
	}, m.chatWindow)
	assert.True(t, m.chatAtTail)
}

// On the tail run go_bottom only scrolls: the window is already there.
func TestRoot_GoBottomOnTheTailLeavesTheWindow(t *testing.T) {
	m, o := rootOnTail(t)
	before := m.chatWindow

	m = runDrained(t, m, o, screens.GoToLatestMsg{ChatID: 1})

	assert.Equal(t, before, m.chatWindow)
}

// A jump to a message that is gone opens the history where it was, on the
// nearest message Telegram returned, with nothing highlighted, and says so.
func TestRoot_AJumpToAMessageThatIsGoneOpensNearIt(t *testing.T) {
	m, o := rootOnTail(t)
	m = runDrained(t, m, o, components.JumpToMsgRequest{MsgID: 500})

	m = runDrained(t, m, o, core.MessageGone{ChatID: 1, MsgID: 500, Nearest: 499})

	assert.Equal(t, "This message is gone", m.statusBar.Status())
	assert.Equal(t, project.Anchor{Kind: project.AnchorMessage, MsgID: 499}, m.chatWindow.Anchor)
	o.state.MergeAround(chatOne, append(jumpMsgs(475, 499), jumpMsgs(501, 525)...))
	next, _ := o.drain(m)
	m = next.(RootModel)
	assert.Zero(t, m.Chat().HighlightedMsgID(), "nothing to highlight: the message is not there")
}

// With nothing returned around it, the window goes back where it was.
func TestRoot_AJumpToAMessageThatIsGoneWithNothingNearStaysPut(t *testing.T) {
	m, o := rootOnTail(t)
	before := m.chatWindow
	m = runDrained(t, m, o, components.JumpToMsgRequest{MsgID: 500})

	m = runDrained(t, m, o, core.MessageGone{ChatID: 1, MsgID: 500})

	assert.Equal(t, "This message is gone", m.statusBar.Status())
	assert.Equal(t, before, m.chatWindow)
}

// A message arriving while the window is far back is counted below it, and
// going back to the tail clears the count.
func TestRoot_WhatLandsAtTheTailIsCountedWhileFarBack(t *testing.T) {
	m, o := jumpedFar(t)

	next, _ := applyEventInternal(t, m, o.state.Store(), store.Event{Kind: store.EventNewMessage,
		Message: domain.Message{ID: 1021, ChatID: 1, Text: "new", Date: time.Unix(1021, 0)}})
	m = next.(RootModel)
	assert.Equal(t, 1, m.Chat().NewCount())

	m = runDrained(t, m, o, screens.GoToLatestMsg{ChatID: 1})
	assert.Zero(t, m.Chat().NewCount())
}

// A reply's original far back is not in the window: the window moves onto it,
// half the history limit either side, and the message is highlighted the
// moment the owner has fetched it.
func TestRoot_JumpToAMessageOutsideTheWindowMovesTheWindow(t *testing.T) {
	m, o := rootOnTail(t)

	m = runDrained(t, m, o, components.JumpToMsgRequest{MsgID: 500})

	assert.Equal(t, project.HistoryWindow{
		ChatID: 1, Anchor: project.Anchor{Kind: project.AnchorMessage, MsgID: 500}, Before: 25, After: 25,
	}, m.chatWindow)

	o.state.MergeAround(chatOne, jumpMsgs(475, 525))
	next, _ := o.drain(m)
	m = next.(RootModel)

	require.Equal(t, 500, m.Chat().HighlightedMsgID())
	assert.NotEqual(t, "Not in buffer", m.statusBar.Status())
}
