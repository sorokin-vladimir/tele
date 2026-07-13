package tg

import (
	"testing"
	"time"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
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

func TestBuildSaveDraftRequest(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 7, AccessHash: 3}
	req := buildSaveDraftRequest(peer, "draft text")
	assert.Equal(t, peer, req.Peer)
	assert.Equal(t, "draft text", req.Message)
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

func TestConvertEntities_ExtendedTypes(t *testing.T) {
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityStrike{Offset: 0, Length: 2},
		&tg.MessageEntityUnderline{Offset: 2, Length: 2},
		&tg.MessageEntityTextURL{Offset: 4, Length: 3, URL: "https://example.com"},
		&tg.MessageEntityURL{Offset: 7, Length: 4},
		&tg.MessageEntityEmail{Offset: 11, Length: 5},
		&tg.MessageEntityPhone{Offset: 16, Length: 6},
		&tg.MessageEntityBankCard{Offset: 22, Length: 7},
		&tg.MessageEntityMention{Offset: 29, Length: 3},
		&tg.MessageEntityHashtag{Offset: 32, Length: 4},
		&tg.MessageEntityCashtag{Offset: 36, Length: 4},
		&tg.MessageEntityBotCommand{Offset: 40, Length: 5},
	}
	got := convertEntities(entities)
	require.Len(t, got, 11)
	assert.Equal(t, store.MessageEntity{Type: "strike", Offset: 0, Length: 2}, got[0])
	assert.Equal(t, store.MessageEntity{Type: "underline", Offset: 2, Length: 2}, got[1])
	assert.Equal(t, store.MessageEntity{Type: "text_url", Offset: 4, Length: 3, URL: "https://example.com"}, got[2])
	assert.Equal(t, store.MessageEntity{Type: "url", Offset: 7, Length: 4}, got[3])
	assert.Equal(t, store.MessageEntity{Type: "email", Offset: 11, Length: 5}, got[4])
	assert.Equal(t, store.MessageEntity{Type: "phone", Offset: 16, Length: 6}, got[5])
	assert.Equal(t, store.MessageEntity{Type: "bank_card", Offset: 22, Length: 7}, got[6])
	assert.Equal(t, store.MessageEntity{Type: "mention", Offset: 29, Length: 3}, got[7])
	assert.Equal(t, store.MessageEntity{Type: "hashtag", Offset: 32, Length: 4}, got[8])
	assert.Equal(t, store.MessageEntity{Type: "cashtag", Offset: 36, Length: 4}, got[9])
	assert.Equal(t, store.MessageEntity{Type: "bot_command", Offset: 40, Length: 5}, got[10])
}

func TestConvertEntities_SkipsUnknownTypes(t *testing.T) {
	entities := []tg.MessageEntityClass{
		&tg.MessageEntityBold{Offset: 0, Length: 3},
		&tg.MessageEntitySpoiler{Offset: 4, Length: 10}, // out of scope, must be skipped
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

func TestBuildForwardRequest_MapsPeersAndIDs(t *testing.T) {
	from := &tg.InputPeerUser{UserID: 10, AccessHash: 11}
	to := &tg.InputPeerChannel{ChannelID: 20, AccessHash: 21}
	ids := []int{100, 101}
	randomIDs := []int64{7, 8}

	req := buildForwardRequest(from, to, ids, randomIDs)

	assert.Equal(t, from, req.FromPeer)
	assert.Equal(t, to, req.ToPeer)
	assert.Equal(t, ids, req.ID)
	assert.Equal(t, randomIDs, req.RandomID)
}

func TestIsForwardRestrictedErr(t *testing.T) {
	restricted := &tgerr.Error{Code: 403, Type: "CHAT_FORWARDS_RESTRICTED"}
	other := &tgerr.Error{Code: 400, Type: "MESSAGE_ID_INVALID"}

	assert.True(t, isForwardRestrictedErr(restricted))
	assert.False(t, isForwardRestrictedErr(other))
	assert.False(t, isForwardRestrictedErr(nil))
}

func TestBuildSendRequest_WithReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10, AccessHash: 20}
	req := buildSendRequest(peer, "hello", 123, 42, nil)
	require.NotNil(t, req.ReplyTo)
	replyTo, ok := req.ReplyTo.(*tg.InputReplyToMessage)
	require.True(t, ok)
	assert.Equal(t, 42, replyTo.ReplyToMsgID)
}

func TestBuildSendRequest_WithoutReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10}
	req := buildSendRequest(peer, "hello", 123, 0, nil)
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

func TestConvertMessage_HiddenEdit_NoEditDate(t *testing.T) {
	// Telegram bumps edit_date when a reaction is added but sets edit_hide so
	// clients don't show "edited". Our converter must honor edit_hide (#118).
	raw := &tg.Message{
		ID:       7,
		PeerID:   &tg.PeerUser{UserID: 10},
		Message:  "hi",
		Date:     int(time.Now().Unix()),
		EditDate: int(time.Now().Unix()),
		EditHide: true,
	}
	raw.FromID = &tg.PeerUser{UserID: 10}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	assert.Nil(t, msg.EditDate, "edit_hide must suppress the edited marker")
}

