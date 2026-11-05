package store

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"go.uber.org/zap"
)

func (s *SQLiteStore) Messages(h domain.HistoryKey) []domain.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.messages[h]
	if msgs == nil {
		return nil
	}
	cp := make([]domain.Message, len(msgs))
	copy(cp, msgs)
	return cp
}

func (s *SQLiteStore) Message(chatID int64, msgID int) (domain.Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return domain.Message{}, false
	}
	return s.messages[h][i], true
}

// historyOfLocked names the history a message belongs to. A chat the store does
// not know is not a forum as far as it can tell, so the message joins the
// chat's one history. Caller holds the lock.
func (s *SQLiteStore) historyOfLocked(m domain.Message) domain.HistoryKey {
	chat, ok := s.chats[m.ChatID]
	if !ok {
		return domain.HistoryKey{ChatID: m.ChatID}
	}
	return chat.HistoryOf(m)
}

// byHistoryLocked splits a page into the histories its messages belong to,
// keeping each history's messages in the page's order. Caller holds the lock.
func (s *SQLiteStore) byHistoryLocked(msgs []domain.Message) map[domain.HistoryKey][]domain.Message {
	out := make(map[domain.HistoryKey][]domain.Message)
	for _, m := range msgs {
		h := s.historyOfLocked(m)
		out[h] = append(out[h], m)
	}
	return out
}

