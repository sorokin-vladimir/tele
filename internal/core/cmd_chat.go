package core

import (
	"context"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// SetMuted mutes or unmutes a chat. The new state is applied before the request
// so every attached client sees it at once, and undone if Telegram refuses.
//
// A forum topic is muted on its own, as a setting of the topic: unmuting one
// topic of a muted forum is how a person hears that topic alone (#275).
func (o *Owner) SetMuted(ctx context.Context, h domain.HistoryKey, muted bool) error {
	chatID := h.ChatID
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	if h.TopicID > 0 {
		mute := domain.TopicUnmuted
		if muted {
			mute = domain.TopicMuted
		}
		was := o.state.ApplyTopicMute(chatID, h.TopicID, mute)
		if err := o.client.SetMuted(ctx, peer, h.TopicID, muted); err != nil {
			o.state.ApplyTopicMute(chatID, h.TopicID, was)
			return err
		}
		return nil
	}
	if _, changed := o.state.ApplyMute(chatID, muted); !changed {
		return nil
	}
	if err := o.client.SetMuted(ctx, peer, 0, muted); err != nil {
		o.state.ApplyMute(chatID, !muted)
		return err
	}
	return nil
}

// SetArchived moves a chat into or out of the Archive folder.
func (o *Owner) SetArchived(ctx context.Context, chatID int64, archived bool) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	if _, changed := o.state.ApplyArchived(chatID, archived); !changed {
		return nil
	}
	if err := o.client.SetArchived(ctx, peer, archived); err != nil {
		o.state.ApplyArchived(chatID, !archived)
		return err
	}
	return nil
}

// MarkRead reports messages up to maxID as read. A maxID of 0 means the whole
// chat, which is what the chat menu's "mark as read" asks for.
//
// Unlike the flag commands this is not optimistic: the pointer moves only after
// Telegram confirms, because an unread count running ahead of the server would
// be wrong in the direction a user notices.
func (o *Owner) MarkRead(ctx context.Context, h domain.HistoryKey, maxID int) error {
	chatID := h.ChatID
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	if chat, ok := o.state.Store().GetChat(chatID); ok && chat.IsForum {
		if chat.IsBot && h.TopicID == 0 && maxID > 0 {
			// Topicless read in a bot forum: mark only direct messages up to maxID.
			if err := o.client.MarkRead(ctx, peer, maxID); err != nil {
				return err
			}
			o.state.ApplyReadInbox(chatID, maxID)
			return nil
		}
		err := o.markForumRead(ctx, peer, h, maxID)
		if chat.IsBot && (h.TopicID == 0 || h.TopicID == domain.AllTopicsID) {
			if directErr := o.client.MarkRead(ctx, peer, maxID); directErr != nil && err == nil {
				err = directErr
			} else if directErr == nil {
				if maxID == 0 {
					o.state.ApplyChatRead(chatID)
				} else {
					o.state.ApplyReadInbox(chatID, maxID)
				}
			}
		}
		return err
	}
	if err := o.client.MarkRead(ctx, peer, maxID); err != nil {
		return err
	}
	if maxID == 0 {
		o.state.ApplyChatRead(chatID)
		return nil
	}
	o.state.ApplyReadInbox(chatID, maxID)
	return nil
}

