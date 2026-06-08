package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/store"
)

// RefreshMessage re-fetches one message and returns it with fresh media refs.
func (c *GotdClient) RefreshMessage(ctx context.Context, peer store.Peer, msgID int) (store.Message, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return store.Message{}, fmt.Errorf("not connected")
	}

	ids := []tg.InputMessageClass{&tg.InputMessageID{ID: msgID}}
	var out store.Message
	err := WithRetry(ctx, func() error {
		var (
			result tg.MessagesMessagesClass
			err    error
		)
		if peer.Type == store.PeerChannel || peer.Type == store.PeerSuperGroup {
			result, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: &tg.InputChannel{ChannelID: peer.ID, AccessHash: peer.AccessHash},
				ID:      ids,
			})
		} else {
			result, err = api.MessagesGetMessages(ctx, ids)
		}
		if err != nil {
			c.log.Error("RefreshMessage failed", zap.Error(err))
			return err
		}
		msgs := parseHistory(result, peer.ID)
		m, ok := selectMessageByID(msgs, msgID)
		if !ok {
			return fmt.Errorf("refresh message %d: not found", msgID)
		}
		out = m
		return nil
	})
	return out, err
}

func (c *GotdClient) GetHistory(ctx context.Context, peer store.Peer, offsetID int, limit int) ([]store.Message, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil, fmt.Errorf("not connected")
	}

	c.traceLog.Debug("GetHistory", zap.Int64("peer_id", peer.ID), zap.Int("offsetID", offsetID), zap.Int("limit", limit))
	inputPeer := peerToInput(peer)
	var msgs []store.Message
	err := WithRetry(ctx, func() error {
		result, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     inputPeer,
			Limit:    limit,
			OffsetID: offsetID,
		})
		if err != nil {
			c.log.Error("MessagesGetHistory failed", zap.Error(err))
			return err
		}
		msgs = parseHistory(result, peer.ID)
		c.traceLog.Debug("GetHistory done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

func (c *GotdClient) SendMessage(ctx context.Context, peer store.Peer, text string, replyToMsgID int) (int, error) {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return 0, fmt.Errorf("not connected")
	}

	c.traceLog.Debug("SendMessage", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	inputPeer := peerToInput(peer)
	var realID int
	err := WithRetry(ctx, func() error {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return err
		}
		randomID := int64(binary.LittleEndian.Uint64(buf[:]))

		updates, err := api.MessagesSendMessage(ctx, buildSendRequest(inputPeer, text, randomID, replyToMsgID))
		if err != nil {
			c.log.Error("MessagesSendMessage failed", zap.Error(err))
			return err
		}
		realID = extractSentMessageID(updates, randomID)
		if realID != 0 {
			c.suppressMu.Lock()
			c.suppressIDs[realID] = struct{}{}
			c.suppressMu.Unlock()
		}
		c.traceLog.Debug("SendMessage ok", zap.Int64("peer_id", peer.ID), zap.Int("real_id", realID))
		return nil
	})
	return realID, err
}

func buildSendRequest(inputPeer tg.InputPeerClass, text string, randomID int64, replyToMsgID int) *tg.MessagesSendMessageRequest {
	req := &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  text,
		RandomID: randomID,
	}
	if replyToMsgID != 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID}
	}
	return req
}

func (c *GotdClient) MarkRead(ctx context.Context, peer store.Peer, maxID int) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	return WithRetry(ctx, func() error {
		if peer.Type == store.PeerChannel || peer.Type == store.PeerSuperGroup {
			_, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
				Channel: &tg.InputChannel{ChannelID: peer.ID, AccessHash: peer.AccessHash},
				MaxID:   maxID,
			})
			return err
		}
		_, err := api.MessagesReadHistory(ctx, &tg.MessagesReadHistoryRequest{
			Peer:  peerToInput(peer),
			MaxID: maxID,
		})
		return err
	})
}

func (c *GotdClient) DeleteMessages(ctx context.Context, peer store.Peer, ids []int, revoke bool) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	c.traceLog.Debug("DeleteMessages", zap.Int64("peer_id", peer.ID), zap.Int("count", len(ids)), zap.Bool("revoke", revoke))
	return WithRetry(ctx, func() error {
		if peer.Type == store.PeerChannel || peer.Type == store.PeerSuperGroup {
			// Channel/supergroup messages are always deleted for all members; revoke is N/A.
			_, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
				Channel: &tg.InputChannel{ChannelID: peer.ID, AccessHash: peer.AccessHash},
				ID:      ids,
			})
			return err
		}
		_, err := api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
			Revoke: revoke,
			ID:     ids,
		})
		return err
	})
}