// chatHistoriesLocked lists the histories of a chat held in memory, in topic
// order so that whatever walks them does so the same way every time. Caller
// holds the lock.
func (s *SQLiteStore) chatHistoriesLocked(chatID int64) []domain.HistoryKey {
	topics := s.histories[chatID]
	out := make([]domain.HistoryKey, 0, len(topics))
	for t := range topics {
		out = append(out, domain.HistoryKey{ChatID: chatID, TopicID: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TopicID < out[j].TopicID })
	return out
}

// noteHistoryLocked records that a chat holds a history in memory. Caller holds
// the lock.
func (s *SQLiteStore) noteHistoryLocked(h domain.HistoryKey) {
	topics := s.histories[h.ChatID]
	if topics == nil {
		topics = make(map[int]struct{})
		s.histories[h.ChatID] = topics
	}
	topics[h.TopicID] = struct{}{}
}

// findLocked locates a held message by the chat and id Telegram addresses it
// by. Ids are unique within a chat, so whichever history holds it is the one;
// the caller need not know which topic the message is in. Caller holds the
// lock.
func (s *SQLiteStore) findLocked(chatID int64, msgID int) (domain.HistoryKey, int, bool) {
	for _, h := range s.chatHistoriesLocked(chatID) {
		for i := range s.messages[h] {
			if s.messages[h][i].ID == msgID {
				return h, i, true
			}
		}
	}
	return domain.HistoryKey{}, 0, false
}

// SetMessages replaces everything a chat holds with a page, laid out among the
// chat's histories. In a forum that is every topic at once: it is how a tail
// reload discards a forum whose gap lay across all of them.
func (s *SQLiteStore) SetMessages(chatID int64, msgs []domain.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.heldReactionsLocked(chatID)
	defer s.traceHeldReactionsLocked(chatID, "SetMessages", held)

	// What survives is decided across the chat rather than per history: a
	// message the page puts in another topic has moved, and its row must not be
	// deleted by the history it left.
	keep := make(map[int]struct{}, len(msgs))
	for _, m := range msgs {
		keep[m.ID] = struct{}{}
	}
	pages := s.byHistoryLocked(msgs)
	for _, h := range s.chatHistoriesLocked(chatID) {
		if _, kept := pages[h]; !kept {
			s.setMessagesLocked(h, nil, 0, keep)
		}
	}
	for h, page := range pages {
		s.setMessagesLocked(h, page, len(page), keep)
	}
}

// detachMovedLocked takes messages out of whichever other history of their
// chat holds them, for a page about to land in the history it names: a message
// lives in one topic, and the copy that names a new one says it moved. The row
// on disk stays; the next flush rewrites it under its new topic. Caller holds
// the lock.
func (s *SQLiteStore) detachMovedLocked(h domain.HistoryKey, msgs []domain.Message) {
	for _, m := range msgs {
		if m.ID <= 0 {
			continue
		}
		was, i, ok := s.findLocked(h.ChatID, m.ID)
		if !ok || was == h {
			continue
		}
		held := s.messages[was]
		s.messages[was] = append(held[:i:i], held[i+1:]...)
	}
}

// MergeMessages merges a fetched page into one history and reports how many
// messages it added. Reading the held history, merging and storing the result
// happen under one hold of the lock, which is what keeps a message arriving
// mid-fetch from being written back out of existence: read and write as two
// calls leave a window where the arrival lands between them and is lost when
// the merged page replaces the slice.
//
// A page that adds nothing still lands, because a message can come back edited
// without changing the count. The caller decides what to do about a zero, and
// what it is told is what the count changed by, not whether anything did.
//
// Only the history asked for is filled deeper. A message in the page that
// belongs elsewhere joins its own history the way a repair would, without
// raising what that history costs to hold.
func (s *SQLiteStore) MergeMessages(h domain.HistoryKey, msgs []domain.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.heldReactionsLocked(h.ChatID)
	defer s.traceHeldReactionsLocked(h.ChatID, "MergeMessages", held)

	added := 0
	for key, page := range s.byHistoryLocked(msgs) {
		s.detachMovedLocked(key, page)
		before := s.messages[key]
		merged := domain.MergeMessages(before, page)
		floor := s.msgFloor[key]
		if key == h {
			floor = len(merged)
		}
		s.setMessagesLocked(key, merged, floor, nil)
		added += len(merged) - len(before)
	}
	return added
}

// RepairMessages merges a page in the way MergeMessages does, laying it out
// among the chat's histories, and leaves each of them no deeper than it found
// it: the cap applies as if the page had never come, trimming the oldest to
// make room.
//
// The floor exists to protect a scrollback somebody scrolled to, so that an
// arriving message cannot cap it back down. A page that closes a gap was asked
// for by nobody, and letting it raise the floor would turn a repair into a
// permanent rise in what the chat costs to hold, once per hole.
func (s *SQLiteStore) RepairMessages(chatID int64, msgs []domain.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	held := s.heldReactionsLocked(chatID)
	defer s.traceHeldReactionsLocked(chatID, "RepairMessages", held)

	added := 0
	for h, page := range s.byHistoryLocked(msgs) {
		s.detachMovedLocked(h, page)
		before := s.messages[h]
		merged := domain.MergeMessages(before, page)
		s.setMessagesLocked(h, merged, s.msgFloor[h], nil)
		// What the page brought, counted before the cap trims the other end: a
		// repair that adds fifty and pushes fifty older ones out still changed
		// every window looking at it.
		added += len(merged) - len(before)
	}
	return added
}

// setMessagesLocked replaces one history with msgs, taking a copy: the slice
// handed in may be the caller's own, or the store's own held slice come back
// through a merge. floor is how deep this write claims the history was filled,
// which is what the cap will not trim below. keep names the messages whose rows
// survive the write wherever they now live; nil means the ones in msgs. Caller
// holds the lock.
func (s *SQLiteStore) setMessagesLocked(h domain.HistoryKey, msgs []domain.Message, floor int, keep map[int]struct{}) {
	cp := make([]domain.Message, len(msgs))
	copy(cp, msgs)

	newIDs := keep
	if newIDs == nil {
		newIDs = make(map[int]struct{}, len(cp))
		for _, m := range cp {
			newIDs[m.ID] = struct{}{}
		}
	}
	// Re-index this history: drop entries for the replaced messages and mark rows
	// the new set no longer contains for deletion on disk.
	for _, m := range s.messages[h] {
		delete(s.msgChat, m.ID)
		if _, keep := newIDs[m.ID]; !keep {
			s.markMsgDeletedLocked(h.ChatID, m.ID)
		}
	}
	s.messages[h] = cp
	s.noteHistoryLocked(h)
	// Remember how deep this history was filled, so an arriving message does not
	// cap the scrollback back down (see capMessagesLocked).
	s.msgFloor[h] = floor
	s.capMessagesLocked(h)
	if chat, ok := s.chats[h.ChatID]; ok && sharedPtsBox(chat.Peer) {
		for _, m := range s.messages[h] {
			s.msgChat[m.ID] = h.ChatID
		}
	}
	for _, m := range s.messages[h] {
		s.markMsgDirtyLocked(h.ChatID, m.ID)
	}
}

// markMsgDirtyLocked queues an upsert of (chatID, msgID) for the next flush.
// Optimistic sentinel messages (negative ids) are session-only and never
// persisted. Caller holds the lock.
func (s *SQLiteStore) markMsgDirtyLocked(chatID int64, msgID int) {
	if msgID <= 0 {
		return
	}
	if d := s.deletedMsgs[chatID]; d != nil {
		delete(d, msgID)
	}
	m := s.dirtyMsgs[chatID]
	if m == nil {
		m = make(map[int]struct{})
		s.dirtyMsgs[chatID] = m
	}
	m[msgID] = struct{}{}
}

// markMsgDeletedLocked queues a delete of (chatID, msgID) for the next flush.
// Caller holds the lock.
func (s *SQLiteStore) markMsgDeletedLocked(chatID int64, msgID int) {
	if msgID <= 0 {
		return
	}
	if d := s.dirtyMsgs[chatID]; d != nil {
		delete(d, msgID)
	}
	m := s.deletedMsgs[chatID]
	if m == nil {
		m = make(map[int]struct{})
		s.deletedMsgs[chatID] = m
	}
	m[msgID] = struct{}{}
}

type msgUpsert struct {
	chatID  int64
	msgID   int
	topicID int
	date    int64
	data    []byte
}

type msgDelete struct {
	chatID int64
	msgID  int
}

// rowWrites is one flush's worth of message and topic rows. purges run first:
// they drop rows written under a shape the chat no longer has, and an upsert
// queued after the purge belongs to the new shape.
type rowWrites struct {
	purges       []int64
	upserts      []msgUpsert
	deletes      []msgDelete
	topicUpserts []topicUpsert
	topicDeletes []topicDelete
}

// snapshotMessageWritesLocked drains the purge, dirty and deleted message sets
// into flat slices, reading upsert payloads from the current in-memory
// messages. Caller holds the lock.
func (s *SQLiteStore) snapshotMessageWritesLocked() rowWrites {
	var purges []int64
	for chatID := range s.purgeMsgs {
		purges = append(purges, chatID)
		s.purgingMsgs[chatID] = struct{}{}
	}
	s.purgeMsgs = make(map[int64]struct{})

	var upserts []msgUpsert
	for chatID, ids := range s.dirtyMsgs {
		for _, h := range s.chatHistoriesLocked(chatID) {
			for _, m := range s.messages[h] {
				if _, ok := ids[m.ID]; !ok {
					continue
				}
				b, err := json.Marshal(m)
				if err != nil {
					s.log.Error("marshal message failed", zap.Int64("chat_id", chatID), zap.Int("msg_id", m.ID), zap.Error(err))
					continue
				}
				upserts = append(upserts, msgUpsert{chatID: chatID, msgID: m.ID, topicID: h.TopicID, date: m.Date.Unix(), data: b})
			}
		}
	}
	s.dirtyMsgs = make(map[int64]map[int]struct{})

	var deletes []msgDelete
	for chatID, ids := range s.deletedMsgs {
		inFlight := s.deletingMsgs[chatID]
		if inFlight == nil {
			inFlight = make(map[int]struct{}, len(ids))
			s.deletingMsgs[chatID] = inFlight
		}
		for msgID := range ids {
			deletes = append(deletes, msgDelete{chatID: chatID, msgID: msgID})
			inFlight[msgID] = struct{}{}
		}
	}
	s.deletedMsgs = make(map[int64]map[int]struct{})
	topicUpserts, topicDeletes := s.snapshotTopicWritesLocked()
	return rowWrites{purges: purges, upserts: upserts, deletes: deletes, topicUpserts: topicUpserts, topicDeletes: topicDeletes}
}

// clearDeletesInFlight forgets purges and deletes whose flush transaction has
// finished.
func (s *SQLiteStore) clearDeletesInFlight(w rowWrites) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chatID := range w.purges {
		delete(s.purgingMsgs, chatID)
	}
	for _, d := range w.deletes {
		ids := s.deletingMsgs[d.chatID]
		delete(ids, d.msgID)
		if len(ids) == 0 {
			delete(s.deletingMsgs, d.chatID)
		}
	}
}

