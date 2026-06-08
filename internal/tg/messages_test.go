package tg

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectMessageByID(t *testing.T) {
	msgs := []store.Message{{ID: 1}, {ID: 2}, {ID: 3}}
	got, ok := selectMessageByID(msgs, 2)
	assert.True(t, ok)
	assert.Equal(t, 2, got.ID)

	_, ok = selectMessageByID(msgs, 99)
	assert.False(t, ok)
}

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

func TestConvertMessage_WithPhoto(t *testing.T) {
	raw := &tg.Message{
		ID:   101,
		Out:  false,
		Date: 1700000000,
		Media: &tg.MessageMediaPhoto{
			Photo: &tg.Photo{
				ID:            555,
				AccessHash:    777,
				FileReference: []byte{0xAA, 0xBB},
				DCID:          2,
				Sizes: []tg.PhotoSizeClass{
					&tg.PhotoSize{Type: "s", W: 100, H: 100},
					&tg.PhotoSize{Type: "m", W: 320, H: 240},
				},
			},
		},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.NotNil(t, msg.Photo)
	require.Equal(t, int64(555), msg.Photo.ID)
	require.Equal(t, int64(777), msg.Photo.AccessHash)
	require.Equal(t, []byte{0xAA, 0xBB}, msg.Photo.FileReference)
	require.Equal(t, "m", msg.Photo.ThumbSize)
}

func TestConvertMessage_NoPhoto(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 1700000000, Message: "hello"}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.Nil(t, msg.Photo)
}

func TestConvertMessage_WithReply(t *testing.T) {
	raw := &tg.Message{
		ID:      99,
		Message: "reply text",
		Date:    1700000000,
		ReplyTo: &tg.MessageReplyHeader{ReplyToMsgID: 42},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Equal(t, 42, msg.ReplyToMsgID)
}

func TestConvertMessage_NoReply_ReplyToMsgIDIsZero(t *testing.T) {
	raw := &tg.Message{ID: 1, Message: "hello", Date: 1700000000}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Equal(t, 0, msg.ReplyToMsgID)
}

func TestBuildSendRequest_WithReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10, AccessHash: 20}
	req := buildSendRequest(peer, "hello", 123, 42)
	require.NotNil(t, req.ReplyTo)
	replyTo, ok := req.ReplyTo.(*tg.InputReplyToMessage)
	require.True(t, ok)
	assert.Equal(t, 42, replyTo.ReplyToMsgID)
}

func TestBuildSendRequest_WithoutReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10}
	req := buildSendRequest(peer, "hello", 123, 0)
	assert.Nil(t, req.ReplyTo)
}

func TestParseHistory_ForwardFromUser_SetsForwardName(t *testing.T) {
	now := time.Now()
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromID(&tg.PeerUser{UserID: 77})
	rawMsg := &tg.Message{
		ID:      1,
		PeerID:  &tg.PeerUser{UserID: 42},
		Message: "forwarded text",
		Date:    int(now.Unix()),
	}
	rawMsg.SetFwdFrom(fwd)
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{rawMsg},
		Users: []tg.UserClass{
			&tg.User{ID: 42, FirstName: "Alice"},
			&tg.User{ID: 77, FirstName: "Bob"},
		},
	}
	msgs := parseHistory(raw, 42)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].Forward)
	assert.Equal(t, "Bob", msgs[0].Forward.From)
}

func TestParseHistory_ForwardFromChannel_SetsChannelTitle(t *testing.T) {
	now := time.Now()
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromID(&tg.PeerChannel{ChannelID: 500})
	rawMsg := &tg.Message{
		ID:      1,
		PeerID:  &tg.PeerUser{UserID: 42},
		Message: "forwarded post",
		Date:    int(now.Unix()),
	}
	rawMsg.SetFwdFrom(fwd)
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{rawMsg},
		Users:    []tg.UserClass{&tg.User{ID: 42, FirstName: "Alice"}},
		Chats:    []tg.ChatClass{&tg.Channel{ID: 500, Title: "Tech News"}},
	}
	msgs := parseHistory(raw, 42)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].Forward)
	assert.Equal(t, "Tech News", msgs[0].Forward.From)
}

