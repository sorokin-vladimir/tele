package tg

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	gotdtg "github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/priority"
	"github.com/sorokin-vladimir/tele/internal/telerr"
)

// throttleClock is the throttle's time, moved only by the waits it is asked
// for, which it records.
type throttleClock struct {
	now   time.Time
	waits []time.Duration
}

func (c *throttleClock) Now() time.Time { return c.now }

func (c *throttleClock) Sleep(_ context.Context, d time.Duration) error {
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	return nil
}

// telegramFake is Telegram behind the throttle: it records which methods
// reached it and answers each with what answer says.
type telegramFake struct {
	reached []string
	answer  func(op string) error
}

func (f *telegramFake) Invoke(_ context.Context, input bin.Encoder, _ bin.Decoder) error {
	op := opName(input)
	f.reached = append(f.reached, op)
	if f.answer == nil {
		return nil
	}
	return f.answer(op)
}

// floodOnce answers the first call of op with a flood wait of d.
func floodOnce(op string, d time.Duration) func(string) error {
	flooded := false
	return func(got string) error {
		if got == op && !flooded {
			flooded = true
			return &telerr.Error{Kind: telerr.RateLimited, Op: op, RetryAfter: d}
		}
		return nil
	}
}

func throttled(t *testing.T, tg *telegramFake) (*throttleClock, gotdtg.Invoker) {
	t.Helper()
	clock := &throttleClock{now: time.Unix(1_700_000_000, 0)}
	th := newThrottle(clock.Now, clock.Sleep)
	return clock, th.middleware().Handle(tg)
}

func call(inv gotdtg.Invoker, req bin.Encoder) error {
	return inv.Invoke(context.Background(), req, nil)
}

const (
	getHistory = "messages.getHistory"
	sendText   = "messages.sendMessage"
)

// A flood wait holds every later call of the method that earned it until the
// wait is over, so none of them reaches Telegram to earn a longer one (#212).
func TestThrottle_AFloodWaitHoldsTheMethodUntilItIsOver(t *testing.T) {
	tg := &telegramFake{answer: floodOnce(getHistory, 30*time.Second)}
	clock, inv := throttled(t, tg)

	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	clock.now = clock.now.Add(10 * time.Second)
	require.NoError(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))

	assert.Equal(t, []time.Duration{20 * time.Second}, clock.waits, "the second call sat out the rest of the wait")
	assert.Equal(t, []string{getHistory, getHistory}, tg.reached, "and only then went to Telegram")
}

// Telegram scopes a flood wait to the method, so the other methods go on.
func TestThrottle_OtherMethodsAreNotHeld(t *testing.T) {
	tg := &telegramFake{answer: floodOnce(getHistory, 30*time.Second)}
	clock, inv := throttled(t, tg)

	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))

	assert.Empty(t, clock.waits)
	assert.Equal(t, []string{getHistory, sendText}, tg.reached)
}

// A wait longer than a call would sit out is refused at once, with what is
// left of it, and Telegram is not asked again meanwhile.
func TestThrottle_AWaitTooLongToSitOutIsRefusedWithoutAsking(t *testing.T) {
	tg := &telegramFake{answer: floodOnce(getHistory, 10*time.Minute)}
	clock, inv := throttled(t, tg)

	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	clock.now = clock.now.Add(time.Minute)
	err := call(inv, &gotdtg.MessagesGetHistoryRequest{})

	e, ok := telerr.As(err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, telerr.RateLimited, e.Kind)
	assert.Equal(t, 9*time.Minute, e.RetryAfter)
	assert.Equal(t, getHistory, e.Op)
	assert.Empty(t, clock.waits)
	assert.Equal(t, []string{getHistory}, tg.reached)
}

// A burst goes straight to Telegram up to the bucket's size; past it each call
// waits its turn at the bucket's rate (#212).
func TestThrottle_ABurstPastTheBucketWaitsItsTurn(t *testing.T) {
	tg := &telegramFake{}
	clock, inv := throttled(t, tg)

	for range throttleBurst {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	assert.Empty(t, clock.waits, "a burst the bucket holds is not held")

	require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))

	assert.Equal(t, []time.Duration{50 * time.Millisecond, 50 * time.Millisecond}, clock.waits,
		"twenty a second is one every 50ms")
	assert.Len(t, tg.reached, throttleBurst+2)
}