// msgDeletePendingLocked reports whether a message is queued for deletion or has
// a delete in flight, so a read from disk must skip its row. Caller holds the lock.
func (s *SQLiteStore) msgDeletePendingLocked(chatID int64, msgID int) bool {
	if _, ok := s.deletedMsgs[chatID][msgID]; ok {
		return true
	}
	_, ok := s.deletingMsgs[chatID][msgID]
	return ok
}

// purgePendingLocked reports whether a chat's rows are queued to be purged or
// have a purge in flight, so nothing may be read back from its rows on disk.
// Caller holds the lock.
func (s *SQLiteStore) purgePendingLocked(chatID int64) bool {
	if _, ok := s.purgeMsgs[chatID]; ok {
		return true
	}
	_, ok := s.purgingMsgs[chatID]
	return ok
}

// discardChatLocked drops everything a chat holds, in memory and on disk: every
// history, its floor and its loaded mark, and the rows of histories this
// session never opened. Caller holds the lock.
func (s *SQLiteStore) discardChatLocked(chatID int64) {
	for _, h := range s.chatHistoriesLocked(chatID) {
		for _, m := range s.messages[h] {
			delete(s.msgChat, m.ID)
		}
		delete(s.messages, h)
		delete(s.msgFloor, h)
	}
	delete(s.histories, chatID)
	for h := range s.loaded {
		if h.ChatID == chatID {
			delete(s.loaded, h)
		}
	}
	// The purge covers every row, so queued per-message writes for the old
	// shape have nothing left to do.
	delete(s.dirtyMsgs, chatID)
	delete(s.deletedMsgs, chatID)
	s.discardTopicsLocked(chatID)
	s.purgeMsgs[chatID] = struct{}{}
}

