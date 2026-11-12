package store

import (
	"sort"

	"github.com/sorokin-vladimir/tele/internal/domain"
)

// span is where one run of a history lies: the ids of its two ends, and which
// ends of the history it reaches. A held message belongs to the span whose
// range holds its id. The range, not the messages, is what records the run, so
// a message deleted at either end leaves the run where it was.
type span struct {
	lo, hi      int
	atTail      bool
	atBeginning bool
}

// Shown is what one window shows of a history: the message it is anchored on,
// zero for the newest, and how many messages older than it the window asks
// for. It is what the window asks for rather than what it was last built with,
// so a page fetched to fill it is shown the moment it lands.
//
// How far a window reaches newer than its anchor does not matter to the cap: it
// trims only the old end of the tail run, and keeps a run far back whole.
type Shown struct {
	Anchor int
	Before int
}

func (a span) overlaps(b span) bool { return a.lo <= b.hi && b.lo <= a.hi }

func (a span) holds(id int) bool { return a.lo <= id && id <= a.hi }

// Runs returns a history's runs, oldest first, each with the messages it holds.
func (s *SQLiteStore) Runs(h domain.HistoryKey) []domain.Run {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.runsLocked(h)
}

func (s *SQLiteStore) runsLocked(h domain.HistoryKey) []domain.Run {
	msgs := s.messages[h]
	spans := s.runs[h]
	if len(spans) == 0 {
		if len(msgs) == 0 {
			return nil
		}
		// Messages with no run recorded are the one stretch the store kept
		// before it knew of runs: it ends at the tail.
		cp := make([]domain.Message, len(msgs))
		copy(cp, msgs)
		return []domain.Run{{Messages: cp, AtTail: true}}
	}
	out := make([]domain.Run, len(spans))
	tail := -1
	for i, sp := range spans {
		out[i] = domain.Run{AtTail: sp.atTail, AtBeginning: sp.atBeginning}
		if sp.atTail {
			tail = i
		}
	}
	for _, m := range msgs {
		if m.ID <= 0 {
			// A send not confirmed yet has no id of its own and is newer than
			// everything: it sits on the tail run, which it starts if need be.
			if tail < 0 {
				out = append(out, domain.Run{AtTail: true})
				tail = len(out) - 1
			}
			out[tail].Messages = append(out[tail].Messages, m)
			continue
		}
		if i := spanOf(spans, m.ID); i >= 0 {
			out[i].Messages = append(out[i].Messages, m)
		}
	}
	// A run whose every message was deleted keeps its place, so a page that
	// reaches it still joins, but there is nothing in it to show.
	shown := out[:0]
	for _, r := range out {
		if len(r.Messages) > 0 {
			shown = append(shown, r)
		}
	}
	return shown
}

// landOnTailLocked records messages newer than the tail on the tail run,
// starting one when the history holds none: a live message, or a page that
// closes a gap. Caller holds the lock.
func (s *SQLiteStore) landOnTailLocked(h domain.HistoryKey, msgs []domain.Message) {
	add, ok := pageSpan(msgs)
	if !ok || len(s.runs[h]) == 0 {
		return
	}
	for i, sp := range s.runs[h] {
		if sp.atTail {
			s.runs[h][i].hi = max(sp.hi, add.hi)
			return
		}
	}
	add.atTail = true
	s.addSpanLocked(h, add)
}

// spanOf is the index of the span holding id, or -1.
func spanOf(spans []span, id int) int {
	for i, sp := range spans {
		if sp.holds(id) {
			return i
		}
	}
	return -1
}

// addSpanLocked records that a stretch of history is stored, joining every
// run it meets into one. Caller holds the lock.
func (s *SQLiteStore) addSpanLocked(h domain.HistoryKey, add span) {
	kept := make([]span, 0, len(s.runs[h])+1)
	for _, sp := range s.runs[h] {
		if !sp.overlaps(add) {
			kept = append(kept, sp)
			continue
		}
		add.lo = min(add.lo, sp.lo)
		add.hi = max(add.hi, sp.hi)
		add.atTail = add.atTail || sp.atTail
		add.atBeginning = add.atBeginning || sp.atBeginning
	}
	kept = append(kept, add)
	sort.Slice(kept, func(i, j int) bool { return kept[i].lo < kept[j].lo })
	s.runs[h] = kept
}

// pageSpan is the stretch a page of messages covers, its oldest id to its
// newest; ok is false for a page with no stored ids.
func pageSpan(msgs []domain.Message) (span, bool) {
	sp, ok := span{}, false
	for _, m := range msgs {
		if m.ID <= 0 {
			continue
		}
		if !ok {
			sp, ok = span{lo: m.ID, hi: m.ID}, true
			continue
		}
		sp.lo = min(sp.lo, m.ID)
		sp.hi = max(sp.hi, m.ID)
	}
	return sp, ok
}

// MergeAround stores a page fetched around one message as a run of its own,
// joined to whichever runs it meets, and reports how many messages it added.
func (s *SQLiteStore) MergeAround(h domain.HistoryKey, msgs []domain.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sp, ok := pageSpan(msgs)
	if !ok {
		return 0
	}
	s.ensureTailSpanLocked(h)
	added := s.mergePageLocked(h, "MergeAround", msgs)
	s.addSpanLocked(h, sp)
	s.capMessagesLocked(h)
	return added
}

