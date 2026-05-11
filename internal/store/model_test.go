package store_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