func TestParseHistory_ForwardHiddenSender_UsesFromName(t *testing.T) {
	now := time.Now()
	fwd := tg.MessageFwdHeader{}
	fwd.SetFromName("Anon Doe")
	rawMsg := &tg.Message{
		ID:      1,
		PeerID:  &tg.PeerUser{UserID: 42},
		Message: "forwarded text",
		Date:    int(now.Unix()),
	}
	rawMsg.SetFwdFrom(fwd)
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{rawMsg},
		Users:    []tg.UserClass{&tg.User{ID: 42, FirstName: "Alice"}},
	}
	msgs := parseHistory(raw, 42)
	require.Len(t, msgs, 1)
	require.NotNil(t, msgs[0].Forward)
	assert.Equal(t, "Anon Doe", msgs[0].Forward.From)
}

func TestParseHistory_NoForward_ForwardIsNil(t *testing.T) {
	now := time.Now()
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{
				ID:      1,
				PeerID:  &tg.PeerUser{UserID: 42},
				Message: "plain text",
				Date:    int(now.Unix()),
			},
		},
		Users: []tg.UserClass{&tg.User{ID: 42, FirstName: "Alice"}},
	}
	msgs := parseHistory(raw, 42)
	require.Len(t, msgs, 1)
	assert.Nil(t, msgs[0].Forward)
}

func TestParseHistory_ChannelPost_SenderNameIsChannelTitle(t *testing.T) {
	now := time.Now()
	raw := &tg.MessagesChannelMessages{
		Messages: []tg.MessageClass{
			&tg.Message{
				ID:      1,
				PeerID:  &tg.PeerChannel{ChannelID: 500},
				Message: "channel post",
				Date:    int(now.Unix()),
				// FromID is nil — anonymous channel post
			},
		},
		Users: []tg.UserClass{},
		Chats: []tg.ChatClass{
			&tg.Channel{ID: 500, Title: "Tech News"},
		},
	}
	msgs := parseHistory(raw, 500)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Tech News", msgs[0].SenderName)
}

func TestParseHistory_ChannelPost_OutgoingNotOverridden(t *testing.T) {
	now := time.Now()
	raw := &tg.MessagesChannelMessages{
		Messages: []tg.MessageClass{
			&tg.Message{
				ID:      1,
				PeerID:  &tg.PeerChannel{ChannelID: 500},
				Message: "admin post",
				Date:    int(now.Unix()),
				Out:     true, // outgoing — do not set SenderName from channel
			},
		},
		Chats: []tg.ChatClass{
			&tg.Channel{ID: 500, Title: "Tech News"},
		},
	}
	msgs := parseHistory(raw, 500)
	require.Len(t, msgs, 1)
	assert.Equal(t, "", msgs[0].SenderName, "outgoing message must not get channel title as SenderName")
}

func TestParseHistory_PrivateChat_IncomingNilFromID_SenderNameIsPeerName(t *testing.T) {
	// In private chats, incoming messages may have FromID==nil.
	// The peer user is in rawUsers; chatID == peer's user ID.
	now := time.Now()
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{
				ID:      1,
				PeerID:  &tg.PeerUser{UserID: 42},
				Message: "hey",
				Date:    int(now.Unix()),
				// FromID is nil — incoming private message without explicit sender
			},
		},
		Users: []tg.UserClass{
			&tg.User{ID: 42, FirstName: "Alice"},
		},
	}
	msgs := parseHistory(raw, 42)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Alice", msgs[0].SenderName)
}

