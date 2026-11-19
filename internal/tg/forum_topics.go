package tg

import (
	"context"
	"time"

	"github.com/gotd/td/tg"
	"go.uber.org/zap"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/store"
)

// TopicsOffset is where a page of topics continues from: the last topic of the
// page before it and its newest message. The zero value asks for the first
// page.
type TopicsOffset struct {
	Date    time.Time
	MsgID   int
	TopicID int
}

// GetForumTopics fetches a page of a forum's topics, newest activity first, the
// order the official clients list them in.
func (c *GotdClient) GetForumTopics(ctx context.Context, peer domain.Peer, after TopicsOffset, limit int) (ForumTopicsPage, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return ForumTopicsPage{}, err
	}
	req := &tg.MessagesGetForumTopicsRequest{
		Peer:        peerToInput(peer),
		OffsetID:    after.MsgID,
		OffsetTopic: after.TopicID,
		Limit:       limit,
	}
	if !after.Date.IsZero() {
		req.OffsetDate = int(after.Date.Unix())
	}
	var page ForumTopicsPage
	err = WithRetry(ctx, func() error {
		res, err := api.MessagesGetForumTopics(ctx, req)
		if err != nil {
			c.log.Error("MessagesGetForumTopics failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
			return err
		}
		page = parseForumTopics(res, peer.ID)
		return nil
	})
	return page, err
}

