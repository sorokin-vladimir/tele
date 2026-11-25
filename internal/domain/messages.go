package domain

// MergeMessages merges a page fetched from Telegram into a chat's stored
// history. Both sides are ordered oldest first, by date and then by id, and so
// is the result.
//
// A message present on both sides is one message, and the fetched copy is the
// one kept: Telegram is the source of truth for what a message says, and a
// message edited while this client was not listening is exactly why the two
// copies can differ. Without the dedup, overlapping pages stack into a
// repeating date range instead (issue #120).
//
// The direction the page extends the history in is not a parameter: an older
// page and a page fetched to close a gap merge by the same rule, and the order
// of the result follows from the messages rather than from which call site
// asked for them.
func MergeMessages(existing, fetched []Message) []Message {
	if len(fetched) == 0 {
		return existing
	}
	if len(existing) == 0 {
		return fetched
	}

	replaced := make(map[int]struct{}, len(fetched))
	for _, m := range fetched {
		replaced[m.ID] = struct{}{}
	}

	out := make([]Message, 0, len(existing)+len(fetched))
	i, j := 0, 0
	for i < len(existing) && j < len(fetched) {
		if _, dup := replaced[existing[i].ID]; dup {
			i++
			continue
		}
		if messageBefore(existing[i], fetched[j]) {
			out = append(out, existing[i])
			i++
			continue
		}
		out = append(out, fetched[j])
		j++
	}
	for ; i < len(existing); i++ {
		if _, dup := replaced[existing[i].ID]; dup {
			continue
		}
		out = append(out, existing[i])
	}
	return append(out, fetched[j:]...)
}

// messageBefore orders two messages the way the store holds them: by date, and
// by id where a date is shared. Telegram stamps a whole album with one date, so
// the id is not a tiebreaker of last resort but the ordinary case.
func messageBefore(a, b Message) bool {
	if !a.Date.Equal(b.Date) {
		return a.Date.Before(b.Date)
	}
	return a.ID < b.ID
}

// Run is one unbroken stretch of a history as it is stored: every message
// between its two ends is there. A history is stored as one run or several,
// with history nobody fetched between them, and two runs are never adjacent.
type Run struct {
	// Messages are the run's messages, oldest first.
	Messages []Message
	// AtTail marks the run live messages land on: it ends at the history's
	// newest message.
	AtTail bool
	// AtBeginning marks a run with no history below it: older history was
	// asked for there and Telegram had none.
	AtBeginning bool
	// UpTo is the newest id the run reaches. It can name a message the store
	// does not hold: a page fetched older than a message reaches up to it,
	// whether that message is kept or not.
	UpTo int
}

// FindMessage finds a message among a history's runs: the run holding it and
// its index in that run's messages.
func FindMessage(runs []Run, id int) (Run, int, bool) {
	for _, r := range runs {
		for i, m := range r.Messages {
			if m.ID == id {
				return r, i, true
			}
		}
	}
	return Run{}, -1, false
}

// TailRun is the run of a history live messages land on, when it holds one.
func TailRun(runs []Run) (Run, bool) {
	for _, r := range runs {
		if r.AtTail && len(r.Messages) > 0 {
			return r, true
		}
	}
	return Run{}, false
}
