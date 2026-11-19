package store

import (
	"encoding/json"
	"math"
	"sort"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"go.uber.org/zap"
)

// Topics returns a forum's topics in the order the official clients list
// them: pinned first, in the order they were pinned, then by newest message.
func (s *SQLiteStore) Topics(chatID int64) []domain.Topic {
	s.mu.RLock()
	defer s.mu.RUnlock()
	held := s.topics[chatID]
	out := make([]domain.Topic, 0, len(held))
	for _, t := range held {
		out = append(out, t)
	}
	ranks := s.pinRank[chatID]
	rank := func(t domain.Topic) int {
		if r, ok := ranks[t.ID]; ok && t.Pinned {
			return r
		}
		return math.MaxInt
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Pinned != b.Pinned {
			return a.Pinned
		}
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if da, db := topicActivity(a), topicActivity(b); !da.Equal(db) {
			return da.After(db)
		}
		return a.ID > b.ID
	})
	return out
}

// topicActivity is when a topic last had a message, for ordering.
func topicActivity(t domain.Topic) time.Time {
	if t.LastMessage == nil {
		return time.Time{}
	}
	return t.LastMessage.Date
}

func (s *SQLiteStore) Topic(chatID int64, topicID int) (domain.Topic, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.topics[chatID][topicID]
	return t, ok
}

// SetTopicsPage records the first page of a forum's topics. It is the only
// statement of the order topics were pinned in, which is their order on the
// page, and since every pinned topic is on it, a topic held from elsewhere that
// still thinks it is pinned no longer is.
func (s *SQLiteStore) SetTopicsPage(chatID int64, topics []domain.Topic) {
	s.mu.Lock()
	defer s.mu.Unlock()
	onPage := make(map[int]struct{}, len(topics))
	ranks := make(map[int]int)
	for _, t := range topics {
		onPage[t.ID] = struct{}{}
		if t.Pinned {
			ranks[t.ID] = len(ranks)
		}
		s.rebaseTopicLocked(t)
		s.putTopicLocked(t)
	}
	s.pinRank[chatID] = ranks
	for id, t := range s.topics[chatID] {
		if _, ok := onPage[id]; !ok && t.Pinned {
			t.Pinned = false
			s.putTopicLocked(t)
		}
	}
}

// UpdateTopics records topics read again by id. Such an answer says whether a
// topic is pinned but not where, so a topic keeps its place, and one pinned
// since the page was read goes after the others.
func (s *SQLiteStore) UpdateTopics(chatID int64, topics []domain.Topic) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ranks := s.pinRank[chatID]
	if ranks == nil {
		ranks = make(map[int]int)
		s.pinRank[chatID] = ranks
	}
	for _, t := range topics {
		if _, ok := ranks[t.ID]; t.Pinned && !ok {
			ranks[t.ID] = len(ranks)
		}
		s.rebaseTopicLocked(t)
		s.putTopicLocked(t)
	}
}

// ApplyIncomingTopic records what a newly received message means for its
// topic: the topic's newest message and, unless the account wrote it or has
// read past it, one more unread there. A topic nobody has described yet is held
// under its id alone, and known reports whether it was described, so the
// caller can ask Telegram about it. Outside a forum there is no topic to touch.
//
// The count is the last number Telegram stated plus the arrivals seen since,
// counted by id, so a second delivery of one message counts nothing.
func (s *SQLiteStore) ApplyIncomingTopic(msg domain.Message) (domain.HistoryKey, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[msg.ChatID]
	if !ok || !chat.IsForum {
		return domain.HistoryKey{ChatID: msg.ChatID}, true
	}
	h := chat.HistoryOf(msg)
	if h.TopicID == 0 {
		// In a bot forum, topicless direct messages live at TopicID 0 rather
		// than inside a named topic (#371).
		return h, true
	}
	t, known := s.topics[h.ChatID][h.TopicID]
	if !known {
		t = domain.Topic{ChatID: h.ChatID, ID: h.TopicID}
	}
	if msg.ID > t.TopMessageID {
		t.TopMessageID = msg.ID
		m := msg
		t.LastMessage = &m
	}
	if !msg.IsOut && msg.ID > t.ReadInboxMaxID {
		t.UnreadCount = counterFor(s.topicUnread, h, t.UnreadCount).add(msg.ID)
		if msg.Mentioned {
			t.UnreadMentionsCount = counterFor(s.topicMentions, h, t.UnreadMentionsCount).add(msg.ID)
		}
	}
	s.putTopicLocked(t)
	return h, known
}

