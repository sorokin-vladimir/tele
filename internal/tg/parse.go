package tg

import (
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/store"
)

func buildNameMap(users []tg.UserClass) map[int64]string {
	m := make(map[int64]string, len(users))
	for _, u := range users {
		user, ok := u.(*tg.User)
		if !ok {
			continue
		}
		name := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if name == "" {
			name = fmt.Sprintf("User %d", user.ID)
		}
		m[user.ID] = name
	}
	return m
}

func channelTitle(ch *tg.Channel) string {
	if ch.Title != "" {
		return ch.Title
	}
	return fmt.Sprintf("Channel %d", ch.ID)
}

func groupTitle(chat *tg.Chat) string {
	if chat.Title != "" {
		return chat.Title
	}
	return fmt.Sprintf("Chat %d", chat.ID)
}

func buildChatNameMap(chats []tg.ChatClass) map[int64]string {
	m := make(map[int64]string, len(chats))
	for _, c := range chats {
		switch v := c.(type) {
		case *tg.Channel:
			m[v.ID] = channelTitle(v)
		case *tg.Chat:
			m[v.ID] = groupTitle(v)
		}
	}
	return m
}

// forwardInfo builds forward display info from a message's forward header.
// resolve maps an origin peer id to a display name; it may return "" when the
// peer is not present in the entity set. Resolution order: origin peer name,
// then the saved FromName (hidden senders), then the channel post author.
func forwardInfo(fwd tg.MessageFwdHeader, resolve func(int64) string) *store.ForwardInfo {
	from := ""
	if id, ok := fwd.GetFromID(); ok {
		from = resolve(peerIDFromPeer(id))
	}
	if from == "" {
		if name, ok := fwd.GetFromName(); ok {
			from = name
		}
	}
	if from == "" {
		if author, ok := fwd.GetPostAuthor(); ok {
			from = author
		}
	}
	return &store.ForwardInfo{From: from}
}

// selectMessageByID returns the message with the given id from a parsed slice.
func selectMessageByID(msgs []store.Message, id int) (store.Message, bool) {
	for _, m := range msgs {
		if m.ID == id {
			return m, true
		}
	}
	return store.Message{}, false
}

