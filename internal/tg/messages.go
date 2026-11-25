package tg

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// RefreshMessages re-fetches several messages in one round-trip and returns them
// with fresh media refs and grouped_id. Only the messages the server returned
// are present; the order follows the server response.
func (c *GotdClient) RefreshMessages(ctx context.Context, peer domain.Peer, msgIDs []int) ([]domain.Message, error) {
	if len(msgIDs) == 0 {
		return nil, nil
	}
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	ids := make([]tg.InputMessageClass, 0, len(msgIDs))
	for _, id := range msgIDs {
		ids = append(ids, &tg.InputMessageID{ID: id})
	}
	var out []domain.Message
	err = WithRetry(ctx, func() error {
		var (
			result tg.MessagesMessagesClass
			err    error
		)
		if isChannelPeer(peer) {
			result, err = api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: inputChannel(peer),
				ID:      ids,
			})
		} else {
			result, err = api.MessagesGetMessages(ctx, ids)
		}
		if err != nil {
			c.log.Error("RefreshMessages failed", zap.Error(err))
			return err
		}
		found := selectMessagesByIDs(parseHistory(result, peer.ID), msgIDs)
		if len(found) == 0 {
			return &telerr.Error{Kind: telerr.NotFound, Op: "refresh messages", Detail: "none found"}
		}
		out = found
		return nil
	})
	return out, err
}

// RefreshMessage re-fetches one message and returns it with fresh media refs.
func (c *GotdClient) RefreshMessage(ctx context.Context, peer domain.Peer, msgID int) (domain.Message, error) {
	msgs, err := c.RefreshMessages(ctx, peer, []int{msgID})
	if err != nil {
		return domain.Message{}, err
	}
	m, ok := selectMessageByID(msgs, msgID)
	if !ok {
		return domain.Message{}, &telerr.Error{Kind: telerr.NotFound, Op: "refresh message", Detail: "not found"}
	}
	return m, nil
}

// selectMessagesByIDs keeps only the messages whose ID is in want, preserving
// the order of msgs.
func selectMessagesByIDs(msgs []domain.Message, want []int) []domain.Message {
	set := make(map[int]struct{}, len(want))
	for _, id := range want {
		set[id] = struct{}{}
	}
	out := make([]domain.Message, 0, len(want))
	for _, m := range msgs {
		if _, ok := set[m.ID]; ok {
			out = append(out, m)
		}
	}
	return out
}