// topicCounter counts arrivals on top of the last number Telegram stated, by
// id, so one message delivered twice counts once.
type topicCounter struct {
	base int
	seen map[int]struct{}
}

func (c *topicCounter) add(msgID int) int {
	if c.seen == nil {
		c.seen = make(map[int]struct{})
	}
	c.seen[msgID] = struct{}{}
	return c.base + len(c.seen)
}

// counterFor returns a topic's counter, starting one from the topic's current
// value when nothing was stated for it yet.
func counterFor(m map[domain.HistoryKey]*topicCounter, h domain.HistoryKey, current int) *topicCounter {
	c := m[h]
	if c == nil {
		c = &topicCounter{base: current}
		m[h] = c
	}
	return c
}

// rebaseTopicLocked makes what Telegram just stated about a topic the count
// arrivals are added to, forgetting the ones it already includes. Caller holds
// the lock.
func (s *SQLiteStore) rebaseTopicLocked(t domain.Topic) {
	h := domain.HistoryKey{ChatID: t.ChatID, TopicID: t.ID}
	s.topicUnread[h] = &topicCounter{base: t.UnreadCount}
	s.topicMentions[h] = &topicCounter{base: t.UnreadMentionsCount}
}

// forgetTopicCountsLocked drops a topic's counters. Caller holds the lock.
func (s *SQLiteStore) forgetTopicCountsLocked(h domain.HistoryKey) {
	delete(s.topicUnread, h)
	delete(s.topicMentions, h)
}

// SetTopicRead moves a topic's read pointer up to maxID. Read up to its newest
// message, nothing is unread there; read short of it, the count is Telegram's
// to state, and the read update that follows has the topic read again.
func (s *SQLiteStore) SetTopicRead(chatID int64, topicID, maxID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[chatID][topicID]
	if !ok {
		return
	}
	t.ReadInboxMaxID = max(t.ReadInboxMaxID, maxID)
	if maxID >= t.TopMessageID {
		t.UnreadCount = 0
	}
	s.topicUnread[domain.HistoryKey{ChatID: chatID, TopicID: topicID}] = &topicCounter{base: t.UnreadCount}
	s.putTopicLocked(t)
}

// SetTopicMentionsRead clears a topic's unread mentions.
func (s *SQLiteStore) SetTopicMentionsRead(chatID int64, topicID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[chatID][topicID]
	if !ok {
		return
	}
	t.UnreadMentionsCount = 0
	s.topicMentions[domain.HistoryKey{ChatID: chatID, TopicID: topicID}] = &topicCounter{}
	s.putTopicLocked(t)
}

// SetTopicReactionsRead clears a topic's unread reactions.
func (s *SQLiteStore) SetTopicReactionsRead(chatID int64, topicID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[chatID][topicID]
	if !ok {
		return
	}
	t.UnreadReactionsCount = 0
	s.putTopicLocked(t)
}

// SetTopicMute records a topic's own notification setting, reporting the one
// it replaced so a refused change can be undone.
func (s *SQLiteStore) SetTopicMute(chatID int64, topicID int, mute domain.TopicMute) domain.TopicMute {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[chatID][topicID]
	if !ok {
		t = domain.Topic{ChatID: chatID, ID: topicID}
	}
	was := t.Mute
	t.Mute = mute
	s.putTopicLocked(t)
	return was
}

// SetTopicDraft records a topic's unsent draft. A topic nothing is known about
// yet is held by its id, as an arriving message would hold it.
func (s *SQLiteStore) SetTopicDraft(chatID int64, topicID int, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.topics[chatID][topicID]
	if !ok {
		t = domain.Topic{ChatID: chatID, ID: topicID}
	}
	t.Draft = text
	s.putTopicLocked(t)
}

// RemoveTopics forgets topics Telegram reported deleted.
func (s *SQLiteStore) RemoveTopics(chatID int64, ids []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		delete(s.topics[chatID], id)
		delete(s.pinRank[chatID], id)
		s.forgetTopicCountsLocked(domain.HistoryKey{ChatID: chatID, TopicID: id})
		s.markTopicDeletedLocked(chatID, id)
	}
}

