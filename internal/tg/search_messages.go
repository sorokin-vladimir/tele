package tg

import (
	"context"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// FoundMessages is Telegram's answer to a message search: the messages, newest
// first, and the chats they were found in, with the addresses to reach them.
type FoundMessages struct {
	Messages []domain.Message
	Chats    []domain.Chat
}

// SearchMessages asks Telegram which messages hold text: in the chat peer
// names (in one topic of it when topicID is not zero), or in every chat of the
// main list when peer is nil (#348).
func (c *GotdClient) SearchMessages(ctx context.Context, peer *domain.Peer, topicID int, text string, limit int) (FoundMessages, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return FoundMessages{}, err
	}
	var found FoundMessages
	err = WithRetry(ctx, func() error {
		var res tg.MessagesMessagesClass
		var err error
		if peer == nil {
			res, err = api.MessagesSearchGlobal(ctx, buildSearchGlobalRequest(text, limit))
		} else {
			res, err = api.MessagesSearch(ctx, buildSearchRequest(peerToInput(*peer), topicID, text, limit))
		}
		if err != nil {
			c.log.Error("message search failed", zap.Bool("everywhere", peer == nil), zap.Error(err))
			return err
		}
		found = parseFound(res)
		for _, m := range found.Messages {
			c.senderNames.put(m.SenderID, m.SenderName)
		}
		return nil
	})
	return found, err
}

// GetHistoryAround fetches up to limit messages around id, oldest first: the
// message itself, up to half the rest newer and the others older. With
// topicID it is one topic of a forum. A message that is not there is simply
// not among them (#348).
func (c *GotdClient) GetHistoryAround(ctx context.Context, peer domain.Peer, topicID, id, limit int) ([]domain.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}
	var msgs []domain.Message
	err = WithRetry(ctx, func() error {
		var res tg.MessagesMessagesClass
		var err error
		if topicID != 0 {
			res, err = api.MessagesGetReplies(ctx, buildAroundRepliesRequest(peerToInput(peer), topicID, id, limit))
		} else {
			res, err = api.MessagesGetHistory(ctx, buildAroundRequest(peerToInput(peer), id, limit))
		}
		if err != nil {
			c.log.Error("history around a message failed", zap.Int64("peer_id", peer.ID), zap.Int("topic", topicID), zap.Error(err))
			return err
		}
		msgs = parseHistory(res, peer.ID)
		for _, m := range msgs {
			c.senderNames.put(m.SenderID, m.SenderName)
		}
		return nil
	})
	return msgs, err
}

// buildSearchRequest asks one chat, or one topic of it, for text.
func buildSearchRequest(peer tg.InputPeerClass, topicID int, text string, limit int) *tg.MessagesSearchRequest {
	req := &tg.MessagesSearchRequest{
		Peer:   peer,
		Q:      text,
		Filter: &tg.InputMessagesFilterEmpty{},
		Limit:  limit,
	}
	if topicID != 0 {
		req.SetTopMsgID(topicID)
	}
	return req
}

// buildSearchGlobalRequest asks every chat of the main list for text. The
// folder is named: left out, Telegram searches the archive as well.
func buildSearchGlobalRequest(text string, limit int) *tg.MessagesSearchGlobalRequest {
	req := &tg.MessagesSearchGlobalRequest{
		Q:          text,
		Filter:     &tg.InputMessagesFilterEmpty{},
		OffsetPeer: &tg.InputPeerEmpty{},
		Limit:      limit,
	}
	req.SetFolderID(0)
	return req
}

// aroundOffset is the add_offset that puts id with half the rest of limit
// newer than it: Telegram pages back from offset_id, and a negative add_offset
// moves that many of the page past it, the message itself among them.
func aroundOffset(limit int) int {
	return -((limit-1)/2 + 1)
}

// buildAroundRequest asks a chat for the history around id.
func buildAroundRequest(peer tg.InputPeerClass, id, limit int) *tg.MessagesGetHistoryRequest {
	return &tg.MessagesGetHistoryRequest{Peer: peer, OffsetID: id, AddOffset: aroundOffset(limit), Limit: limit}
}

// buildAroundRepliesRequest asks one topic of a forum for the history around id.
func buildAroundRepliesRequest(peer tg.InputPeerClass, topicID, id, limit int) *tg.MessagesGetRepliesRequest {
	return &tg.MessagesGetRepliesRequest{Peer: peer, MsgID: topicID, OffsetID: id, AddOffset: aroundOffset(limit), Limit: limit}
}

// parseFound is a search's answer: each message in the chat it was sent in,
// newest first as Telegram sends them, and the chats those are.
func parseFound(res tg.MessagesMessagesClass) FoundMessages {
	rawMsgs, rawUsers, rawChats, _ := unpackMessages(res)
	msgs := convertMessages(rawMsgs, rawUsers, rawChats, func(raw tg.MessageClass) int64 {
		return peerIDFromPeer(messagePeer(raw))
	})

	withMessages := make(map[int64]bool, len(msgs))
	for _, m := range msgs {
		withMessages[m.ChatID] = true
	}
	var chats []domain.Chat
	for _, u := range rawUsers {
		if user, ok := u.(*tg.User); ok && withMessages[user.ID] {
			if chat, ok := convertUser(user); ok {
				chats = append(chats, chat)
			}
		}
	}
	for _, raw := range rawChats {
		var chat domain.Chat
		var ok bool
		switch ch := raw.(type) {
		case *tg.Chat:
			chat, ok = convertGroupChat(ch)
		case *tg.Channel:
			chat, ok = convertChannel(ch)
		}
		if ok && withMessages[chat.ID] {
			chats = append(chats, chat)
		}
	}
	return FoundMessages{Messages: msgs, Chats: chats}
}

// messagePeer is the chat a message was sent in.
func messagePeer(raw tg.MessageClass) tg.PeerClass {
	switch m := raw.(type) {
	case *tg.Message:
		return m.PeerID
	case *tg.MessageService:
		return m.PeerID
	case *tg.MessageEmpty:
		if p, ok := m.GetPeerID(); ok {
			return p
		}
	}
	return nil
}