// flushMessageRows applies queued purges, upserts and deletes of messages and
// topics in one transaction. Runs off-lock. Logs errors; the Store interface
// does not propagate them.
func (s *SQLiteStore) flushMessageRows(w rowWrites) {
	if len(w.purges) == 0 && len(w.upserts) == 0 && len(w.deletes) == 0 &&
		len(w.topicUpserts) == 0 && len(w.topicDeletes) == 0 {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		s.log.Error("begin message flush failed", zap.Error(err))
		return
	}
	for _, chatID := range w.purges {
		for _, q := range []string{`DELETE FROM messages WHERE chat_id = ?`, `DELETE FROM topics WHERE chat_id = ?`} {
			if _, err := tx.Exec(q, chatID); err != nil {
				_ = tx.Rollback()
				s.log.Error("purge chat rows failed", zap.Int64("chat_id", chatID), zap.Error(err))
				return
			}
		}
	}
	for _, u := range w.topicUpserts {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO topics(chat_id, topic_id, pin_rank, data) VALUES (?, ?, ?, ?)`,
			u.chatID, u.topicID, u.pinRank, u.data); err != nil {
			_ = tx.Rollback()
			s.log.Error("upsert topic failed", zap.Int64("chat_id", u.chatID), zap.Int("topic_id", u.topicID), zap.Error(err))
			return
		}
	}
	for _, d := range w.topicDeletes {
		if _, err := tx.Exec(`DELETE FROM topics WHERE chat_id = ? AND topic_id = ?`, d.chatID, d.topicID); err != nil {
			_ = tx.Rollback()
			s.log.Error("delete topic failed", zap.Int64("chat_id", d.chatID), zap.Int("topic_id", d.topicID), zap.Error(err))
			return
		}
	}
	for _, u := range w.upserts {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO messages(chat_id, msg_id, topic_id, date, data) VALUES (?, ?, ?, ?, ?)`,
			u.chatID, u.msgID, u.topicID, u.date, u.data); err != nil {
			_ = tx.Rollback()
			s.log.Error("upsert message failed", zap.Int64("chat_id", u.chatID), zap.Int("msg_id", u.msgID), zap.Error(err))
			return
		}
	}
	for _, d := range w.deletes {
		if _, err := tx.Exec(`DELETE FROM messages WHERE chat_id = ? AND msg_id = ?`, d.chatID, d.msgID); err != nil {
			_ = tx.Rollback()
			s.log.Error("delete message failed", zap.Int64("chat_id", d.chatID), zap.Int("msg_id", d.msgID), zap.Error(err))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.log.Error("commit message flush failed", zap.Error(err))
	}
}