// MergeOlder stores a page fetched older than message below, which lies right
// under the run holding below and grows it. Zero for below means the page was
// fetched from the newest message down, so it ends at the tail. A page with
// nothing older than below says the run reaches the beginning.
func (s *SQLiteStore) MergeOlder(h domain.HistoryKey, below int, msgs []domain.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Capped once the runs say where the page lies, still under the lock.
	defer s.capMessagesLocked(h)
	s.ensureTailSpanLocked(h)
	added := s.mergePageLocked(h, "MergeOlder", msgs)
	sp, ok := pageSpan(older(msgs, below))
	switch {
	case below == 0 && ok:
		sp.atTail = true
		s.addSpanLocked(h, sp)
	case below == 0:
	case ok:
		sp.hi = max(sp.hi, below)
		s.addSpanLocked(h, sp)
	default:
		if i := spanOf(s.runs[h], below); i >= 0 {
			s.runs[h][i].atBeginning = true
		}
	}
	return added
}

// MergeNewer stores a page fetched newer than message above, which lies right
// over the run holding above and grows it. A page with nothing newer than above
// says the run ends at the tail; Telegram answers that way by sliding back over
// messages already held.
func (s *SQLiteStore) MergeNewer(h domain.HistoryKey, above int, msgs []domain.Message) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Capped once the runs say where the page lies, still under the lock.
	defer s.capMessagesLocked(h)
	s.ensureTailSpanLocked(h)
	added := s.mergePageLocked(h, "MergeNewer", msgs)
	var newer []domain.Message
	for _, m := range msgs {
		if m.ID > above {
			newer = append(newer, m)
		}
	}
	if sp, ok := pageSpan(newer); ok {
		sp.lo = min(sp.lo, above)
		s.addSpanLocked(h, sp)
		return added
	}
	if i := spanOf(s.runs[h], above); i >= 0 {
		sp := s.runs[h][i]
		sp.atTail = true
		// A tail run held apart from this one has nothing between them:
		// Telegram has nothing newer than above.
		for _, t := range s.runs[h] {
			if t.atTail {
				sp.hi = max(sp.hi, t.hi)
			}
		}
		s.addSpanLocked(h, sp)
	}
	return added
}

// older is the messages of a page older than below, every one when below is
// zero.
func older(msgs []domain.Message, below int) []domain.Message {
	if below == 0 {
		return msgs
	}
	var out []domain.Message
	for _, m := range msgs {
		if m.ID < below {
			out = append(out, m)
		}
	}
	return out
}

// mergePageLocked merges a fetched page into the store, laid out among the
// chat's histories, and reports how many messages it added. Reading the held
// history, merging and storing the result happen under one hold of the lock,
// which is what keeps a message arriving mid-fetch from being written back out
// of existence: read and write as two calls leave a window where the arrival
// lands between them and is lost when the merged page replaces the slice.
//
// A page that adds nothing still lands, because a message can come back edited
// without changing the count. The caller decides what to do about a zero.
//
// Only the history asked for grows a run, and the caller caps it once its runs
// are recorded; any other history the page reaches is capped here. Caller
// holds the lock.
func (s *SQLiteStore) mergePageLocked(h domain.HistoryKey, via string, msgs []domain.Message) int {
	held := s.heldReactionsLocked(h.ChatID)
	defer s.traceHeldReactionsLocked(h.ChatID, via, held)
	added := 0
	for key, page := range s.byHistoryLocked(msgs) {
		s.detachMovedLocked(key, page)
		if key != h {
			page = s.withinRunsLocked(key, page)
		}
		before := s.messages[key]
		merged := domain.MergeMessages(before, page)
		s.setMessagesLocked(key, merged, nil)
		if key != h {
			s.capMessagesLocked(key)
		}
		added += len(merged) - len(before)
	}
	return added
}

// withinRunsLocked keeps the messages of a page that a run of the history lies
// over and forgets the rest, rows included: a message that belongs here and
// lies outside every run is history nobody fetched here. Caller holds the
// lock.
func (s *SQLiteStore) withinRunsLocked(h domain.HistoryKey, msgs []domain.Message) []domain.Message {
	var kept []domain.Message
	for _, m := range msgs {
		if s.runHoldsLocked(h, m.ID) {
			kept = append(kept, m)
			continue
		}
		delete(s.msgChat, m.ID)
		s.markMsgDeletedLocked(m.ChatID, m.ID)
	}
	return kept
}

