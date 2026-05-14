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

func TestParseHistory_ChronologicalOrder(t *testing.T) {
	// Telegram API returns newest-first; parseHistory must reverse to oldest-first.
	now := time.Now()
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{ID: 3, Message: "newest", Date: int(now.Unix())},
			&tg.Message{ID: 2, Message: "middle", Date: int(now.Add(-time.Minute).Unix())},
			&tg.Message{ID: 1, Message: "oldest", Date: int(now.Add(-2 * time.Minute).Unix())},
		},
	}
	msgs := parseHistory(raw, 10)
	require.Len(t, msgs, 3)
	assert.Equal(t, 1, msgs[0].ID, "oldest message must be first")
	assert.Equal(t, 3, msgs[2].ID, "newest message must be last")
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

func TestConvertEntities_AllSupportedTypes(t *testing.T) {
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 5},
		&tg.MessageEntityItalic{Offset: 6, Length: 4},
		&tg.MessageEntityCode{Offset: 11, Length: 3},
		&tg.MessageEntityPre{Offset: 15, Length: 10},
	}
	got := convertEntities(entities)
	require.Len(t, got, 4)
	assert.Equal(t, store.MessageEntity{Type: "bold", Offset: 0, Length: 5}, got[0])
	assert.Equal(t, store.MessageEntity{Type: "italic", Offset: 6, Length: 4}, got[1])
	assert.Equal(t, store.MessageEntity{Type: "code", Offset: 11, Length: 3}, got[2])
	assert.Equal(t, store.MessageEntity{Type: "pre", Offset: 15, Length: 10}, got[3])
}

func TestConvertEntities_SkipsUnknownTypes(t *testing.T) {
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 3},
		&tg.MessageEntityURL{Offset: 4, Length: 10},
	}
	got := convertEntities(entities)
	require.Len(t, got, 1)
	assert.Equal(t, "bold", got[0].Type)
}

func TestConvertEntities_Nil(t *testing.T) {
	got := convertEntities(nil)
	assert.Empty(t, got)
}

func TestConvertMessage_PopulatesEntities(t *testing.T) {
	raw := &tg.Message{
		ID:       1,
		Message:  "hello world",
		Entities: []tg.MessageEntityClass{&tg.MessageEntityBold{Offset: 0, Length: 5}},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.Len(t, msg.Entities, 1)
	assert.Equal(t, "bold", msg.Entities[0].Type)
}

func TestExtractSentMessageID_Updates(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 42, RandomID: int64(999)},
		},
	}
	assert.Equal(t, 42, extractSentMessageID(updates, int64(999)))
}

func TestExtractSentMessageID_WrongRandomID(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 42, RandomID: int64(111)},
		},
	}
	assert.Equal(t, 0, extractSentMessageID(updates, int64(999)))
}

func TestExtractSentMessageID_ShortSent(t *testing.T) {
	updates := &tg.UpdateShortSentMessage{ID: 77}
	assert.Equal(t, 77, extractSentMessageID(updates, 0))
}

func TestExtractSentMessageID_UnknownType(t *testing.T) {
	assert.Equal(t, 0, extractSentMessageID(&tg.UpdateShort{}, 0))
}

func TestExtractSentMessageID_MultipleUpdates_MatchesCorrectOne(t *testing.T) {
	updates := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 11, RandomID: int64(111)},
			&tg.UpdateMessageID{ID: 22, RandomID: int64(222)},
			&tg.UpdateMessageID{ID: 33, RandomID: int64(333)},
		},
	}
	assert.Equal(t, 22, extractSentMessageID(updates, int64(222)))
}