func (c *GotdClient) GetHistory(ctx context.Context, peer domain.Peer, offsetID int, limit int) ([]domain.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	c.traceLog.Debug("GetHistory", zap.Int64("peer_id", peer.ID), zap.Int("offsetID", offsetID), zap.Int("limit", limit))
	inputPeer := peerToInput(peer)
	var msgs []domain.Message
	err = WithRetry(ctx, func() error {
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
		// Seed the sender-name cache from the fully-resolved history so a later
		// live update that omits a sender's entity still resolves the name (#161).
		for _, m := range msgs {
			c.senderNames.put(m.SenderID, m.SenderName)
		}
		c.traceLog.Debug("GetHistory done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

// GetHistoryAfter fetches up to limit messages newer than afterID, oldest
// first. It is the direction backfill does not go: messages.getHistory pages
// backwards from offset_id, and a negative add_offset is what moves the window
// to the other side of it.
//
// Telegram slides the window when there are fewer than limit messages newer
// than afterID, so the page can come back holding messages at or below afterID.
// The caller sees that as a page whose newest message did not advance, which is
// how it learns there is nothing newer to fetch.
//
// With topicID it is one topic of a forum, whose history is its replies.
func (c *GotdClient) GetHistoryAfter(ctx context.Context, peer domain.Peer, topicID, afterID, limit int) ([]domain.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}

	c.traceLog.Debug("GetHistoryAfter", zap.Int64("peer_id", peer.ID), zap.Int("topic", topicID), zap.Int("afterID", afterID), zap.Int("limit", limit))
	inputPeer := peerToInput(peer)
	var msgs []domain.Message
	err = WithRetry(ctx, func() error {
		var result tg.MessagesMessagesClass
		var err error
		if topicID != 0 {
			result, err = api.MessagesGetReplies(ctx, buildAfterRepliesRequest(inputPeer, topicID, afterID, limit))
		} else {
			result, err = api.MessagesGetHistory(ctx, buildAfterRequest(inputPeer, afterID, limit))
		}
		if err != nil {
			c.log.Error("MessagesGetHistory forward failed", zap.Error(err))
			return err
		}
		msgs = parseHistory(result, peer.ID)
		// Seed the sender-name cache from the fully-resolved history so a later
		// live update that omits a sender's entity still resolves the name (#161).
		for _, m := range msgs {
			c.senderNames.put(m.SenderID, m.SenderName)
		}
		c.traceLog.Debug("GetHistoryAfter done", zap.Int64("peer_id", peer.ID), zap.Int("count", len(msgs)))
		return nil
	})
	return msgs, err
}

// buildAfterRequest asks a chat for the history newer than afterID.
func buildAfterRequest(peer tg.InputPeerClass, afterID, limit int) *tg.MessagesGetHistoryRequest {
	return &tg.MessagesGetHistoryRequest{Peer: peer, OffsetID: afterID, AddOffset: -limit, Limit: limit}
}

// buildAfterRepliesRequest asks one topic of a forum for the history newer
// than afterID.
func buildAfterRepliesRequest(peer tg.InputPeerClass, topicID, afterID, limit int) *tg.MessagesGetRepliesRequest {
	return &tg.MessagesGetRepliesRequest{Peer: peer, MsgID: topicID, OffsetID: afterID, AddOffset: -limit, Limit: limit}
}

func (c *GotdClient) SendMessage(ctx context.Context, peer domain.Peer, text string, replyToMsgID, topicID int, entities []domain.MessageEntity, randomID int64) (domain.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return domain.Message{}, err
	}

	c.traceLog.Debug("SendMessage", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	inputPeer := peerToInput(peer)
	var sent domain.Message
	err = WithRetry(ctx, func() error {
		updates, err := api.MessagesSendMessage(ctx, buildSendRequest(inputPeer, text, randomID, replyToMsgID, topicID, entities))
		if err != nil {
			c.log.Error("MessagesSendMessage failed", zap.Error(err))
			return err
		}
		// The created message is returned rather than left to the update stream:
		// Telegram sends no echo for your own message, and the reply to a send
		// into a user chat is an updateShortSentMessage with no body at all. The
		// caller records it, exactly as it always had to (#193).
		//
		// Nothing is suppressed here any more. Suppression existed so a client's
		// optimistic bubble would not double up; the durable outbox has none, and
		// a reply that does carry the message (groups, channels) is applied by id,
		// so the pipeline delivering it a second time is a no-op. Media keeps
		// suppressing until #195.
		sent = sentMessage(updates, randomID, peer, text, replyToMsgID, topicID, entities)
		c.traceLog.Debug("SendMessage ok", zap.Int64("peer_id", peer.ID), zap.Int("real_id", sent.ID))
		return nil
	})
	return sent, err
}

// SendMediaParams carries everything SendMedia needs. Media is a ready-made
// InputMediaClass; SendMedia is type-agnostic and does not inspect it.
type SendMediaParams struct {
	Peer         domain.Peer
	Media        tg.InputMediaClass
	Caption      string
	ReplyToMsgID int
	// TopicID is the forum topic the send lands in, 0 outside a forum (#275).
	TopicID  int
	Entities []domain.MessageEntity
	// RandomID is the caller's deduplication key. It must stay the same across
	// every retry of one logical send, or Telegram cannot tell a retry from a
	// second message (#193).
	RandomID int64
}

func (c *GotdClient) SendMedia(ctx context.Context, p SendMediaParams) (int, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return 0, err
	}

	c.traceLog.Debug("SendMedia", zap.Int64("peer_id", p.Peer.ID), zap.Int("caption_len", len(p.Caption)))
	inputPeer := peerToInput(p.Peer)
	var realID int
	err = WithRetry(ctx, func() error {
		updates, err := api.MessagesSendMedia(ctx, buildSendMediaRequest(inputPeer, p.Media, p.Caption, p.RandomID, p.ReplyToMsgID, p.TopicID, p.Entities))
		if err != nil {
			c.log.Error("MessagesSendMedia failed", zap.Error(err))
			return err
		}
		realID = extractSentMessageID(updates, p.RandomID)
		if realID != 0 {
			c.suppressMu.Lock()
			c.suppressIDs[realID] = struct{}{}
			c.suppressMu.Unlock()
		}
		c.traceLog.Debug("SendMedia ok", zap.Int64("peer_id", p.Peer.ID), zap.Int("real_id", realID))
		return nil
	})
	return realID, err
}

// ForwardMessages forwards messages by ID from one peer to another via
// messages.forwardMessages. No optimistic insert is performed: when the target
// is the open chat, the message arrives through the normal live-update path.
func (c *GotdClient) ForwardMessages(ctx context.Context, from domain.Peer, to domain.Peer, toTopicID int, ids []int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("ForwardMessages", zap.Int64("from", from.ID), zap.Int64("to", to.ID), zap.Int("count", len(ids)))
	return WithRetry(ctx, func() error {
		randomIDs := make([]int64, len(ids))
		for i := range randomIDs {
			var buf [8]byte
			if _, err := rand.Read(buf[:]); err != nil {
				return err
			}
			randomIDs[i] = int64(binary.LittleEndian.Uint64(buf[:]))
		}
		updates, err := api.MessagesForwardMessages(ctx, buildForwardRequest(peerToInput(from), peerToInput(to), ids, randomIDs, toTopicID))
		if err != nil {
			c.log.Error("MessagesForwardMessages failed", zap.Error(err))
			return err
		}
		// The reply carries the created messages. Nothing is done with them —
		// the copies are expected to arrive through the live update path — so
		// say what came back, which is where a forward that never renders has to
		// be traced from.
		c.log.Debug("forward: reply from telegram",
			zap.Int64("to", to.ID),
			zap.Strings("updates", describeUpdates(updates)),
			zap.Ints("new_msg_ids", extractSentMessageIDs(updates, randomIDs)))
		return nil
	})
}

// describeUpdates names the updates in an RPC reply, so a log line shows what
// Telegram answered with rather than only that it answered.
func describeUpdates(updates tg.UpdatesClass) []string {
	upds, ok := updates.(*tg.Updates)
	if !ok {
		return []string{fmt.Sprintf("%T", updates)}
	}
	out := make([]string, 0, len(upds.Updates))
	for _, u := range upds.Updates {
		switch t := u.(type) {
		case *tg.UpdateNewMessage:
			out = append(out, fmt.Sprintf("NewMessage(id=%d)", messageID(t.Message)))
		case *tg.UpdateNewChannelMessage:
			out = append(out, fmt.Sprintf("NewChannelMessage(id=%d)", messageID(t.Message)))
		case *tg.UpdateMessageID:
			out = append(out, fmt.Sprintf("MessageID(id=%d)", t.ID))
		default:
			out = append(out, fmt.Sprintf("%T", u))
		}
	}
	return out
}

// messageID reports a message's id whatever concrete class it is.
func messageID(m tg.MessageClass) int {
	switch t := m.(type) {
	case *tg.Message:
		return t.ID
	case *tg.MessageService:
		return t.ID
	default:
		return 0
	}
}

// buildForwardRequest forwards ids into toPeer, and into toTopicID there when
// the target is a forum topic other than General (#275).
func buildForwardRequest(fromPeer, toPeer tg.InputPeerClass, ids []int, randomIDs []int64, toTopicID int) *tg.MessagesForwardMessagesRequest {
	req := &tg.MessagesForwardMessagesRequest{
		FromPeer: fromPeer,
		ToPeer:   toPeer,
		ID:       ids,
		RandomID: randomIDs,
	}
	if toTopicID != 0 && toTopicID != domain.GeneralTopicID {
		req.TopMsgID = toTopicID
	}
	return req
}

func buildSendRequest(inputPeer tg.InputPeerClass, text string, randomID int64, replyToMsgID, topicID int, entities []domain.MessageEntity) *tg.MessagesSendMessageRequest {
	req := &tg.MessagesSendMessageRequest{
		Peer:     inputPeer,
		Message:  text,
		RandomID: randomID,
	}
	if r := replyHeader(replyToMsgID, topicID); r != nil {
		req.ReplyTo = r
	}
	if ent := convertToTGEntities(entities); len(ent) > 0 {
		req.Entities = ent
	}
	return req
}

// convertToTGEntities maps store entities to Telegram send-side entities.
// Name-based mentions carry an InputUser and need InputMessageEntityMentionName;
// auto-detected types (url, email, hashtag, …) are found server-side and are
// skipped, as are types we cannot produce.
func convertToTGEntities(es []domain.MessageEntity) []tg.MessageEntityClass {
	if len(es) == 0 {
		return nil
	}
	var out []tg.MessageEntityClass
	for _, e := range es {
		switch e.Type {
		case "bold":
			out = append(out, &tg.MessageEntityBold{Offset: e.Offset, Length: e.Length})
		case "italic":
			out = append(out, &tg.MessageEntityItalic{Offset: e.Offset, Length: e.Length})
		case "strike":
			out = append(out, &tg.MessageEntityStrike{Offset: e.Offset, Length: e.Length})
		case "underline":
			out = append(out, &tg.MessageEntityUnderline{Offset: e.Offset, Length: e.Length})
		case "code":
			out = append(out, &tg.MessageEntityCode{Offset: e.Offset, Length: e.Length})
		case "pre":
			out = append(out, &tg.MessageEntityPre{Offset: e.Offset, Length: e.Length, Language: e.Language})
		case "text_url":
			out = append(out, &tg.MessageEntityTextURL{Offset: e.Offset, Length: e.Length, URL: e.URL})
		case "mention_name":
			out = append(out, &tg.InputMessageEntityMentionName{
				Offset: e.Offset,
				Length: e.Length,
				UserID: &tg.InputUser{UserID: e.UserID, AccessHash: e.AccessHash},
			})
		}
	}
	return out
}

func buildSendMediaRequest(inputPeer tg.InputPeerClass, media tg.InputMediaClass, caption string, randomID int64, replyToMsgID, topicID int, entities []domain.MessageEntity) *tg.MessagesSendMediaRequest {
	req := &tg.MessagesSendMediaRequest{
		Peer:     inputPeer,
		Media:    media,
		Message:  caption,
		RandomID: randomID,
	}
	if r := replyHeader(replyToMsgID, topicID); r != nil {
		req.ReplyTo = r
	}
	if ent := convertToTGEntities(entities); len(ent) > 0 {
		req.Entities = ent
	}
	return req
}

// BuildInputMediaUploadedPhoto wraps an uploaded InputFile into an
// InputMediaUploadedPhoto for messages.sendMedia. The server recompresses it and
// produces an inline photo. Caption/entities are carried separately by SendMedia.
func BuildInputMediaUploadedPhoto(f tg.InputFileClass) tg.InputMediaClass {
	return &tg.InputMediaUploadedPhoto{File: f}
}

// BuildInputMediaUploadedDocument wraps an uploaded InputFile into an
// InputMediaUploadedDocument for messages.sendMedia, forcing the generic
// document path (ForceFile) so Telegram does not reinterpret an image as a
// photo. The filename is attached via DocumentAttributeFilename; an empty MIME
// falls back to application/octet-stream.
func BuildInputMediaUploadedDocument(f tg.InputFileClass, fileName, mime string) tg.InputMediaClass {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return &tg.InputMediaUploadedDocument{
		File:     f,
		MimeType: mime,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: fileName},
		},
		ForceFile: true,
	}
}

