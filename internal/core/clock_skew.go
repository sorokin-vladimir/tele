package core

import (
	"sync"
	"time"
)

// ClockSkew is how far the local clock is from Telegram's, when it is far
// enough that the messages between them are refused (#277). Positive means the
// local clock is ahead; zero means there is no skew, or no longer is.
type ClockSkew struct {
	Skew time.Duration
}

// clockSkewOut carries the latest ClockSkew to the client. It holds one value:
// a skew is a state, so a client that fell behind wants where things stand now,
// not every change it missed.
type clockSkewOut struct {
	mu sync.Mutex
	ch chan ClockSkew
}

func newClockSkewOut() *clockSkewOut { return &clockSkewOut{ch: make(chan ClockSkew, 1)} }

// ClockSkew reports changes in clock skew.
func (o *Owner) ClockSkew() <-chan ClockSkew { return o.clockSkew.ch }

// SetClockSkew publishes the current skew, replacing one the client has not
// read yet. It never blocks: it is called from gotd's connection, which must not
// wait for a client.
func (o *Owner) SetClockSkew(d time.Duration) {
	out := o.clockSkew
	out.mu.Lock()
	defer out.mu.Unlock()
	select {
	case <-out.ch:
	default:
	}
	out.ch <- ClockSkew{Skew: d}
}