func (c *GotdClient) EditMessage(ctx context.Context, peer store.Peer, msgID int, text string) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	c.traceLog.Debug("EditMessage", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
			Peer:    peerToInput(peer),
			ID:      msgID,
			Message: text,
		})
		if err != nil {
			c.log.Error("MessagesEditMessage failed", zap.Error(err))
		}
		return err
	})
}

func (c *GotdClient) SendReaction(ctx context.Context, peer store.Peer, msgID int, emoji string) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return fmt.Errorf("not connected")
	}
	c.traceLog.Debug("SendReaction", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID), zap.String("emoji", emoji))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
			Peer:     peerToInput(peer),
			MsgID:    msgID,
			Reaction: buildReactionArg(emoji),
		})
		if err != nil {
			c.log.Error("MessagesSendReaction failed", zap.Error(err))
		}
		return err
	})
}

func buildReactionArg(emoji string) []tg.ReactionClass {
	if emoji == "" {
		return []tg.ReactionClass{} // empty vector = remove reaction
	}
	return []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
}

func peerToInput(p store.Peer) tg.InputPeerClass {
	switch p.Type {
	case store.PeerUser:
		return &tg.InputPeerUser{UserID: p.ID, AccessHash: p.AccessHash}
	case store.PeerGroup:
		return &tg.InputPeerChat{ChatID: p.ID}
	case store.PeerChannel, store.PeerSuperGroup:
		return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

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

// pickFullThumbSize returns the largest available PhotoSize type for full-quality download.
func pickFullThumbSize(sizes []tg.PhotoSizeClass) string {
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

// pickThumbSize returns the best thumb type string for inline display.
// Prefers "m" (320px), then largest available PhotoSize by area.
func pickThumbSize(sizes []tg.PhotoSizeClass) string {
	for _, s := range sizes {
		if ps, ok := s.(*tg.PhotoSize); ok && ps.Type == "m" {
			return "m"
		}
	}
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
	if msg.EditDate != 0 {
		t := time.Unix(int64(msg.EditDate), 0)
		out.EditDate = &t
	}
	if msg.Reactions.Results != nil {
		out.Reactions = convertReactions(msg.Reactions)
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

func extractSentMessageID(updates tg.UpdatesClass, randomID int64) int {
	if short, ok := updates.(*tg.UpdateShortSentMessage); ok {
		return short.ID
	}
	upds, ok := updates.(*tg.Updates)
	if !ok {
		return 0
	}
	for _, u := range upds.Updates {
		if mid, ok := u.(*tg.UpdateMessageID); ok && mid.RandomID == randomID {
			return mid.ID
		}
	}
	return 0
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

func typingActionToTG(a store.TypingAction) tg.SendMessageActionClass {
	switch a {
	case store.TypingActionTyping:
		return &tg.SendMessageTypingAction{}
	case store.TypingActionRecordAudio:
		return &tg.SendMessageRecordAudioAction{}
	case store.TypingActionUploadAudio:
		return &tg.SendMessageUploadAudioAction{}
	case store.TypingActionRecordVideo:
		return &tg.SendMessageRecordVideoAction{}
	case store.TypingActionUploadVideo:
		return &tg.SendMessageUploadVideoAction{}
	case store.TypingActionUploadPhoto:
		return &tg.SendMessageUploadPhotoAction{}
	case store.TypingActionUploadDocument:
		return &tg.SendMessageUploadDocumentAction{}
	case store.TypingActionChooseSticker:
		return &tg.SendMessageChooseStickerAction{}
	case store.TypingActionRecordRound:
		return &tg.SendMessageRecordRoundAction{}
	default:
		return &tg.SendMessageCancelAction{}
	}
}

func (c *GotdClient) SetTyping(ctx context.Context, peer store.Peer, action store.TypingAction) error {
	c.mu.RLock()
	api := c.api
	c.mu.RUnlock()
	if api == nil {
		return nil
	}
	_, err := api.MessagesSetTyping(ctx, &tg.MessagesSetTypingRequest{
		Peer:   peerToInput(peer),
		Action: typingActionToTG(action),
	})
	if err != nil {
		c.traceLog.Debug("SetTyping failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
	}
	return err
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
		}
	}
	return out
}
