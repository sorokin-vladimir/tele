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

func TestMessage_PhotoField(t *testing.T) {
	m := store.Message{
		Photo: &store.PhotoRef{
			ID:            42,
			AccessHash:    99,
			FileReference: []byte{1, 2, 3},
			DCID:          2,
			ThumbSize:     "m",
		},
	}
	require.NotNil(t, m.Photo)
	require.Equal(t, int64(42), m.Photo.ID)
	require.Equal(t, "m", m.Photo.ThumbSize)
}

func TestChat_NewFields(t *testing.T) {
	c := store.Chat{IsContact: true, IsBot: false, IsMuted: true}
	assert.True(t, c.IsContact)
	assert.False(t, c.IsBot)
	assert.True(t, c.IsMuted)
}

func TestTypingAction_Label_KnownValues(t *testing.T) {
	assert.Equal(t, "typing", store.TypingActionTyping.Label())
	assert.Equal(t, "recording audio", store.TypingActionRecordAudio.Label())
	assert.Equal(t, "sending audio", store.TypingActionUploadAudio.Label())
	assert.Equal(t, "recording video", store.TypingActionRecordVideo.Label())
	assert.Equal(t, "sending video", store.TypingActionUploadVideo.Label())
	assert.Equal(t, "sending a photo", store.TypingActionUploadPhoto.Label())
	assert.Equal(t, "sending a file", store.TypingActionUploadDocument.Label())
	assert.Equal(t, "choosing a sticker", store.TypingActionChooseSticker.Label())
	assert.Equal(t, "recording a video message", store.TypingActionRecordRound.Label())
}

func TestTypingAction_Label_EmptyForUnknownAndCancel(t *testing.T) {
	assert.Equal(t, "", store.TypingActionUnknown.Label())
	assert.Equal(t, "", store.TypingActionCancel.Label())
}

func TestEventTyping_TypingActionField(t *testing.T) {
	evt := store.Event{Kind: store.EventTyping, ChatID: 42, TypingAction: store.TypingActionTyping}
	assert.Equal(t, store.EventTyping, evt.Kind)
	assert.Equal(t, store.TypingActionTyping, evt.TypingAction)
	assert.Equal(t, int64(42), evt.ChatID)
}