// BuildInputMediaUploadedVideo wraps an uploaded InputFile into an
// InputMediaUploadedDocument carrying DocumentAttributeVideo (always
// SupportsStreaming; duration/w/h are passed through verbatim) plus
// DocumentAttributeFilename. Unlike the generic document builder it does NOT set
// ForceFile, so Telegram renders it as inline video. thumb is optional: when
// non-nil it is attached as the document Thumb so the bubble has a preview before
// the server generates its own. An empty mime falls back to video/mp4.
func BuildInputMediaUploadedVideo(f tg.InputFileClass, fileName, mime string, dur, w, h int, thumb tg.InputFileClass) tg.InputMediaClass {
	if mime == "" {
		mime = "video/mp4"
	}
	doc := &tg.InputMediaUploadedDocument{
		File:     f,
		MimeType: mime,
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeVideo{
				SupportsStreaming: true,
				Duration:          float64(dur),
				W:                 w,
				H:                 h,
			},
			&tg.DocumentAttributeFilename{FileName: fileName},
		},
	}
	if thumb != nil {
		doc.SetThumb(thumb)
	}
	return doc
}

func (c *GotdClient) MarkRead(ctx context.Context, peer domain.Peer, maxID int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		if isChannelPeer(peer) {
			_, err := api.ChannelsReadHistory(ctx, &tg.ChannelsReadHistoryRequest{
				Channel: inputChannel(peer),
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

func (c *GotdClient) ReadReactions(ctx context.Context, peer domain.Peer, topicID int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		_, err := api.MessagesReadReactions(ctx, buildReadReactionsRequest(peerToInput(peer), topicID))
		return err
	})
}

func (c *GotdClient) ReadMentions(ctx context.Context, peer domain.Peer, topicID int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		_, err := api.MessagesReadMentions(ctx, buildReadMentionsRequest(peerToInput(peer), topicID))
		return err
	})
}