// markForumRead reads in a forum, where reading is per topic: a topic is read
// up to maxID, or to its newest message when maxID is zero, and the forum as a
// whole is every topic with unread read to its newest (#275). Each topic read
// is its own request, so one refused leaves the others read.
func (o *Owner) markForumRead(ctx context.Context, peer domain.Peer, h domain.HistoryKey, maxID int) error {
	var topics []domain.Topic
	if h.TopicID > 0 {
		t, ok := o.state.Store().Topic(h.ChatID, h.TopicID)
		if !ok {
			t = domain.Topic{ChatID: h.ChatID, ID: h.TopicID}
		}
		if maxID != 0 {
			t.TopMessageID = maxID
		}
		topics = []domain.Topic{t}
	} else {
		for _, t := range o.state.Store().Topics(h.ChatID) {
			if t.UnreadCount <= 0 {
				continue
			}
			if maxID > 0 && t.ReadInboxMaxID >= maxID {
				continue
			}
			if maxID > 0 && (t.TopMessageID == 0 || maxID < t.TopMessageID) {
				t.TopMessageID = maxID
			}
			topics = append(topics, t)
		}
	}
	var first error
	for _, t := range topics {
		if err := o.client.ReadDiscussion(ctx, peer, t.ID, t.TopMessageID); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		o.state.ApplyTopicRead(h.ChatID, t.ID, t.TopMessageID)
	}
	return first
}

// ReadReactions marks every unread reaction in a chat as read.
//
// The badge clears before the request, unlike MarkRead: opening a chat is meant
// to drop its indicators at once (#142, #155), and waiting on the round-trip
// would leave them lit on a chat the user is already looking at. It is not
// restored on failure — the count cannot be reconstructed once cleared, and the
// next dialog-list sync is authoritative anyway.
//
// In a forum the reactions read are one topic's (#275).
func (o *Owner) ReadReactions(ctx context.Context, h domain.HistoryKey) error {
	peer, err := o.peer(h.ChatID)
	if err != nil {
		return err
	}
	topicID := h.TopicID
	if topicID > 0 {
		o.state.ApplyTopicReactionsRead(h.ChatID, topicID)
		return o.client.ReadReactions(ctx, peer, topicID)
	}
	if topicID == domain.AllTopicsID {
		for _, t := range o.state.Store().Topics(h.ChatID) {
			if t.UnreadReactionsCount > 0 {
				o.state.ApplyTopicReactionsRead(h.ChatID, t.ID)
			}
		}
		topicID = 0
	}
	o.state.ApplyReactionsRead(h.ChatID)
	return o.client.ReadReactions(ctx, peer, topicID)
}

// ReadMentions marks every unread mention in a chat as read, clearing the badge
// up front for the same reason as ReadReactions. In a forum the mentions read
// are one topic's.
func (o *Owner) ReadMentions(ctx context.Context, h domain.HistoryKey) error {
	peer, err := o.peer(h.ChatID)
	if err != nil {
		return err
	}
	topicID := h.TopicID
	if topicID > 0 {
		o.state.ApplyTopicMentionsRead(h.ChatID, topicID)
		return o.client.ReadMentions(ctx, peer, topicID)
	}
	if topicID == domain.AllTopicsID {
		for _, t := range o.state.Store().Topics(h.ChatID) {
			if t.UnreadMentionsCount > 0 {
				o.state.ApplyTopicMentionsRead(h.ChatID, t.ID)
			}
		}
		topicID = 0
	}
	o.state.ApplyMentionsRead(h.ChatID)
	return o.client.ReadMentions(ctx, peer, topicID)
}

// AddToFolder adds or removes a chat from a folder filter.
func (o *Owner) AddToFolder(ctx context.Context, filterID int, chatID int64, add bool) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	if _, changed := o.state.ApplyFolderMembership(filterID, chatID, add); !changed {
		return nil
	}
	if err := o.client.AddToFolder(ctx, filterID, peer, add); err != nil {
		o.state.ApplyFolderMembership(filterID, chatID, !add)
		return err
	}
	return nil
}

// SetUnreadMark sets or clears the manual unread mark on a chat.
func (o *Owner) SetUnreadMark(ctx context.Context, chatID int64, unread bool) error {
	peer, err := o.peer(chatID)
	if err != nil {
		return err
	}
	if _, changed := o.state.ApplyUnreadMark(chatID, unread); !changed {
		return nil
	}
	if err := o.client.MarkDialogUnread(ctx, peer, unread); err != nil {
		o.state.ApplyUnreadMark(chatID, !unread)
		return err
	}
	return nil
}