// mergeMessagesByID unions a history's on-disk tail (disk) with its in-memory
// messages (mem), keeping the in-memory copy on an id collision (live updates
// are fresher) and returning the result sorted by (date, id).
func mergeMessagesByID(disk, mem []domain.Message) []domain.Message {
	seen := make(map[int]struct{}, len(mem))
	for _, m := range mem {
		seen[m.ID] = struct{}{}
	}
	out := make([]domain.Message, 0, len(disk)+len(mem))
	for _, d := range disk {
		if _, ok := seen[d.ID]; !ok {
			out = append(out, d)
		}
	}
	out = append(out, mem...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// LoadMessages loads a history's persisted tail into memory on first open,
// merging it with any messages already held (live updates). Idempotent per
// history; a pure read that never queues a write. See issue #139.
func (s *SQLiteStore) LoadMessages(h domain.HistoryKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded[h] {
		return
	}
	s.loaded[h] = true
	// Every row a chat awaiting a purge has on disk was written under the shape
	// it no longer has; what it holds now is all in memory.
	if s.purgePendingLocked(h.ChatID) {
		s.noteHistoryLocked(h)
		return
	}

	rows, err := s.db.Query(`SELECT data FROM messages WHERE chat_id = ? AND topic_id = ? ORDER BY date, msg_id`, h.ChatID, h.TopicID)
	if err != nil {
		s.log.Error("load messages failed", zap.Int64("chat_id", h.ChatID), zap.Int("topic_id", h.TopicID), zap.Error(err))
		return
	}
	defer func() { _ = rows.Close() }()

	var disk []domain.Message
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			s.log.Error("scan message failed", zap.Int64("chat_id", h.ChatID), zap.Error(err))
			return
		}
		var m domain.Message
		if err := json.Unmarshal(data, &m); err != nil {
			s.log.Error("unmarshal message failed", zap.Int64("chat_id", h.ChatID), zap.Error(err))
			continue
		}
		// A delete that has not reached disk yet already happened as far as the
		// rest of the app is concerned; its row must not come back on open.
		if s.msgDeletePendingLocked(h.ChatID, m.ID) {
			continue
		}
		disk = append(disk, m)
	}
	if err := rows.Err(); err != nil {
		s.log.Error("iterate messages failed", zap.Int64("chat_id", h.ChatID), zap.Error(err))
		return
	}

	if mem := s.messages[h]; len(mem) == 0 {
		s.messages[h] = disk
	} else {
		s.messages[h] = mergeMessagesByID(disk, mem)
	}
	s.noteHistoryLocked(h)
	s.capMessagesLocked(h)
	if chat, ok := s.chats[h.ChatID]; ok && sharedPtsBox(chat.Peer) {
		for _, m := range s.messages[h] {
			s.msgChat[m.ID] = h.ChatID
		}
	}
}