// buildReadReactionsRequest reads a chat's unread reactions, or one forum
// topic's (#275).
func buildReadReactionsRequest(peer tg.InputPeerClass, topicID int) *tg.MessagesReadReactionsRequest {
	return &tg.MessagesReadReactionsRequest{Peer: peer, TopMsgID: topicID}
}

// buildReadMentionsRequest reads a chat's unread mentions, or one forum
// topic's (#275).
func buildReadMentionsRequest(peer tg.InputPeerClass, topicID int) *tg.MessagesReadMentionsRequest {
	return &tg.MessagesReadMentionsRequest{Peer: peer, TopMsgID: topicID}
}

// ReadDiscussion marks a forum topic read up to maxID. A topic is a replies
// thread, General's being thread 1, and is read as one (#275).
func (c *GotdClient) ReadDiscussion(ctx context.Context, peer domain.Peer, topicID, maxID int) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	return WithRetry(ctx, func() error {
		_, err := api.MessagesReadDiscussion(ctx, &tg.MessagesReadDiscussionRequest{
			Peer:      peerToInput(peer),
			MsgID:     topicID,
			ReadMaxID: maxID,
		})
		return err
	})
}

func (c *GotdClient) DeleteMessages(ctx context.Context, peer domain.Peer, ids []int, revoke bool) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("DeleteMessages", zap.Int64("peer_id", peer.ID), zap.Int("count", len(ids)), zap.Bool("revoke", revoke))
	return WithRetry(ctx, func() error {
		if isChannelPeer(peer) {
			// Channel/supergroup messages are always deleted for all members; revoke is N/A.
			_, err := api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{
				Channel: inputChannel(peer),
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

func buildEditRequest(inputPeer tg.InputPeerClass, msgID int, text string, entities []domain.MessageEntity) *tg.MessagesEditMessageRequest {
	req := &tg.MessagesEditMessageRequest{
		Peer:    inputPeer,
		ID:      msgID,
		Message: text,
	}
	if ent := convertToTGEntities(entities); len(ent) > 0 {
		req.Entities = ent
	}
	return req
}

func (c *GotdClient) EditMessage(ctx context.Context, peer domain.Peer, msgID int, text string, entities []domain.MessageEntity) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("EditMessage", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesEditMessage(ctx, buildEditRequest(peerToInput(peer), msgID, text, entities))
		if err != nil {
			c.log.Error("MessagesEditMessage failed", zap.Error(err))
		}
		return err
	})
}

// SaveDraft persists (or clears, when text is empty) the message draft for a
// peer via messages.saveDraft (#62). Telegram broadcasts the change to the
// account's other clients as updateDraftMessage.
func (c *GotdClient) SaveDraft(ctx context.Context, peer domain.Peer, topicID int, text string) error {
	api, err := c.acquireAPI()
	if err != nil {
		return err
	}
	c.traceLog.Debug("SaveDraft", zap.Int64("peer_id", peer.ID), zap.Int("text_len", len(text)))
	return WithRetry(ctx, func() error {
		_, err := api.MessagesSaveDraft(ctx, buildSaveDraftRequest(peerToInput(peer), text, topicID))
		if err != nil {
			c.log.Error("MessagesSaveDraft failed", zap.Error(err))
		}
		return err
	})
}

// buildSaveDraftRequest saves a draft for a chat, or for one topic of a forum:
// a draft is kept where the message would be sent (#275).
func buildSaveDraftRequest(inputPeer tg.InputPeerClass, text string, topicID int) *tg.MessagesSaveDraftRequest {
	req := &tg.MessagesSaveDraftRequest{
		Peer:    inputPeer,
		Message: text,
	}
	if r := replyHeader(0, topicID); r != nil {
		req.ReplyTo = r
	}
	return req
}

func (c *GotdClient) SendReaction(ctx context.Context, peer domain.Peer, msgID int, emoji string) ([]domain.Reaction, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}
	c.traceLog.Debug("SendReaction", zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID), zap.String("emoji", emoji))
	attempt := 0
	var confirmed []domain.Reaction
	err = WithRetry(ctx, func() error {
		attempt++
		start := time.Now()
		reply, err := api.MessagesSendReaction(ctx, &tg.MessagesSendReactionRequest{
			Peer:     peerToInput(peer),
			MsgID:    msgID,
			Reaction: buildReactionArg(emoji),
		})
		// Reaction trace (#248): the reply is where Telegram states the set it
		// ended up with. Its updates also reach the dispatcher through the
		// update hook, so this line is what pairs them with this request.
		if err != nil {
			c.log.Error("MessagesSendReaction failed", zap.Error(err))
			c.traceLog.Debug("reaction: send attempt failed",
				zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID), zap.Int("attempt", attempt),
				zap.Duration("took", time.Since(start)), zap.Error(err))
			return err
		}
		types, found := reactionsInReply(reply, msgID)
		c.traceLog.Debug("reaction: send reply",
			zap.Int64("peer_id", peer.ID), zap.Int("msg_id", msgID), zap.String("emoji", emoji),
			zap.Int("attempt", attempt), zap.Duration("took", time.Since(start)),
			zap.Strings("updates", types), zap.String("reactions", found))
		confirmed = confirmedReactions(reply, msgID)
		return nil
	})
	return confirmed, err
}

