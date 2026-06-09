package tg

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
)

func TestToggleInputPeer_AddsWhenAbsent(t *testing.T) {
	peers := []tg.InputPeerClass{&tg.InputPeerUser{UserID: 1}}
	got := toggleInputPeer(peers, &tg.InputPeerUser{UserID: 2}, true)
	assert.Equal(t, []int64{1, 2}, inputPeerIDs(got))
}

func TestToggleInputPeer_NoDuplicateOnAdd(t *testing.T) {
	peers := []tg.InputPeerClass{&tg.InputPeerUser{UserID: 1}}
	got := toggleInputPeer(peers, &tg.InputPeerUser{UserID: 1}, true)
	assert.Equal(t, []int64{1}, inputPeerIDs(got))
}

func TestToggleInputPeer_RemovesWhenPresent(t *testing.T) {
	peers := []tg.InputPeerClass{&tg.InputPeerUser{UserID: 1}, &tg.InputPeerChannel{ChannelID: 5}}
	got := toggleInputPeer(peers, &tg.InputPeerChannel{ChannelID: 5}, false)
	assert.Equal(t, []int64{1}, inputPeerIDs(got))
}

func TestRemoveInputPeer(t *testing.T) {
	peers := []tg.InputPeerClass{&tg.InputPeerUser{UserID: 1}, &tg.InputPeerUser{UserID: 2}}
	got := removeInputPeer(peers, 1)
	assert.Equal(t, []int64{2}, inputPeerIDs(got))
}

func inputPeerIDs(peers []tg.InputPeerClass) []int64 {
	out := make([]int64, 0, len(peers))
	for _, p := range peers {
		out = append(out, inputPeerID(p))
	}
	return out
}
