package main

import (
	"bytes"
	"testing"
	"time"
)

// timers is the clock of an indicator as a test plays it: the delay passes
// and the frames turn when the test says so.
type timers struct {
	delay chan time.Time
	frame chan time.Time
}

func newTimers() *timers {
	return &timers{delay: make(chan time.Time), frame: make(chan time.Time)}
}

func (tm *timers) indicator(out *bytes.Buffer, terminal bool) indicator {
	return indicator{
		out:      out,
		terminal: terminal,
		after:    func(time.Duration) <-chan time.Time { return tm.delay },
		every:    func(time.Duration) (<-chan time.Time, func()) { return tm.frame, func() {} },
	}
}

// A wait that ends within the second it is given says nothing (#327).
func TestWaiting_AShortWaitSaysNothing(t *testing.T) {
	var out bytes.Buffer
	done := newTimers().indicator(&out, true).waiting()

	done()

	if out.Len() != 0 {
		t.Errorf("said %q", out.String())
	}
}

// Where nothing redraws a line, a long wait is said once, as a line of its
// own (#327).
func TestWaiting_ALongWaitIsSaidOnceWithNoTerminal(t *testing.T) {
	var out bytes.Buffer
	tm := newTimers()
	done := tm.indicator(&out, false).waiting()

	tm.delay <- time.Time{}
	done()

	if want := "Waiting for the running service to stop\n"; out.String() != want {
		t.Errorf("said %q, want %q", out.String(), want)
	}
}

// On a terminal a long wait is drawn as one braille cell whose empty dot goes
// round clockwise, and the line is erased when it ends (#327).
func TestWaiting_ALongWaitIsDrawnOnATerminal(t *testing.T) {
	var out bytes.Buffer
	tm := newTimers()
	done := tm.indicator(&out, true).waiting()

	tm.delay <- time.Time{}
	for range 8 {
		tm.frame <- time.Time{}
	}
	done()

	want := "⣷ Waiting for the running service to stop" +
		"\r⣯\r⣟\r⡿\r⢿\r⣻\r⣽\r⣾\r⣷" +
		"\r\x1b[K"
	if out.String() != want {
		t.Errorf("drew %q, want %q", out.String(), want)
	}
}