// confirmedReactions is the set the reply to messages.sendReaction states for
// msgID, or nil when it states none the owner can judge our pick against
// (#248). Only an UpdateMessageReactions states it: an edit's copy yields to
// one (see yieldPairedEditReactions), and a min set does not say which
// reaction is ours.
func confirmedReactions(reply tg.UpdatesClass, msgID int) []domain.Reaction {
	var upds []tg.UpdateClass
	switch r := reply.(type) {
	case *tg.Updates:
		upds = r.Updates
	case *tg.UpdatesCombined:
		upds = r.Updates
	case *tg.UpdateShort:
		upds = []tg.UpdateClass{r.Update}
	}
	for _, u := range upds {
		if r, ok := u.(*tg.UpdateMessageReactions); ok && r.MsgID == msgID && !r.Reactions.Min {
			return convertReactions(r.Reactions)
		}
	}
	return nil
}

func buildReactionArg(emoji string) []tg.ReactionClass {
	if emoji == "" {
		return []tg.ReactionClass{} // empty vector = remove reaction
	}
	return []tg.ReactionClass{&tg.ReactionEmoji{Emoticon: emoji}}
}

// isChannelPeer reports whether a peer is addressed via the channels.* API
// (channels and supergroups), as opposed to the messages.* API.
func isChannelPeer(p domain.Peer) bool {
	return p.Type == domain.PeerChannel || p.Type == domain.PeerSuperGroup
}