func TestParseHistory_UserMessage_SenderNameFromUsers(t *testing.T) {
	now := time.Now()
	raw := &tg.MessagesMessages{
		Messages: []tg.MessageClass{
			&tg.Message{
				ID:      1,
				PeerID:  &tg.PeerUser{UserID: 99},
				FromID:  &tg.PeerUser{UserID: 99},
				Message: "hello",
				Date:    int(now.Unix()),
			},
		},
		Users: []tg.UserClass{
			&tg.User{ID: 99, FirstName: "Alice"},
		},
	}
	msgs := parseHistory(raw, 99)
	require.Len(t, msgs, 1)
	assert.Equal(t, "Alice", msgs[0].SenderName)
}

func TestClassifyMedia_Photo(t *testing.T) {
	m := classifyMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 1}})
	require.NotNil(t, m)
	assert.Equal(t, store.MediaPhoto, m.Kind)
}

func TestClassifyMedia_Nil(t *testing.T) {
	assert.Nil(t, classifyMedia(nil))
}

func TestClassifyMedia_Location(t *testing.T) {
	for _, media := range []tg.MessageMediaClass{
		&tg.MessageMediaGeo{},
		&tg.MessageMediaGeoLive{},
		&tg.MessageMediaVenue{},
	} {
		m := classifyMedia(media)
		require.NotNil(t, m)
		assert.Equal(t, store.MediaLocation, m.Kind)
	}
}

func TestClassifyMedia_Other(t *testing.T) {
	m := classifyMedia(&tg.MessageMediaContact{})
	require.NotNil(t, m)
	assert.Equal(t, store.MediaOther, m.Kind)
}

func docMedia(attrs ...tg.DocumentAttributeClass) *tg.MessageMediaDocument {
	return &tg.MessageMediaDocument{
		Document: &tg.Document{ID: 1, Size: 123, Attributes: attrs},
	}
}

func TestClassifyMedia_Document(t *testing.T) {
	tests := []struct {
		name  string
		attrs []tg.DocumentAttributeClass
		want  store.MediaKind
	}{
		{"sticker", []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{Alt: "🐱"}}, store.MediaSticker},
		{"sticker beats animated", []tg.DocumentAttributeClass{&tg.DocumentAttributeSticker{}, &tg.DocumentAttributeAnimated{}}, store.MediaSticker},
		{"gif", []tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}}, store.MediaGIF},
		{"animated beats video (gif)", []tg.DocumentAttributeClass{&tg.DocumentAttributeAnimated{}, &tg.DocumentAttributeVideo{}}, store.MediaGIF},
		{"video note", []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{RoundMessage: true}}, store.MediaVideoNote},
		{"video", []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}}, store.MediaVideo},
		{"voice", []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{Voice: true}}, store.MediaVoice},
		{"audio", []tg.DocumentAttributeClass{&tg.DocumentAttributeAudio{}}, store.MediaAudio},
		{"file", []tg.DocumentAttributeClass{&tg.DocumentAttributeFilename{FileName: "a.pdf"}}, store.MediaFile},
		{"empty file", nil, store.MediaFile},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := classifyMedia(docMedia(tc.attrs...))
			require.NotNil(t, m)
			assert.Equal(t, tc.want, m.Kind)
		})
	}
}

func TestClassifyMedia_StickerEmoji(t *testing.T) {
	m := classifyMedia(docMedia(&tg.DocumentAttributeSticker{Alt: "🐱"}))
	require.NotNil(t, m)
	assert.Equal(t, "🐱", m.Emoji)
}

func TestClassifyMedia_VoiceWaveformDuration(t *testing.T) {
	wave := []byte{0x01, 0x02, 0x03}
	m := classifyMedia(docMedia(&tg.DocumentAttributeAudio{
		Voice: true, Duration: 15, Waveform: wave,
	}))
	require.NotNil(t, m)
	assert.Equal(t, store.MediaVoice, m.Kind)
	assert.Equal(t, 15, m.Duration)
	assert.Equal(t, wave, m.Waveform)
}