// capMessagesLocked trims a history to the newest MaxMessagesPerChat, dropping
// the oldest from the front and clearing their index entries. Caller holds the
// lock. See issue #73.
//
// The cap bounds what arriving messages accumulate; it never trims below what
// was last set outright. Scrolling far into history sets a longer tail on
// purpose, and one incoming message must not throw that scrollback away — the
// view is built from what the store holds, so the trim would be visible.
func (s *SQLiteStore) capMessagesLocked(h domain.HistoryKey) {
	msgs := s.messages[h]
	limit := MaxMessagesPerChat
	if floor := s.msgFloor[h]; floor > limit {
		limit = floor
	}
	if len(msgs) <= limit {
		return
	}
	drop := len(msgs) - limit
	for _, m := range msgs[:drop] {
		delete(s.msgChat, m.ID)
		s.markMsgDeletedLocked(h.ChatID, m.ID)
	}
	s.messages[h] = msgs[drop:]
}

// BumpChatLastMessage updates a chat's last-message preview and moves it up in
// the list, WITHOUT appending to the chat's message slice. It optimistically
// surfaces a chat that just received an outgoing message sent from elsewhere
// (e.g. a forward target), whose full message arrives later via the update
// stream (or on next open). No-op if the chat is unknown.
func (s *SQLiteStore) BumpChatLastMessage(chatID int64, msg domain.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	chat, ok := s.chats[chatID]
	if !ok {
		return
	}
	m := msg
	chat.LastMessage = &m
	s.chats[chatID] = chat
	s.orderDirty = true // newer last-message moves the chat in the list
	s.markDirtyLocked(chatID)
}

// AppendMessage adds a message to its history, or replaces it when that ID is
// already held. The same message legitimately arrives twice — once in the reply
// to the RPC that created it, once from a later getDifference — and the newer
// copy may carry more (a resolved sender name, filled-in media refs), so it wins
// in place rather than appearing a second time. Sentinel IDs are negative and
// unique, so optimistic messages never collide here.
func (s *SQLiteStore) AppendMessage(msg domain.Message) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.historyOfLocked(msg)
	if msg.ID > 0 {
		if was, i, ok := s.findLocked(msg.ChatID, msg.ID); ok {
			s.traceReactionChangeLocked(msg.ChatID, msg.ID, "AppendMessage", s.messages[was][i].Reactions, msg.Reactions)
			if was == h {
				s.messages[h][i] = msg
			} else {
				// Moved to another topic: it takes its place in the history it
				// joined, which need not be the end, since it may be old.
				s.detachMovedLocked(h, []domain.Message{msg})
				s.messages[h] = domain.MergeMessages(s.messages[h], []domain.Message{msg})
				s.noteHistoryLocked(h)
			}
			s.markMsgDirtyLocked(msg.ChatID, msg.ID)
			return false
		}
	}
	s.messages[h] = append(s.messages[h], msg)
	s.noteHistoryLocked(h)
	// A history holding more than the cap is holding a scrollback someone loaded
	// on purpose. An arriving message adds to it rather than pushing the oldest
	// out.
	if s.msgFloor[h] >= MaxMessagesPerChat {
		s.msgFloor[h]++
	}
	s.markMsgDirtyLocked(msg.ChatID, msg.ID)
	if chat, ok := s.chats[msg.ChatID]; ok {
		m := msg
		chat.LastMessage = &m
		s.chats[msg.ChatID] = chat
		s.orderDirty = true // newer last-message moves the chat in the list
		if sharedPtsBox(chat.Peer) {
			s.msgChat[msg.ID] = msg.ChatID
		}
		s.markDirtyLocked(msg.ChatID) // write-behind: last-message persists on flush
	}
	s.capMessagesLocked(h)
	return true
}