// inputChannel builds the InputChannel for a channel/supergroup peer.
func inputChannel(p domain.Peer) *tg.InputChannel {
	return &tg.InputChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
}

func peerToInput(p domain.Peer) tg.InputPeerClass {
	switch p.Type {
	case domain.PeerUser:
		return &tg.InputPeerUser{UserID: p.ID, AccessHash: p.AccessHash}
	case domain.PeerGroup:
		return &tg.InputPeerChat{ChatID: p.ID}
	case domain.PeerChannel, domain.PeerSuperGroup:
		return &tg.InputPeerChannel{ChannelID: p.ID, AccessHash: p.AccessHash}
	default:
		return &tg.InputPeerEmpty{}
	}
}

// extractSentMessageIDs maps each randomID to the message ID Telegram assigned
// it. The result is index-aligned with randomIDs; an entry the server did not
// report back is 0.
func extractSentMessageIDs(updates tg.UpdatesClass, randomIDs []int64) []int {
	out := make([]int, len(randomIDs))
	if short, ok := updates.(*tg.UpdateShortSentMessage); ok {
		if len(out) > 0 {
			out[0] = short.ID
		}
		return out
	}
	upds, ok := updates.(*tg.Updates)
	if !ok {
		return out
	}
	index := make(map[int64]int, len(upds.Updates))
	for _, u := range upds.Updates {
		if mid, ok := u.(*tg.UpdateMessageID); ok {
			index[mid.RandomID] = mid.ID
		}
	}
	for i, rid := range randomIDs {
		out[i] = index[rid]
	}
	return out
}