// putTopicLocked holds a topic and queues it for the next flush. Caller holds
// the lock.
func (s *SQLiteStore) putTopicLocked(t domain.Topic) {
	held := s.topics[t.ChatID]
	if held == nil {
		held = make(map[int]domain.Topic)
		s.topics[t.ChatID] = held
	}
	held[t.ID] = t
	if d := s.deletedTopics[t.ChatID]; d != nil {
		delete(d, t.ID)
	}
	dirty := s.dirtyTopics[t.ChatID]
	if dirty == nil {
		dirty = make(map[int]struct{})
		s.dirtyTopics[t.ChatID] = dirty
	}
	dirty[t.ID] = struct{}{}
}

// markTopicDeletedLocked queues a topic's row for deletion. Caller holds the
// lock.
func (s *SQLiteStore) markTopicDeletedLocked(chatID int64, topicID int) {
	if d := s.dirtyTopics[chatID]; d != nil {
		delete(d, topicID)
	}
	deleted := s.deletedTopics[chatID]
	if deleted == nil {
		deleted = make(map[int]struct{})
		s.deletedTopics[chatID] = deleted
	}
	deleted[topicID] = struct{}{}
}

// discardTopicsLocked forgets every topic of a chat. Its rows go with the
// chat's purge, which drops them alongside its messages. Caller holds the lock.
func (s *SQLiteStore) discardTopicsLocked(chatID int64) {
	for id := range s.topics[chatID] {
		s.forgetTopicCountsLocked(domain.HistoryKey{ChatID: chatID, TopicID: id})
	}
	delete(s.topics, chatID)
	delete(s.pinRank, chatID)
	delete(s.dirtyTopics, chatID)
	delete(s.deletedTopics, chatID)
}

type topicUpsert struct {
	chatID  int64
	topicID int
	pinRank int
	data    []byte
}

type topicDelete struct {
	chatID  int64
	topicID int
}

// snapshotTopicWritesLocked drains the queued topic writes. Caller holds the
// lock.
func (s *SQLiteStore) snapshotTopicWritesLocked() ([]topicUpsert, []topicDelete) {
	var upserts []topicUpsert
	for chatID, ids := range s.dirtyTopics {
		for id := range ids {
			t, ok := s.topics[chatID][id]
			if !ok {
				continue
			}
			b, err := json.Marshal(t)
			if err != nil {
				s.log.Error("marshal topic failed", zap.Int64("chat_id", chatID), zap.Int("topic_id", id), zap.Error(err))
				continue
			}
			rank, ok := s.pinRank[chatID][id]
			if !ok {
				rank = -1
			}
			upserts = append(upserts, topicUpsert{chatID: chatID, topicID: id, pinRank: rank, data: b})
		}
	}
	s.dirtyTopics = make(map[int64]map[int]struct{})
	var deletes []topicDelete
	for chatID, ids := range s.deletedTopics {
		for id := range ids {
			deletes = append(deletes, topicDelete{chatID: chatID, topicID: id})
		}
	}
	s.deletedTopics = make(map[int64]map[int]struct{})
	return upserts, deletes
}

// loadTopics reads every persisted topic into memory at startup. Topics are
// few next to messages, and the chat list needs all of them to badge a forum.
func (s *SQLiteStore) loadTopics() error {
	rows, err := s.db.Query(`SELECT chat_id, topic_id, pin_rank, data FROM topics`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			chatID  int64
			topicID int
			rank    int
			data    []byte
		)
		if err := rows.Scan(&chatID, &topicID, &rank, &data); err != nil {
			return err
		}
		var t domain.Topic
		if err := json.Unmarshal(data, &t); err != nil {
			s.log.Error("unmarshal topic failed", zap.Int64("chat_id", chatID), zap.Int("topic_id", topicID), zap.Error(err))
			continue
		}
		held := s.topics[chatID]
		if held == nil {
			held = make(map[int]domain.Topic)
			s.topics[chatID] = held
		}
		held[topicID] = t
		if rank >= 0 {
			ranks := s.pinRank[chatID]
			if ranks == nil {
				ranks = make(map[int]int)
				s.pinRank[chatID] = ranks
			}
			ranks[topicID] = rank
		}
	}
	return rows.Err()
}