// AdvanceAppliedPosition compares and records under one lock, so the decision
// and the record cannot disagree.
func (s *SQLiteStore) AdvanceAppliedPosition(chatID int64, msgID, position int) bool {
	if position == 0 {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return true
	}
	if position <= s.messages[h][i].AppliedPosition {
		return false
	}
	s.messages[h][i].AppliedPosition = position
	s.markMsgDirtyLocked(chatID, msgID)
	return true
}

// UpdateMessageText replaces a message's text and its entities together. They
// must move as a unit: entity offsets address the text they were parsed from,
// so keeping the old ones would leave them pointing at characters that changed.
//
// The edit marker is not touched here. Whether an edit earns the "edited" label
// is Telegram's to say, and it says so separately (#269).
func (s *SQLiteStore) UpdateMessageText(chatID int64, msgID int, text string, entities []domain.MessageEntity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	m := &s.messages[h][i]
	// A caption can legitimately be removed, and a broken delivery looks exactly
	// the same from here. Rather than guess, say so and carry on: the counts and
	// the position are enough to tell the two apart in a log, and the text
	// itself never goes into one (#80).
	if text == "" && m.Text != "" {
		s.log.Warn("stored text replaced by an empty one",
			zap.Int64("chat_id", chatID),
			zap.Int("msg_id", msgID),
			zap.Int("was_len", len([]rune(m.Text))),
			zap.Int("now_len", 0),
			zap.Bool("media", m.Media != nil || m.Photo != nil || m.Document != nil),
			zap.Int("position", m.AppliedPosition),
		)
	}
	m.Text = text
	cp := make([]domain.MessageEntity, len(entities))
	copy(cp, entities)
	m.Entities = cp
	s.markMsgDirtyLocked(chatID, msgID)
}

// UpdateMessageRich replaces a message's rich block content. A nil clears it:
// an edit carries the message's whole current state, so a new version without
// blocks must not keep drawing the old ones.
func (s *SQLiteStore) UpdateMessageRich(chatID int64, msgID int, rich *domain.RichMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	s.messages[h][i].Rich = rich
	s.markMsgDirtyLocked(chatID, msgID)
}

// MarkMessageEdited records the edit time and whether the label is hidden.
func (s *SQLiteStore) MarkMessageEdited(chatID int64, msgID int, editDate time.Time, hidden bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	t := editDate
	s.messages[h][i].EditDate = &t
	s.messages[h][i].EditHidden = hidden
	s.markMsgDirtyLocked(chatID, msgID)
}

func (s *SQLiteStore) UpdateMessageReactions(chatID int64, msgID int, reactions []domain.Reaction, via string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	cp := make([]domain.Reaction, len(reactions))
	copy(cp, reactions)
	s.traceReactionChangeLocked(chatID, msgID, via, s.messages[h][i].Reactions, cp)
	s.messages[h][i].Reactions = cp
	s.markMsgDirtyLocked(chatID, msgID)
}

// UpdateMessageMedia replaces the photo/document refs of a cached message. A nil
// ref leaves that field unchanged.
func (s *SQLiteStore) UpdateMessageMedia(chatID int64, msgID int, photo *domain.PhotoRef, document *domain.DocumentRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	if photo != nil {
		s.messages[h][i].Photo = photo
	}
	if document != nil {
		s.messages[h][i].Document = document
	}
	s.markMsgDirtyLocked(chatID, msgID)
}

// ReplaceMessage overwrites a stored message with msg, fields and all. Unlike
// the field-wise updates it can clear EditDate, which a rolled-back edit must
// do: the message was never edited (#118).
func (s *SQLiteStore) ReplaceMessage(chatID int64, msg domain.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msg.ID)
	if !ok {
		return
	}
	s.traceReactionChangeLocked(chatID, msg.ID, "ReplaceMessage", s.messages[h][i].Reactions, msg.Reactions)
	s.messages[h][i] = msg
	s.markMsgDirtyLocked(chatID, msg.ID)
}

func (s *SQLiteStore) RemoveMessage(chatID int64, msgID int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, i, ok := s.findLocked(chatID, msgID)
	if !ok {
		return
	}
	msgs := s.messages[h]
	s.messages[h] = append(msgs[:i], msgs[i+1:]...)
	delete(s.msgChat, msgID)
	s.markMsgDeletedLocked(chatID, msgID)
}