func TestClassifyMedia_AudioTitlePerformerDuration(t *testing.T) {
	m := classifyMedia(docMedia(&tg.DocumentAttributeAudio{
		Duration: 200, Title: "Song", Performer: "Artist",
	}))
	require.NotNil(t, m)
	assert.Equal(t, store.MediaAudio, m.Kind)
	assert.Equal(t, 200, m.Duration)
	assert.Equal(t, "Song", m.Title)
	assert.Equal(t, "Artist", m.Performer)
}

func TestClassifyMedia_VideoDuration(t *testing.T) {
	m := classifyMedia(docMedia(&tg.DocumentAttributeVideo{Duration: 42.7}))
	require.NotNil(t, m)
	assert.Equal(t, store.MediaVideo, m.Kind)
	assert.Equal(t, 42, m.Duration)
}

func TestClassifyMedia_VideoNoteDuration(t *testing.T) {
	m := classifyMedia(docMedia(&tg.DocumentAttributeVideo{RoundMessage: true, Duration: 8.2}))
	require.NotNil(t, m)
	assert.Equal(t, store.MediaVideoNote, m.Kind)
	assert.Equal(t, 8, m.Duration)
}

func TestConvertMessage_SetsMediaForPhoto(t *testing.T) {
	raw := &tg.Message{
		ID: 1, Date: 1700000000,
		Media: &tg.MessageMediaPhoto{Photo: &tg.Photo{
			ID: 5, Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "m", W: 320, H: 240}},
		}},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.NotNil(t, msg.Media)
	assert.Equal(t, store.MediaPhoto, msg.Media.Kind)
	require.NotNil(t, msg.Photo) // photo ref still populated
}

func TestConvertMessage_NoMediaForText(t *testing.T) {
	raw := &tg.Message{ID: 1, Date: 1700000000, Message: "hi"}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Nil(t, msg.Media)
}

func TestConvertMessage_SetsDocumentRefAndFileMeta(t *testing.T) {
	raw := &tg.Message{
		ID: 7, Date: 1700000000,
		Media: &tg.MessageMediaDocument{Document: &tg.Document{
			ID: 99, AccessHash: 1234, FileReference: []byte{0xDE, 0xAD},
			DCID: 2, MimeType: "audio/ogg", Size: 4096,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeFilename{FileName: "voice.ogg"},
				&tg.DocumentAttributeAudio{Voice: true, Duration: 5},
			},
		}},
	}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.NotNil(t, msg.Document)
	assert.Equal(t, int64(99), msg.Document.ID)
	assert.Equal(t, int64(1234), msg.Document.AccessHash)
	assert.Equal(t, []byte{0xDE, 0xAD}, msg.Document.FileReference)
	assert.Equal(t, 2, msg.Document.DCID)
	assert.Equal(t, "audio/ogg", msg.Document.MimeType)
	assert.Equal(t, int64(4096), msg.Document.Size)
	assert.Equal(t, "voice.ogg", msg.Document.FileName)
	require.NotNil(t, msg.Media)
	assert.Equal(t, int64(4096), msg.Media.Size)
	assert.Equal(t, "voice.ogg", msg.Media.FileName)
}

func TestConvertMessage_DocumentThumbSize(t *testing.T) {
	raw := &tg.Message{
		ID: 8, Date: 1700000000,
		Media: &tg.MessageMediaDocument{Document: &tg.Document{
			ID:         1,
			Thumbs:     []tg.PhotoSizeClass{&tg.PhotoSize{Type: "m", W: 320, H: 240}},
			Attributes: []tg.DocumentAttributeClass{&tg.DocumentAttributeVideo{}},
		}},
	}
	msg, ok := convertMessage(raw, 1)
	require.True(t, ok)
	require.NotNil(t, msg.Document)
	assert.Equal(t, "m", msg.Document.ThumbSize)
}
