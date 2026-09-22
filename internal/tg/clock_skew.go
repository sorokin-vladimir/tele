package tg

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/proto"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Clock skew (#277). gotd checks the time in the id of every message it
// receives against the local clock, and drops the message if it is more than
// 300 seconds old or 30 seconds early. It has no notion of the difference
// between the local clock and Telegram's, which the protocol asks a client to
// keep, so a clock that is off drops every message: nothing is answered, no
// error is raised, and the connection looks alive (gotd/td#1856).
//
// The only trace of it is a log line per dropped message. That line carries the
// id of the message, and the id carries the time the server created it, so the
// skew can be read off it exactly. See docs/gotd-workarounds.md.

// rejectedEntry is the message of gotd's log entry for a dropped message, and
// rejectedForTime matches the error it carries when the time was the reason.
const rejectedEntry = "Ignoring rejected message"

var rejectedForTime = regexp.MustCompile(`bad message id (\d+): created too far in (?:past|future)`)

// skewStep is how far a skew already reported has to move to be reported
// again. Every message is rejected while the clock is off, so the same skew
// arrives over and over.
const skewStep = time.Minute

// clockSkew tracks whether the local clock is off by enough that gotd drops what
// Telegram sends, and reports each change: the skew when it appears or moves by
// skewStep, and zero when a message gets through again. A positive skew means
// the local clock is ahead.
type clockSkew struct {
	// skewed is read on every RPC and every update, so the common case - no
	// skew - is a load rather than a lock.
	skewed atomic.Bool
	mu     sync.Mutex
	last   time.Duration
	// report is called under mu, which keeps reports in order. It must not
	// block and must not call back.
	report func(time.Duration)
	// now has to be the clock gotd checks message ids against. The client sets
	// no Clock option, so gotd uses the system clock, and so does this.
	now func() time.Time
}

func newClockSkew() *clockSkew { return &clockSkew{now: time.Now} }

// SetReport installs the function each change is reported to.
func (s *clockSkew) SetReport(report func(time.Duration)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.report = report
}

// rejected records a message the server created at serverTime and gotd dropped.
// A nil tracker, on a client not built by NewGotdClient, tracks nothing.
func (s *clockSkew) rejected(serverTime time.Time) {
	if s == nil {
		return
	}
	skew := s.now().Sub(serverTime)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.skewed.Load() && (skew-s.last).Abs() < skewStep {
		return
	}
	s.skewed.Store(true)
	s.last = skew
	if s.report != nil {
		s.report(skew)
	}
}

// passed records a message that got through, which ends any skew.
func (s *clockSkew) passed() {
	if s == nil || !s.skewed.Load() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.skewed.Load() {
		return
	}
	s.skewed.Store(false)
	s.last = 0
	if s.report != nil {
		s.report(0)
	}
}

// arrivals wraps the update handler so that every update Telegram sends counts
// as a message that got through.
func (s *clockSkew) arrivals(next telegram.UpdateHandler) telegram.UpdateHandler {
	return telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
		s.passed()
		return next.Handle(ctx, u)
	})
}

// watchClockSkew returns a logger that reports gotd's rejected-message entries
// to s. It sees them whatever the log level: the entry is how the skew is found,
// and a quieter log must not hide it.
func watchClockSkew(log *zap.Logger, s *clockSkew) *zap.Logger {
	return log.WithOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
		return skewCore{Core: c, skew: s}
	}))
}

type skewCore struct {
	zapcore.Core
	skew *clockSkew
}

// Enabled claims Warn, the level gotd writes the entry at, so that zap asks
// Check about it even when the log itself is quieter.
func (c skewCore) Enabled(l zapcore.Level) bool {
	return l >= zapcore.WarnLevel || c.Core.Enabled(l)
}

func (c skewCore) With(fields []zapcore.Field) zapcore.Core {
	return skewCore{Core: c.Core.With(fields), skew: c.skew}
}

func (c skewCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if ent.Message == rejectedEntry || c.Core.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c skewCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if ent.Message == rejectedEntry {
		if at, ok := rejectedAt(fields); ok {
			c.skew.rejected(at)
		}
	}
	if !c.Core.Enabled(ent.Level) {
		return nil
	}
	return c.Core.Write(ent, fields)
}

// rejectedAt reads the time the server created a message out of the error
// gotd logged for dropping it, if the reason was the time.
func rejectedAt(fields []zapcore.Field) (time.Time, bool) {
	for _, f := range fields {
		if f.Key != "error" || f.Type != zapcore.ErrorType {
			continue
		}
		err, ok := f.Interface.(error)
		if !ok {
			continue
		}
		m := rejectedForTime.FindStringSubmatch(err.Error())
		if m == nil {
			continue
		}
		id, perr := strconv.ParseInt(m[1], 10, 64)
		if perr != nil {
			continue
		}
		return proto.MessageID(id).Time(), true
	}
	return time.Time{}, false
}