func (s *SQLiteStore) RemoveMessages(chatID int64, msgIDs []int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeMessagesLocked(chatID, msgIDs)
}

// removeMessagesLocked drops the given message IDs from one chat, whichever of
// its histories holds them, from the msgChat index and from the database. IDs
// the chat does not hold in memory are queued for deletion all the same: a chat
// that was not opened this session holds nothing in memory, and leaving its row
// behind resurrects the message on the next open. Caller holds the lock.
func (s *SQLiteStore) removeMessagesLocked(chatID int64, msgIDs []int) {
	toRemove := make(map[int]struct{}, len(msgIDs))
	for _, id := range msgIDs {
		toRemove[id] = struct{}{}
		// Only drop the index entry if it points here: message IDs are unique
		// within the shared pts box, but a channel numbers its own and may reuse
		// a number that belongs to a private chat.
		if cid, ok := s.msgChat[id]; ok && cid == chatID {
			delete(s.msgChat, id)
		}
		s.markMsgDeletedLocked(chatID, id)
	}
	for _, h := range s.chatHistoriesLocked(chatID) {
		msgs := s.messages[h]
		if len(msgs) == 0 {
			continue
		}
		kept := msgs[:0]
		for _, m := range msgs {
			if _, remove := toRemove[m.ID]; remove {
				continue
			}
			kept = append(kept, m)
		}
		s.messages[h] = kept
	}
}

// resolveChatsByMsgIDLocked finds the owning chat of message IDs the in-memory
// index does not cover, by looking them up on disk. Only shared-pts-box chats
// count: IDs are globally unique there, whereas a channel's ID space is its own
// and could collide by number. Caller holds the lock.
func (s *SQLiteStore) resolveChatsByMsgIDLocked(msgIDs []int) map[int64][]int {
	if len(msgIDs) == 0 {
		return nil
	}
	args := make([]any, 0, len(msgIDs))
	for _, id := range msgIDs {
		args = append(args, id)
	}
	q := `SELECT chat_id, msg_id FROM messages WHERE msg_id IN (?` + strings.Repeat(",?", len(msgIDs)-1) + `)`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		s.log.Error("resolve chats by message id failed", zap.Error(err))
		return nil
	}
	defer func() { _ = rows.Close() }()

	byChat := make(map[int64][]int)
	for rows.Next() {
		var chatID int64
		var msgID int
		if err := rows.Scan(&chatID, &msgID); err != nil {
			s.log.Error("scan message owner failed", zap.Error(err))
			return nil
		}
		if chat, ok := s.chats[chatID]; !ok || !sharedPtsBox(chat.Peer) {
			continue
		}
		byChat[chatID] = append(byChat[chatID], msgID)
	}
	if err := rows.Err(); err != nil {
		s.log.Error("iterate message owners failed", zap.Error(err))
		return nil
	}
	return byChat
}

// RemoveMessagesByID resolves each message ID to its owning chat and removes it
// there, returning the affected chat IDs. Used for the Telegram non-channel
// delete that carries message IDs but no peer context (issue #72). The in-memory
// index only covers chats touched this session, so the rest are resolved on disk.
func (s *SQLiteStore) RemoveMessagesByID(msgIDs []int) []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	byChat := make(map[int64][]int)
	var unindexed []int
	for _, id := range msgIDs {
		if cid, ok := s.msgChat[id]; ok {
			byChat[cid] = append(byChat[cid], id)
			continue
		}
		unindexed = append(unindexed, id)
	}
	for cid, ids := range s.resolveChatsByMsgIDLocked(unindexed) {
		byChat[cid] = append(byChat[cid], ids...)
	}
	affected := make([]int64, 0, len(byChat))
	for cid, ids := range byChat {
		s.removeMessagesLocked(cid, ids)
		affected = append(affected, cid)
	}
	return affected
}