// An idle bucket fills again at its rate, and no fuller than its size.
func TestThrottle_TheBucketFillsAgainWhileIdle(t *testing.T) {
	tg := &telegramFake{}
	clock, inv := throttled(t, tg)

	for range throttleBurst {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	clock.now = clock.now.Add(time.Second)
	for range 20 {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	assert.Empty(t, clock.waits, "a second idle is twenty calls")

	clock.now = clock.now.Add(time.Hour)
	for range throttleBurst {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	assert.Equal(t, []time.Duration{50 * time.Millisecond}, clock.waits, "an hour idle is still only the bucket's size")
}

// Background work leaves the last third of the bucket to the calls someone is
// waiting on: it takes a token only while that much would be left (#212).
func TestThrottle_BackgroundLeavesTheReserveToInteractiveCalls(t *testing.T) {
	tg := &telegramFake{}
	clock, inv := throttled(t, tg)
	background := priority.InBackground(context.Background())

	for range throttleBurst - throttleReserve - 1 {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	require.NoError(t, inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil))
	assert.Empty(t, clock.waits, "with more than the reserve left, background goes")

	require.NoError(t, inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil))
	assert.Equal(t, []time.Duration{50 * time.Millisecond}, clock.waits,
		"with only the reserve left, it waits for a token above it")

	clock.waits = nil
	for range throttleReserve {
		require.NoError(t, call(inv, &gotdtg.MessagesSendMessageRequest{}))
	}
	assert.Empty(t, clock.waits, "the reserve is the interactive calls' own")
}

// Nobody is waiting on background work, so a flood wait is sat out whole
// rather than refused.
func TestThrottle_BackgroundSitsOutAFloodWaitWhole(t *testing.T) {
	tg := &telegramFake{answer: floodOnce("upload.getFile", 10*time.Minute)}
	clock, inv := throttled(t, tg)
	background := priority.InBackground(context.Background())

	require.Error(t, inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil))
	clock.now = clock.now.Add(time.Minute)
	require.NoError(t, inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil))

	assert.Equal(t, []time.Duration{9 * time.Minute}, clock.waits)
	assert.Len(t, tg.reached, 2)
}

// reported is what the throttle said it holds, in order.
func reported(th *throttle) *[]Throttled {
	var got []Throttled
	th.SetReport(func(t Throttled) { got = append(got, t) })
	return &got
}

// A call somebody waits on that runs into a flood wait is said, so that the
// wait shows rather than looking like a freeze (#212).
func TestThrottle_AnInteractiveCallHeldIsSaid(t *testing.T) {
	tg := &telegramFake{answer: floodOnce(getHistory, 10*time.Minute)}
	clock := &throttleClock{now: time.Unix(1_700_000_000, 0)}
	th := newThrottle(clock.Now, clock.Sleep)
	got := reported(th)
	inv := th.middleware().Handle(tg)

	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	until := clock.now.Add(10 * time.Minute)
	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))

	assert.Equal(t, []Throttled{{Method: getHistory, Until: until}}, *got,
		"said once a call ran into it, and not again for the same wait")
}

// A wait too short to notice, and one only background work runs into, are
// not said: nobody would see anything frozen.
func TestThrottle_AShortWaitOrABackgroundOneIsNotSaid(t *testing.T) {
	tg := &telegramFake{answer: func(op string) error {
		d := time.Second
		if op == "upload.getFile" {
			d = 10 * time.Minute
		}
		return floodOnce(op, d)(op)
	}}
	clock := &throttleClock{now: time.Unix(1_700_000_000, 0)}
	th := newThrottle(clock.Now, clock.Sleep)
	got := reported(th)
	inv := th.middleware().Handle(tg)
	background := priority.InBackground(context.Background())

	_ = call(inv, &gotdtg.MessagesGetHistoryRequest{})
	_ = call(inv, &gotdtg.MessagesGetHistoryRequest{})
	_ = inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil)
	_ = inv.Invoke(background, &gotdtg.UploadGetFileRequest{}, nil)

	assert.Empty(t, *got)
}

func TestThrottle_AWaitThatIsOverHoldsNothing(t *testing.T) {
	tg := &telegramFake{answer: floodOnce(getHistory, 30*time.Second)}
	clock, inv := throttled(t, tg)

	require.Error(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))
	clock.now = clock.now.Add(31 * time.Second)
	require.NoError(t, call(inv, &gotdtg.MessagesGetHistoryRequest{}))

	assert.Empty(t, clock.waits)
	assert.Len(t, tg.reached, 2)
}
