package tg

import (
	"context"
	"sync"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	gotdtg "github.com/gotd/td/tg"

	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// throttle is the owner's one outbound budget, shared by every call to
// Telegram whoever makes it (#212). A call that is answered with a flood wait
// holds every later call of the same method until the wait is over: Telegram
// scopes a flood wait to the method that earned it, and a caller that asked
// again meanwhile would only earn a longer one.
//
// Ahead of any flood wait, every call takes a token from one bucket, so a
// burst of calls is spread out before Telegram has to complain about it.
type throttle struct {
	now   func() time.Time
	sleep func(context.Context, time.Duration) error

	mu sync.Mutex
	// held is when each method held by a flood wait may be called again.
	held map[string]time.Time
	// tokens is what the bucket holds as of filled. It goes below zero while
	// calls wait their turn: each one has taken the token it waits for.
	tokens float64
	filled time.Time
	// report is told of a wait somebody runs into; said is, per method, the
	// last wait it was told of.
	report func(Throttled)
	said   map[string]time.Time
}

// Throttled is a flood wait a call somebody waits on has run into: the method
// Telegram holds, and until when. Once Until has passed it holds nothing, so
// nothing has to say that it ended.
type Throttled struct {
	Method string
	Until  time.Time
}

// throttleShown is how long a wait has to be for anybody to notice it.
const throttleShown = 2 * time.Second

// SetReport installs the function told of a wait somebody runs into. It must
// not block: it is called on the way to Telegram.
func (th *throttle) SetReport(report func(Throttled)) {
	th.mu.Lock()
	defer th.mu.Unlock()
	th.report = report
}

// The bucket's rate and size. Telegram publishes no limits, so these are a
// guess: enough to open a chat with its avatars and thumbnails at once, too
// few for a script sending in a loop to earn a flood wait. Constants rather
// than a setting, since nobody could know what to set them to.
//
// throttleReserve is the part of the bucket background work never takes, so a
// call someone is waiting on always finds a token close at hand.
const (
	throttleRate    = 20 // calls a second
	throttleBurst   = 40
	throttleReserve = throttleBurst / 3
)

func newThrottle(now func() time.Time, sleep func(context.Context, time.Duration) error) *throttle {
	return &throttle{
		now: now, sleep: sleep, held: map[string]time.Time{}, tokens: throttleBurst, filled: now(),
		report: func(Throttled) {}, said: map[string]time.Time{},
	}
}

// take takes a token from the bucket, waiting its turn when there is none.
//
// A call whose context ends while it waits keeps the token it took: it is not
// given back, so each call queued behind it waits one turn (1/throttleRate)
// longer than it had to. Giving it back would mean tracking every waiting
// call's place in the queue; for a few cancelled calls the cost is tens of
// milliseconds. If calls are ever reported slow after many cancellations (a
// chat left mid-load, a scrolled-past image), this is the place to look.
//
// Background work takes a token only while the reserve would still be left
// after it, and otherwise waits for the bucket to fill above the reserve; it
// never queues in debt, so it is never ahead of a call someone waits on.
func (th *throttle) take(ctx context.Context) error {
	background := priority.IsBackground(ctx)
	for {
		th.mu.Lock()
		now := th.now()
		th.tokens = min(throttleBurst, th.tokens+now.Sub(th.filled).Seconds()*throttleRate)
		th.filled = now
		var short float64
		if background {
			short = throttleReserve + 1 - th.tokens
			if short <= 0 {
				th.tokens--
			}
		} else {
			th.tokens--
			short = -th.tokens
		}
		th.mu.Unlock()
		if short <= 0 {
			return nil
		}
		if err := th.sleep(ctx, time.Duration(short*float64(time.Second)/throttleRate)); err != nil || !background {
			return err
		}
	}
}

// sleepCtx waits for d or until ctx ends, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// middleware puts every call through the throttle. It sits outside the error
// mapping, so the flood waits it reads are already domain errors.
func (th *throttle) middleware() telegram.Middleware {
	return telegram.MiddlewareFunc(func(next gotdtg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			op := opName(input)
			if err := th.wait(ctx, op); err != nil {
				return err
			}
			if err := th.take(ctx); err != nil {
				return err
			}
			err := next.Invoke(ctx, input, output)
			if e, ok := telerr.As(err); ok && e.Kind == telerr.RateLimited {
				th.hold(op, e.RetryAfter)
			}
			return err
		}
	})
}

// wait sits out what is left of op's flood wait, when a call would sit it out
// anyway. A longer one is refused at once with what is left of it, the same
// answer Telegram would give, without asking Telegram again. Background work
// sits out any wait whole: nobody is waiting on it to be told.
//
// A call somebody waits on that runs into a wait long enough to notice has it
// said, once per wait, so that the wait shows rather than looking like a
// freeze.
func (th *throttle) wait(ctx context.Context, op string) error {
	background := priority.IsBackground(ctx)
	th.mu.Lock()
	until := th.held[op]
	left := until.Sub(th.now())
	var say func(Throttled)
	if !background && left >= throttleShown && until.After(th.said[op]) {
		th.said[op] = until
		say = th.report
	}
	th.mu.Unlock()
	if say != nil {
		say(Throttled{Method: op, Until: until})
	}
	if left <= 0 {
		return nil
	}
	if left > rateLimitBudget && !background {
		return &telerr.Error{Kind: telerr.RateLimited, Op: op, RetryAfter: left}
	}
	return th.sleep(ctx, left)
}

// hold keeps op from being called for d from now, unless it is held longer
// already.
func (th *throttle) hold(op string, d time.Duration) {
	th.mu.Lock()
	defer th.mu.Unlock()
	if until := th.now().Add(d); until.After(th.held[op]) {
		th.held[op] = until
	}
}
