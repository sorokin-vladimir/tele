package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/sorokin-vladimir/tele/internal/store"
)

func TestPeer_TypeHelpers(t *testing.T) {
	assert.True(t, store.Peer{Type: store.PeerUser}.IsUser())
	assert.True(t, store.Peer{Type: store.PeerGroup}.IsGroup())
	assert.True(t, store.Peer{Type: store.PeerChannel}.IsChannel())
	assert.False(t, store.Peer{Type: store.PeerUser}.IsChannel())
}

func TestMessage_Fields(t *testing.T) {
	now := time.Now()
	m := store.Message{ID: 1, ChatID: 10, SenderID: 5, Text: "hi", Date: now, IsOut: true}
	assert.Equal(t, 1, m.ID)
	assert.Equal(t, "hi", m.Text)
	assert.True(t, m.IsOut)
}

func TestMessage_HasEntitiesField(t *testing.T) {
	msg := store.Message{
		ID:     1,
		ChatID: 10,
		Text:   "**bold**",
		Entities: []store.MessageEntity{
			{Type: "bold", Offset: 0, Length: 6},
		},
	}
	require.Len(t, msg.Entities, 1)
	assert.Equal(t, "bold", msg.Entities[0].Type)
	assert.Equal(t, 0, msg.Entities[0].Offset)
	assert.Equal(t, 6, msg.Entities[0].Length)
}
