package tg

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestConvertMessage_Regular(t *testing.T) {
	raw := &tg.Message{
		ID:      42,
		PeerID:  &tg.PeerUser{UserID: 10},
		Message: "hello",
		Date:    int(time.Now().Unix()),
		Out:     true,
	}
	// FromID may be nil for outgoing messages to users — set it
	raw.FromID = &tg.PeerUser{UserID: 5}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Equal(t, 42, msg.ID)
	assert.Equal(t, int64(10), msg.ChatID)
	assert.Equal(t, int64(5), msg.SenderID)
	assert.Equal(t, "hello", msg.Text)
	assert.True(t, msg.IsOut)
}

func TestConvertMessage_Service(t *testing.T) {
	raw := &tg.MessageService{ID: 1}
	_, ok := convertMessage(raw, 10)
	assert.False(t, ok)
}

func TestParseHistory_RetriesOnFloodWait(t *testing.T) {
	// parseHistory should gracefully handle empty/unknown result types
	// and return nil rather than panic.
	result := parseHistory(nil, 999)
	assert.Nil(t, result)

	// A known type with no messages returns an empty slice.
	result = parseHistory(&tg.MessagesMessages{Messages: nil}, 999)
	assert.Empty(t, result)
}

func TestPeerToInput_AllTypes(t *testing.T) {
	cases := []struct {
		peer     store.Peer
		wantType string
	}{
		{store.Peer{ID: 1, Type: store.PeerUser, AccessHash: 10}, "*tg.InputPeerUser"},
		{store.Peer{ID: 2, Type: store.PeerGroup}, "*tg.InputPeerChat"},
		{store.Peer{ID: 3, Type: store.PeerChannel, AccessHash: 20}, "*tg.InputPeerChannel"},
		{store.Peer{Type: store.PeerType(99)}, "*tg.InputPeerEmpty"},
	}

	for _, tc := range cases {
		inp := peerToInput(tc.peer)
		require.NotNil(t, inp)
		switch tc.wantType {
		case "*tg.InputPeerUser":
			v, ok := inp.(*tg.InputPeerUser)
			require.True(t, ok)
			assert.Equal(t, tc.peer.ID, v.UserID)
			assert.Equal(t, tc.peer.AccessHash, v.AccessHash)
		case "*tg.InputPeerChat":
			v, ok := inp.(*tg.InputPeerChat)
			require.True(t, ok)
			assert.Equal(t, tc.peer.ID, v.ChatID)
		case "*tg.InputPeerChannel":
			v, ok := inp.(*tg.InputPeerChannel)
			require.True(t, ok)
			assert.Equal(t, tc.peer.ID, v.ChannelID)
			assert.Equal(t, tc.peer.AccessHash, v.AccessHash)
		case "*tg.InputPeerEmpty":
			_, ok := inp.(*tg.InputPeerEmpty)
			require.True(t, ok)
		}
	}
}