func extractSentMessageID(updates tg.UpdatesClass, randomID int64) int {
	return extractSentMessageIDs(updates, []int64{randomID})[0]
}

// sentMessage is the message a send created.
//
// It has to be assembled here because Telegram does not echo your own message
// back through the update stream. messages.sendMessage to a user answers with
// updateShortSentMessage, which carries an id, a date and nothing else — no
// text, no peer, and no UpdateNewMessage for the dispatcher to convert. Nothing
// downstream would ever see the message otherwise (#193).
//
// A group or channel does answer with the whole message; that one is preferred,
// since it carries what the server decided rather than what was asked for.
func sentMessage(updates tg.UpdatesClass, randomID int64, peer domain.Peer, text string, replyToMsgID, topicID int, entities []domain.MessageEntity) domain.Message {
	msg := domain.Message{
		ChatID:       peer.ID,
		Text:         text,
		Entities:     entities,
		ReplyToMsgID: replyToMsgID,
		TopicID:      topicID,
		IsOut:        true,
		Date:         time.Now(),
	}
	switch u := updates.(type) {
	case *tg.UpdateShortSentMessage:
		msg.ID = u.ID
		msg.Date = time.Unix(int64(u.Date), 0)
	case *tg.Updates:
		msg.ID = extractSentMessageID(updates, randomID)
		if full, ok := findSentMessage(u.Updates, msg.ID, peer.ID); ok {
			return full
		}
	}
	return msg
}

// findSentMessage picks the created message out of a reply that carries one.
func findSentMessage(updates []tg.UpdateClass, msgID int, chatID int64) (domain.Message, bool) {
	if msgID == 0 {
		return domain.Message{}, false
	}
	for _, u := range updates {
		var raw tg.MessageClass
		switch v := u.(type) {
		case *tg.UpdateNewMessage:
			raw = v.Message
		case *tg.UpdateNewChannelMessage:
			raw = v.Message
		default:
			continue
		}
		msg, ok := convertMessage(raw, chatID)
		if ok && msg.ID == msgID {
			return msg, true
		}
	}
	return domain.Message{}, false
}

func typingActionToTG(a domain.TypingAction) tg.SendMessageActionClass {
	switch a {
	case domain.TypingActionTyping:
		return &tg.SendMessageTypingAction{}
	case domain.TypingActionRecordAudio:
		return &tg.SendMessageRecordAudioAction{}
	case domain.TypingActionUploadAudio:
		return &tg.SendMessageUploadAudioAction{}
	case domain.TypingActionRecordVideo:
		return &tg.SendMessageRecordVideoAction{}
	case domain.TypingActionUploadVideo:
		return &tg.SendMessageUploadVideoAction{}
	case domain.TypingActionUploadPhoto:
		return &tg.SendMessageUploadPhotoAction{}
	case domain.TypingActionUploadDocument:
		return &tg.SendMessageUploadDocumentAction{}
	case domain.TypingActionChooseSticker:
		return &tg.SendMessageChooseStickerAction{}
	case domain.TypingActionRecordRound:
		return &tg.SendMessageRecordRoundAction{}
	default:
		return &tg.SendMessageCancelAction{}
	}
}

func (c *GotdClient) SetTyping(ctx context.Context, peer domain.Peer, topicID int, action domain.TypingAction) error {
	api, err := c.acquireAPI()
	if err != nil {
		return nil //nolint:nilerr // typing is best-effort; ignore when not connected
	}
	_, err = api.MessagesSetTyping(ctx, buildSetTypingRequest(peerToInput(peer), typingActionToTG(action), topicID))
	if err != nil {
		c.traceLog.Debug("SetTyping failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
	}
	return err
}