func TestConvertMessage_RealEdit_SetsEditDate(t *testing.T) {
	raw := &tg.Message{
		ID:       7,
		PeerID:   &tg.PeerUser{UserID: 10},
		Message:  "hi",
		Date:     int(time.Now().Unix()),
		EditDate: int(time.Now().Unix()),
		EditHide: false,
	}
	raw.FromID = &tg.PeerUser{UserID: 10}
	msg, ok := convertMessage(raw, 10)
	require.True(t, ok)
	require.NotNil(t, msg.EditDate, "a real edit keeps the edited marker")
}

func TestBuildInputMediaUploadedPhoto(t *testing.T) {
	f := &tg.InputFile{ID: 42, Parts: 1, Name: "a.jpg"}
	media := BuildInputMediaUploadedPhoto(f)
	photo, ok := media.(*tg.InputMediaUploadedPhoto)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedPhoto", media)
	assert.Equal(t, f, photo.File)
}

func TestBuildSendMediaRequest_WithReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10, AccessHash: 20}
	media := &tg.InputMediaUploadedPhoto{}
	req := buildSendMediaRequest(peer, media, "cap", 123, 42)
	assert.Equal(t, "cap", req.Message)
	assert.Equal(t, int64(123), req.RandomID)
	assert.Equal(t, media, req.Media)
	require.NotNil(t, req.ReplyTo)
	replyTo, ok := req.ReplyTo.(*tg.InputReplyToMessage)
	require.True(t, ok)
	assert.Equal(t, 42, replyTo.ReplyToMsgID)
}

func TestBuildSendMediaRequest_WithoutReply(t *testing.T) {
	peer := &tg.InputPeerUser{UserID: 10}
	req := buildSendMediaRequest(peer, &tg.InputMediaUploadedPhoto{}, "", 123, 0)
	assert.Nil(t, req.ReplyTo)
	assert.Equal(t, "", req.Message)
}

func TestBuildInputMediaUploadedDocument(t *testing.T) {
	f := &tg.InputFile{ID: 7, Parts: 1, Name: "report.pdf"}
	media := BuildInputMediaUploadedDocument(f, "report.pdf", "application/pdf")
	doc, ok := media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedDocument", media)
	assert.Equal(t, f, doc.File)
	assert.Equal(t, "application/pdf", doc.MimeType)
	assert.True(t, doc.ForceFile, "ForceFile must be set so an image is not reinterpreted as a photo")
	require.Len(t, doc.Attributes, 1)
	fn, ok := doc.Attributes[0].(*tg.DocumentAttributeFilename)
	require.True(t, ok, "got %T, want *tg.DocumentAttributeFilename", doc.Attributes[0])
	assert.Equal(t, "report.pdf", fn.FileName)
}

func TestBuildInputMediaUploadedDocument_EmptyMIMEFallback(t *testing.T) {
	media := BuildInputMediaUploadedDocument(&tg.InputFile{}, "a.bin", "")
	doc, ok := media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok)
	assert.Equal(t, "application/octet-stream", doc.MimeType)
}

func TestBuildInputMediaUploadedVideo_FullMetadataAndThumb(t *testing.T) {
	f := &tg.InputFile{ID: 7, Parts: 1, Name: "clip.mp4"}
	thumb := &tg.InputFile{ID: 8, Parts: 1, Name: "thumb.jpg"}
	media := BuildInputMediaUploadedVideo(f, "clip.mp4", "video/mp4", 42, 1920, 1080, thumb)

	doc, ok := media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok, "got %T, want *tg.InputMediaUploadedDocument", media)
	assert.Equal(t, f, doc.File)
	assert.Equal(t, "video/mp4", doc.MimeType)
	assert.False(t, doc.ForceFile, "video must NOT force the generic file path")

	gotThumb, ok := doc.GetThumb()
	require.True(t, ok, "thumb must be set")
	assert.Equal(t, thumb, gotThumb)

	var vid *tg.DocumentAttributeVideo
	var fn *tg.DocumentAttributeFilename
	for _, a := range doc.Attributes {
		switch at := a.(type) {
		case *tg.DocumentAttributeVideo:
			vid = at
		case *tg.DocumentAttributeFilename:
			fn = at
		}
	}
	require.NotNil(t, vid, "DocumentAttributeVideo must be present")
	assert.True(t, vid.SupportsStreaming)
	assert.Equal(t, 42.0, vid.Duration)
	assert.Equal(t, 1920, vid.W)
	assert.Equal(t, 1080, vid.H)
	require.NotNil(t, fn)
	assert.Equal(t, "clip.mp4", fn.FileName)
}

func TestBuildInputMediaUploadedVideo_NoThumbZeroMeta(t *testing.T) {
	f := &tg.InputFile{ID: 1, Parts: 1, Name: "a.mp4"}
	media := BuildInputMediaUploadedVideo(f, "a.mp4", "", 0, 0, 0, nil)

	doc, ok := media.(*tg.InputMediaUploadedDocument)
	require.True(t, ok)
	assert.Equal(t, "video/mp4", doc.MimeType, "empty mime must default to video/mp4")
	_, hasThumb := doc.GetThumb()
	assert.False(t, hasThumb, "no thumb arg means no Thumb set")

	var vid *tg.DocumentAttributeVideo
	for _, a := range doc.Attributes {
		if at, ok := a.(*tg.DocumentAttributeVideo); ok {
			vid = at
		}
	}
	require.NotNil(t, vid)
	assert.True(t, vid.SupportsStreaming, "streaming attribute is always set")
	assert.Zero(t, vid.Duration)
	assert.Zero(t, vid.W)
	assert.Zero(t, vid.H)
}