func parseHistory(result tg.MessagesMessagesClass, chatID int64) []store.Message {
	var rawMsgs []tg.MessageClass
	var rawUsers []tg.UserClass
	var rawChats []tg.ChatClass

	switch v := result.(type) {
	case *tg.MessagesMessages:
		rawMsgs, rawUsers, rawChats = v.Messages, v.Users, v.Chats
	case *tg.MessagesMessagesSlice:
		rawMsgs, rawUsers, rawChats = v.Messages, v.Users, v.Chats
	case *tg.MessagesChannelMessages:
		rawMsgs, rawUsers, rawChats = v.Messages, v.Users, v.Chats
	default:
		return nil
	}

	nameMap := buildNameMap(rawUsers)
	chatNameMap := buildChatNameMap(rawChats)

	out := make([]store.Message, 0, len(rawMsgs))
	for _, raw := range rawMsgs {
		if msg, ok := convertMessage(raw, chatID); ok {
			msg.SenderName = nameMap[msg.SenderID]
			if msg.SenderName == "" && msg.SenderID == 0 && !msg.IsOut {
				// nil FromID: sender is the chat peer (private chat → user)
				// or the chat entity itself (channel/group anonymous post)
				if name := nameMap[chatID]; name != "" {
					msg.SenderName = name
				} else {
					msg.SenderName = chatNameMap[chatID]
				}
			}
			if m, ok := raw.(*tg.Message); ok {
				if fwd, ok := m.GetFwdFrom(); ok {
					msg.Forward = forwardInfo(fwd, func(id int64) string {
						if name := nameMap[id]; name != "" {
							return name
						}
						return chatNameMap[id]
					})
				}
			}
			out = append(out, msg)
		}
	}
	// Telegram API returns newest-first; reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// largestPhotoSize returns the PhotoSize type with the greatest pixel area, or
// "" when none is a concrete PhotoSize.
func largestPhotoSize(sizes []tg.PhotoSizeClass) string {
	best := ""
	bestArea := 0
	for _, s := range sizes {
		if ps, ok := s.(*tg.PhotoSize); ok {
			if ps.W*ps.H > bestArea {
				bestArea = ps.W * ps.H
				best = ps.Type
			}
		}
	}
	return best
}

// pickFullThumbSize returns the largest available PhotoSize type for full-quality download.
func pickFullThumbSize(sizes []tg.PhotoSizeClass) string {
	return largestPhotoSize(sizes)
}

// pickThumbSize returns the best thumb type string for inline display.
// Prefers "m" (320px), then largest available PhotoSize by area.
func pickThumbSize(sizes []tg.PhotoSizeClass) string {
	for _, s := range sizes {
		if ps, ok := s.(*tg.PhotoSize); ok && ps.Type == "m" {
			return "m"
		}
	}
	return largestPhotoSize(sizes)
}

// classifyMedia maps a Telegram media object to a display-level MediaRef.
// Returns nil when there is no media to show.
func classifyMedia(media tg.MessageMediaClass) *store.MediaRef {
	switch m := media.(type) {
	case *tg.MessageMediaPhoto:
		return &store.MediaRef{Kind: store.MediaPhoto}
	case *tg.MessageMediaDocument:
		return classifyDocument(m)
	case *tg.MessageMediaGeo, *tg.MessageMediaGeoLive, *tg.MessageMediaVenue:
		return &store.MediaRef{Kind: store.MediaLocation}
	case nil:
		return nil
	default:
		return &store.MediaRef{Kind: store.MediaOther}
	}
}

// classifyDocument inspects document attributes by strict priority to decide
// the media kind. A document can carry several attributes at once.
func classifyDocument(m *tg.MessageMediaDocument) *store.MediaRef {
	doc, ok := m.Document.(*tg.Document)
	if !ok {
		return &store.MediaRef{Kind: store.MediaOther}
	}
	var (
		sticker  *tg.DocumentAttributeSticker
		video    *tg.DocumentAttributeVideo
		audio    *tg.DocumentAttributeAudio
		animated bool
	)
	for _, a := range doc.Attributes {
		switch at := a.(type) {
		case *tg.DocumentAttributeSticker:
			sticker = at
		case *tg.DocumentAttributeVideo:
			video = at
		case *tg.DocumentAttributeAudio:
			audio = at
		case *tg.DocumentAttributeAnimated:
			animated = true
		}
	}
	switch {
	case sticker != nil:
		return &store.MediaRef{Kind: store.MediaSticker, Emoji: sticker.Alt}
	case animated:
		return &store.MediaRef{Kind: store.MediaGIF}
	case video != nil && video.RoundMessage:
		return &store.MediaRef{Kind: store.MediaVideoNote, Duration: int(video.Duration)}
	case video != nil:
		return &store.MediaRef{Kind: store.MediaVideo, Duration: int(video.Duration)}
	case audio != nil && audio.Voice:
		return &store.MediaRef{
			Kind:     store.MediaVoice,
			Duration: audio.Duration,
			Waveform: audio.Waveform,
		}
	case audio != nil:
		return &store.MediaRef{
			Kind:      store.MediaAudio,
			Duration:  audio.Duration,
			Title:     audio.Title,
			Performer: audio.Performer,
		}
	default:
		return &store.MediaRef{Kind: store.MediaFile}
	}
}

// buildDocumentRef builds a download-capable reference from a Telegram document,
// picking the best thumbnail PhotoSize and the filename attribute when present.
func buildDocumentRef(doc *tg.Document) *store.DocumentRef {
	ref := &store.DocumentRef{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		DCID:          doc.DCID,
		MimeType:      doc.MimeType,
		Size:          doc.Size,
		ThumbSize:     pickThumbSize(doc.Thumbs),
	}
	for _, a := range doc.Attributes {
		if fn, ok := a.(*tg.DocumentAttributeFilename); ok {
			ref.FileName = fn.FileName
			break
		}
	}
	return ref
}

func convertMessage(raw tg.MessageClass, chatID int64) (store.Message, bool) {
	msg, ok := raw.(*tg.Message)
	if !ok {
		return store.Message{}, false
	}
	senderID := int64(0)
	if from, ok := msg.FromID.(*tg.PeerUser); ok {
		senderID = from.UserID
	}
	out := store.Message{
		ID:       msg.ID,
		ChatID:   chatID,
		SenderID: senderID,
		Text:     msg.Message,
		Date:     time.Unix(int64(msg.Date), 0),
		IsOut:    msg.Out,
		Entities: convertEntities(msg.Entities),
	}
	out.Media = classifyMedia(msg.Media)
	if hdr, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok {
		out.ReplyToMsgID = hdr.ReplyToMsgID
	}
	// EditHide is set when edit_date changes for a non-content reason (e.g. a
	// reaction bump): Telegram tells clients to hide the "edited" label. Honor
	// it so reactions don't mark the message as edited (issue #118).
	if msg.EditDate != 0 && !msg.EditHide {
		t := time.Unix(int64(msg.EditDate), 0)
		out.EditDate = &t
	}
	if msg.Reactions.Results != nil {
		out.Reactions = convertReactions(msg.Reactions)
		out.HasUnreadReactions = reactionsHaveUnread(msg.Reactions)
	}
	if media, ok := msg.Media.(*tg.MessageMediaPhoto); ok {
		if photo, ok := media.Photo.(*tg.Photo); ok && len(photo.Sizes) > 0 {
			thumb := pickThumbSize(photo.Sizes)
			if thumb != "" {
				out.Photo = &store.PhotoRef{
					ID:            photo.ID,
					AccessHash:    photo.AccessHash,
					FileReference: photo.FileReference,
					DCID:          photo.DCID,
					ThumbSize:     thumb,
					FullThumbSize: pickFullThumbSize(photo.Sizes),
				}
			}
		}
	}
	if media, ok := msg.Media.(*tg.MessageMediaDocument); ok {
		if doc, ok := media.Document.(*tg.Document); ok {
			out.Document = buildDocumentRef(doc)
			if out.Media != nil {
				out.Media.Size = doc.Size
				if out.Document.FileName != "" {
					out.Media.FileName = out.Document.FileName
				}
			}
		}
	}
	return out, true
}

// reactionsHaveUnread reports whether any recent reaction on the message is
// flagged unread (a not-yet-viewed reaction on one of our messages).
func reactionsHaveUnread(mr tg.MessageReactions) bool {
	for _, r := range mr.RecentReactions {
		if r.Unread {
			return true
		}
	}
	return false
}

func convertReactions(mr tg.MessageReactions) []store.Reaction {
	out := make([]store.Reaction, 0, len(mr.Results))
	for _, rc := range mr.Results {
		emoji, ok := rc.Reaction.(*tg.ReactionEmoji)
		if !ok {
			continue
		}
		_, isChosen := rc.GetChosenOrder()
		out = append(out, store.Reaction{
			Emoji:    emoji.Emoticon,
			Count:    rc.Count,
			IsChosen: isChosen,
		})
	}
	return out
}

func convertEntities(entities []tg.MessageEntityClass) []store.MessageEntity {
	if len(entities) == 0 {
		return nil
	}
	out := make([]store.MessageEntity, 0, len(entities))
	for _, e := range entities {
		switch v := e.(type) {
		case *tg.MessageEntityBold:
			out = append(out, store.MessageEntity{Type: "bold", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityItalic:
			out = append(out, store.MessageEntity{Type: "italic", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityCode:
			out = append(out, store.MessageEntity{Type: "code", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityPre:
			out = append(out, store.MessageEntity{Type: "pre", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityStrike:
			out = append(out, store.MessageEntity{Type: "strike", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityUnderline:
			out = append(out, store.MessageEntity{Type: "underline", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityTextURL:
			out = append(out, store.MessageEntity{Type: "text_url", Offset: v.Offset, Length: v.Length, URL: v.URL})
		case *tg.MessageEntityURL:
			out = append(out, store.MessageEntity{Type: "url", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityEmail:
			out = append(out, store.MessageEntity{Type: "email", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityPhone:
			out = append(out, store.MessageEntity{Type: "phone", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityBankCard:
			out = append(out, store.MessageEntity{Type: "bank_card", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityMention:
			out = append(out, store.MessageEntity{Type: "mention", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityHashtag:
			out = append(out, store.MessageEntity{Type: "hashtag", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityCashtag:
			out = append(out, store.MessageEntity{Type: "cashtag", Offset: v.Offset, Length: v.Length})
		case *tg.MessageEntityBotCommand:
			out = append(out, store.MessageEntity{Type: "bot_command", Offset: v.Offset, Length: v.Length})
		}
	}
	return out
}
