package project

import "github.com/sorokin-vladimir/tele/internal/domain"

// TopicRow is one rendered topic-list row: what a chat-list row shows about a
// chat, for a topic (#275).
type TopicRow struct {
	ID    int
	Title string
	// Unread, Mentions and Reactions are the topic's own counts.
	Unread    int
	Mentions  int
	Reactions int
	Pinned    bool
	Closed    bool
	// Hidden is General folded away by the forum's admins.
	Hidden bool
	// Muted is whether the topic is muted, by its own setting or by following
	// its forum's.
	Muted bool
}

// TopicListContents is everything a topic-list subscription currently shows.
type TopicListContents struct {
	ChatID int64
	Offset int
	Total  int
	Rows   []TopicRow
}

// BuildTopicList slices the window out of a forum's topics, prepending a
// virtual "All" entry that interleaves messages from all topics (and any
// topicless messages). Order comes from the reader; this function never sorts.
func BuildTopicList(r Reader, w TopicListWindow) TopicListContents {
	topics := TopicRows(r, w.ChatID)
	chat, hasChat := r.GetChat(w.ChatID)
	all := make([]TopicRow, 0, len(topics)+1)
	if (hasChat && chat.IsForum) || len(topics) > 0 {
		all = append(all, allTopicsRow(chat, topics))
	}
	all = append(all, topics...)
	out := TopicListContents{ChatID: w.ChatID, Offset: w.Offset, Total: len(all)}
	start := min(max(w.Offset, 0), len(all))
	end := min(start+w.Limit, len(all))
	out.Rows = all[start:end]
	return out
}

func allTopicsRow(chat domain.Chat, topics []TopicRow) TopicRow {
	unread, mentions, reactions := 0, 0, 0
	for _, t := range topics {
		unread += t.Unread
		mentions += t.Mentions
		reactions += t.Reactions
	}
	if chat.UnreadCount > 0 && unread == 0 {
		unread = chat.UnreadCount
	}
	mentions = max(mentions, chat.UnreadMentionsCount)
	reactions = max(reactions, chat.UnreadReactionsCount)
	return TopicRow{
		ID:        domain.AllTopicsID,
		Title:     "All",
		Unread:    unread,
		Mentions:  mentions,
		Reactions: reactions,
		Muted:     chat.IsMuted,
	}
}

// TopicRows is a forum's whole topic list as rows, in the reader's order. A
// query answers with it too, so a client handles one shape of topic whether it
// came from a subscription or a one-off answer.
func TopicRows(r Reader, chatID int64) []TopicRow {
	all := r.Topics(chatID)
	chat, _ := r.GetChat(chatID)
	out := make([]TopicRow, 0, len(all))
	for _, t := range all {
		out = append(out, topicRow(t, chat.IsMuted))
	}
	return out
}

func topicRow(t domain.Topic, forumMuted bool) TopicRow {
	return TopicRow{
		ID:        t.ID,
		Title:     t.Title,
		Unread:    t.UnreadCount,
		Mentions:  t.UnreadMentionsCount,
		Reactions: t.UnreadReactionsCount,
		Pinned:    t.Pinned,
		Closed:    t.Closed,
		Hidden:    t.Hidden,
		Muted:     t.MutedIn(forumMuted),
	}
}

// TopicListDeltaKind names what can happen to a topic-list subscription. As
// with the chat list, anything that moves a row is a Reset.
type TopicListDeltaKind int

const (
	// TopicListReset replaces the window's whole contents, and is the first
	// delta of every subscription.
	TopicListReset TopicListDeltaKind = iota
	// TopicListRow replaces one row in place, found by its id.
	TopicListRow
)

type TopicListDelta struct {
	Kind     TopicListDeltaKind
	Contents TopicListContents
	Row      TopicRow
}

// DiffTopicList turns a pair of successive contents into the deltas that carry
// the difference.
func DiffTopicList(prev, next TopicListContents) []TopicListDelta {
	if prev.ChatID != next.ChatID || prev.Offset != next.Offset || prev.Total != next.Total || !sameTopicIDs(prev.Rows, next.Rows) {
		return []TopicListDelta{{Kind: TopicListReset, Contents: next}}
	}
	var out []TopicListDelta
	for i, row := range next.Rows {
		if row != prev.Rows[i] {
			out = append(out, TopicListDelta{Kind: TopicListRow, Row: row})
		}
	}
	return out
}

func sameTopicIDs(a, b []TopicRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}