// dropFarRunsLocked gives up runs that do not end at the tail, oldest first and
// each whole, until at least excess messages are gone or none is left to give,
// and returns what the history holds after. A run with a shown message in it is
// not given up. Caller holds the lock.
func (s *SQLiteStore) dropFarRunsLocked(h domain.HistoryKey, excess int, shown map[int]struct{}) []domain.Message {
	msgs := s.messages[h]
	for excess > 0 {
		i := -1
		count := 0
		for j, sp := range s.runs[h] {
			if sp.atTail || showsAny(shown, sp) {
				continue
			}
			if n := countIn(msgs, sp); n > 0 {
				i, count = j, n
				break
			}
		}
		if i < 0 {
			return msgs
		}
		sp := s.runs[h][i]
		kept := msgs[:0:0]
		for _, m := range msgs {
			if m.ID > 0 && sp.holds(m.ID) {
				delete(s.msgChat, m.ID)
				s.markMsgDeletedLocked(h.ChatID, m.ID)
				continue
			}
			kept = append(kept, m)
		}
		msgs = kept
		s.runs[h] = append(s.runs[h][:i:i], s.runs[h][i+1:]...)
		excess -= count
	}
	return msgs
}

// countIn is how many of msgs a span holds.
func countIn(msgs []domain.Message, sp span) int {
	n := 0
	for _, m := range msgs {
		if m.ID > 0 && sp.holds(m.ID) {
			n++
		}
	}
	return n
}

// showsAny reports whether a span holds a shown message.
func showsAny(shown map[int]struct{}, sp span) bool {
	for id := range shown {
		if sp.holds(id) {
			return true
		}
	}
	return false
}

// trimTailRunLocked drops up to excess messages from the old end of the tail
// run of msgs, stopping at the first one a window shows: what is trimmed must
// stay one stretch at the end, or the run would have a hole in it. The run
// then starts at its oldest message left, and no longer reaches the
// beginning. It returns what the history holds after. Caller holds the lock.
func (s *SQLiteStore) trimTailRunLocked(h domain.HistoryKey, msgs []domain.Message, excess int, shown map[int]struct{}) []domain.Message {
	if excess <= 0 {
		return msgs
	}
	tail := -1
	for i, sp := range s.runs[h] {
		if sp.atTail {
			tail = i
		}
	}
	// With no runs recorded every message is the tail run's.
	onTail := func(m domain.Message) bool {
		return len(s.runs[h]) == 0 || (tail >= 0 && s.runs[h][tail].holds(m.ID))
	}
	kept := msgs[:0:0]
	trimming, oldest := true, 0
	for _, m := range msgs {
		if m.ID <= 0 || !onTail(m) {
			kept = append(kept, m)
			continue
		}
		if _, ok := shown[m.ID]; ok {
			trimming = false
		}
		if trimming && excess > 0 {
			excess--
			delete(s.msgChat, m.ID)
			s.markMsgDeletedLocked(h.ChatID, m.ID)
			continue
		}
		if oldest == 0 {
			oldest = m.ID
		}
		kept = append(kept, m)
	}
	if tail >= 0 && oldest > s.runs[h][tail].lo {
		s.runs[h][tail].lo = oldest
		s.runs[h][tail].atBeginning = false
	}
	return kept
}

// SetShown records what windows show, replacing what was recorded before. It
// trims nothing by itself: the next write to a history caps it against what is
// shown then.
func (s *SQLiteStore) SetShown(shown map[domain.HistoryKey][]Shown) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shown = shown
}

// shownIDsLocked is the ids of the messages windows show of a history: each
// window's anchor, as many messages older than it as it asks for, and every
// newer one, within the run that holds the anchor, since a window never reaches
// across into another run. A window anchored on a message the store does not
// hold shows nothing of it yet. Caller holds the lock.
func (s *SQLiteStore) shownIDsLocked(h domain.HistoryKey) map[int]struct{} {
	out := make(map[int]struct{})
	runs := s.runsLocked(h)
	for _, w := range s.shown[h] {
		run, idx := anchorIn(runs, w.Anchor)
		if idx < 0 {
			continue
		}
		for _, m := range run[max(0, idx-w.Before):] {
			out[m.ID] = struct{}{}
		}
	}
	return out
}

// anchorIn finds a window's anchor among runs: the newest message of the tail
// run for zero, else the message with that id. It returns the messages of the
// run holding it and the anchor's index there, -1 when no run holds it.
func anchorIn(runs []domain.Run, anchor int) ([]domain.Message, int) {
	for _, r := range runs {
		if anchor == 0 {
			if r.AtTail {
				return r.Messages, len(r.Messages) - 1
			}
			continue
		}
		for i, m := range r.Messages {
			if m.ID == anchor {
				return r.Messages, i
			}
		}
	}
	return nil, -1
}

// runHoldsLocked reports whether a run of the history lies over id. Caller
// holds the lock.
func (s *SQLiteStore) runHoldsLocked(h domain.HistoryKey, id int) bool {
	s.ensureTailSpanLocked(h)
	return spanOf(s.runs[h], id) >= 0
}

// ensureTailSpanLocked records the run a history held before anything told the
// store where its runs lie: every held message, ending at the tail. Caller
// holds the lock.
func (s *SQLiteStore) ensureTailSpanLocked(h domain.HistoryKey) {
	if len(s.runs[h]) > 0 {
		return
	}
	if sp, ok := pageSpan(s.messages[h]); ok {
		sp.atTail = true
		s.runs[h] = []span{sp}
	}
}
