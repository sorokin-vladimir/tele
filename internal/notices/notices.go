// Package notices holds one-time startup messages about changes a user must
// not miss, and tracks which of them have already been shown.
package notices

import (
	"context"
	"time"
)

// Notice is a single message shown once, ever.
//
// ID is a stable string rather than a version, so notices can be added
// independently and someone who skips a release still sees what applies to
// them. Delay is how long dismissal is blocked, making the message
// unskippable; that is only acceptable because it happens once per notice.
//
// A notice with Yes asks rather than tells: it is answered yes or no, and only
// an answer dismisses it. Yes is what a yes does, and says what came of it.
// AfterLogin keeps a notice back until the account is logged in, for one that
// means nothing to a person who has not got that far.
type Notice struct {
	ID         string
	Title      string
	Body       string
	Delay      time.Duration
	Yes        func(context.Context) (string, error)
	AfterLogin bool
}

// Asks reports whether n is a question to answer rather than a message to read.
func (n Notice) Asks() bool { return n.Yes != nil }

// Pending returns the notices that have not been shown yet, preserving order.
func Pending(all []Notice, seen Seen) []Notice {
	out := make([]Notice, 0, len(all))
	for _, n := range all {
		if !seen.IsSeen(n.ID) {
			out = append(out, n)
		}
	}
	return out
}
