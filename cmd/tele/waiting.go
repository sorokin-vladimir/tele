package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
)

// indicator shows that a command waits for the running service to stop, when
// that takes long enough to be noticed (#327).
type indicator struct {
	out io.Writer
	// terminal says out redraws a line, so the wait can be drawn on it.
	terminal bool
	// after and every are the clock: time.After and a ticker unless a test
	// plays it. every gives the ticks and what stops them.
	after func(time.Duration) <-chan time.Time
	every func(time.Duration) (<-chan time.Time, func())
}

// waitingText is what a long wait says.
const waitingText = "Waiting for the running service to stop"

// waitingFrames are one braille cell with every dot but one, the empty one
// going round clockwise.
var waitingFrames = [...]string{"⣷", "⣯", "⣟", "⡿", "⢿", "⣻", "⣽", "⣾"}

// newIndicator is an indicator drawing on out.
func newIndicator(out io.Writer) indicator {
	f, ok := out.(*os.File)
	return indicator{
		out:      out,
		terminal: ok && term.IsTerminal(f.Fd()),
		after:    time.After,
		every: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		},
	}
}

// waiting starts showing a wait, once it has lasted a second, and returns
// what ends it. Once that returns, nothing more is written.
func (in indicator) waiting() (done func()) {
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		select {
		case <-stop:
			return
		case <-in.after(time.Second):
		}
		if !in.terminal {
			_, _ = fmt.Fprintln(in.out, waitingText)
			return
		}
		frames, halt := in.every(100 * time.Millisecond)
		defer halt()
		_, _ = fmt.Fprintf(in.out, "%s %s", waitingFrames[0], waitingText)
		for i := 1; ; i++ {
			select {
			case <-stop:
				// Erased, so what is said next starts a clean line.
				_, _ = fmt.Fprint(in.out, "\r\x1b[K")
				return
			case <-frames:
				_, _ = fmt.Fprint(in.out, "\r"+waitingFrames[i%len(waitingFrames)])
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-finished
		})
	}
}