// GetReplies fetches a page of one topic's history, older than offsetID, or
// the newest when offsetID is zero. A topic's history is its replies thread:
// General's is thread 1, as the official clients read it.
func (c *GotdClient) GetReplies(ctx context.Context, peer domain.Peer, topicID, offsetID, limit int) ([]domain.Message, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return nil, err
	}
	var msgs []domain.Message
	err = WithRetry(ctx, func() error {
		res, err := api.MessagesGetReplies(ctx, &tg.MessagesGetRepliesRequest{
			Peer:     peerToInput(peer),
			MsgID:    topicID,
			OffsetID: offsetID,
			Limit:    limit,
		})
		if err != nil {
			c.log.Error("MessagesGetReplies failed", zap.Int64("peer_id", peer.ID), zap.Int("topic", topicID), zap.Error(err))
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

// GetForumTopicsByID fetches named topics of a forum: the authoritative state
// of each, read again whenever something about one of them changes.
func (c *GotdClient) GetForumTopicsByID(ctx context.Context, peer domain.Peer, ids []int) (ForumTopicsPage, error) {
	api, err := c.acquireAPI()
	if err != nil {
		return ForumTopicsPage{}, err
	}
	var page ForumTopicsPage
	err = WithRetry(ctx, func() error {
		res, err := api.MessagesGetForumTopicsByID(ctx, &tg.MessagesGetForumTopicsByIDRequest{
			Peer:   peerToInput(peer),
			Topics: ids,
		})
		if err != nil {
			c.log.Error("MessagesGetForumTopicsByID failed", zap.Int64("peer_id", peer.ID), zap.Error(err))
			return err
		}
		page = parseForumTopics(res, peer.ID)
		return nil
	})
	return page, err
}

// replyHeader says where a message goes in its chat: what it replies to, and in
// a forum which topic it lands in. A plain message in a topic names the topic
// as what it replies to; a reply inside the topic names the message it
// answers and carries the topic as its top. General takes messages that name
// no topic, so sending there is sending an ordinary message (#275).
func replyHeader(replyToMsgID, topicID int) tg.InputReplyToClass {
	if topicID == domain.GeneralTopicID || topicID <= 0 {
		topicID = 0
	}
	switch {
	case replyToMsgID != 0 && topicID != 0:
		return &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID, TopMsgID: topicID}
	case replyToMsgID != 0:
		return &tg.InputReplyToMessage{ReplyToMsgID: replyToMsgID}
	case topicID != 0:
		return &tg.InputReplyToMessage{ReplyToMsgID: topicID}
	}
	return nil
}

// notifyPeer names whose notification setting is changed: a chat's, or one
// forum topic's (#275).
func notifyPeer(peer tg.InputPeerClass, topicID int) tg.InputNotifyPeerClass {
	if topicID > 0 {
		return &tg.InputNotifyForumTopic{Peer: peer, TopMsgID: topicID}
	}
	return &tg.InputNotifyPeer{Peer: peer}
}

// buildSetTypingRequest reports typing in a chat, or in one topic of a forum;
// in General it is reported as in any ordinary chat.
func buildSetTypingRequest(inputPeer tg.InputPeerClass, action tg.SendMessageActionClass, topicID int) *tg.MessagesSetTypingRequest {
	req := &tg.MessagesSetTypingRequest{Peer: inputPeer, Action: action}
	if topicID > 0 && topicID != domain.GeneralTopicID {
		req.TopMsgID = topicID
	}
	return req
}

// topicServiceEvent recognises the service message that opens or edits a
// forum topic. It names the topic to read again: the one the message opened,
// or the one it was posted in.
func topicServiceEvent(raw tg.MessageClass) (store.Event, bool) {
	svc, ok := raw.(*tg.MessageService)
	if !ok {
		return store.Event{}, false
	}
	chatID := peerIDFromPeer(svc.PeerID)
	switch svc.Action.(type) {
	case *tg.MessageActionTopicCreate:
		return store.Event{Kind: store.EventTopicsChanged, ChatID: chatID, MsgIDs: []int{svc.ID}}, true
	case *tg.MessageActionTopicEdit:
		hdr, ok := svc.ReplyTo.(*tg.MessageReplyHeader)
		if !ok || !hdr.ForumTopic {
			return store.Event{}, false
		}
		topic := hdr.ReplyToMsgID
		if hdr.ReplyToTopID != 0 {
			topic = hdr.ReplyToTopID
		}
		return store.Event{Kind: store.EventTopicsChanged, ChatID: chatID, MsgIDs: []int{topic}}, true
	}
	return store.Event{}, false
}

// ForumTopicsPage is what Telegram said about a forum's topics in one answer:
// the topics it described and the ones it reported deleted.
type ForumTopicsPage struct {
	Topics  []domain.Topic
	Deleted []int
	// Total is how many topics the forum has in all, which is how a reader of
	// one page knows whether there is another.
	Total int
}

// parseForumTopics converts a getForumTopics or getForumTopicsByID answer. The
// newest message of each topic comes in the same answer, and is converted the
// way a history page is, so its preview names its sender.
func parseForumTopics(res *tg.MessagesForumTopics, chatID int64) ForumTopicsPage {
	msgs := parseHistory(&tg.MessagesMessages{Messages: res.Messages, Users: res.Users, Chats: res.Chats}, chatID)

	page := ForumTopicsPage{Total: res.Count}
	for _, raw := range res.Topics {
		switch t := raw.(type) {
		case *tg.ForumTopic:
			topic := domain.Topic{
				ChatID:               chatID,
				ID:                   t.ID,
				Title:                t.Title,
				Pinned:               t.Pinned,
				Closed:               t.Closed,
				Hidden:               t.Hidden,
				TopMessageID:         t.TopMessage,
				ReadInboxMaxID:       t.ReadInboxMaxID,
				ReadOutboxMaxID:      t.ReadOutboxMaxID,
				UnreadCount:          t.UnreadCount,
				UnreadMentionsCount:  t.UnreadMentionsCount,
				UnreadReactionsCount: t.UnreadReactionsCount,
			}
			if d, ok := t.Draft.(*tg.DraftMessage); ok {
				topic.Draft = d.Message
			}
			if _, own := t.NotifySettings.GetMuteUntil(); own {
				topic.Mute = domain.TopicUnmuted
				if mutedFromSettings(t.NotifySettings) {
					topic.Mute = domain.TopicMuted
				}
			}
			if m, ok := selectMessageByID(msgs, t.TopMessage); ok {
				topic.LastMessage = &m
			}
			page.Topics = append(page.Topics, topic)
		case *tg.ForumTopicDeleted:
			page.Deleted = append(page.Deleted, t.ID)
		}
	}
	return page
}
