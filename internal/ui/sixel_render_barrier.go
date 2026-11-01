package ui

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// SixelRenderBarrier observes the actual renderer flush. A unique OSC-8 link
// on one existing cell forces a diff even when the visible frame is unchanged.
// Waiters are released only after the entire write containing that link has
// completed. The OSC-8 attribute has no cell width and its changing URI makes
// Ultraviolet's cell comparison produce a diff. The private URI is replaced by
// a hyperlink reset before downstream output, so it cannot appear on screen or
// become a clickable link. Waiters for superseded views share the newer flush;
// generation/revision checks reject obsolete paints, again at the actual output
// write (Bubble Tea buffers RawMsg separately). Raw paints use the same serialized writer, so they cannot overtake
// the frame, including on slow terminals. This relies on the pinned renderer
// preserving hyperlinks; the program-level regression test covers that contract.
type SixelRenderBarrier struct {
	out     io.Writer
	mu      sync.Mutex
	next    uint64
	waiters map[uint64]chan struct{}
	raw     map[string]fmt.Stringer
	stopped bool
}

func NewSixelRenderBarrier(out io.Writer) *SixelRenderBarrier {
	return &SixelRenderBarrier{out: out, waiters: make(map[uint64]chan struct{}), raw: make(map[string]fmt.Stringer)}
}

// Preserve Bubble Tea's terminal detection when wrapping standard output.
func (b *SixelRenderBarrier) Fd() uintptr {
	if f, ok := b.out.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}

// term.File requires Read as well as Fd/Write/Close. Without it Bubble Tea's
// color-profile detector treats the wrapped terminal as NoTTY and drops OSC-8.
func (b *SixelRenderBarrier) Read(p []byte) (int, error) {
	if r, ok := b.out.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, io.EOF
}

// Close releases outstanding commands without closing the caller's output.
func (b *SixelRenderBarrier) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopped = true
	clear(b.raw)
	for id, ch := range b.waiters {
		close(ch)
		delete(b.waiters, id)
	}
	return nil
}

func (b *SixelRenderBarrier) failed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped
}

func (b *SixelRenderBarrier) register() (string, <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.next++
	id := b.next
	ready := make(chan struct{})
	if b.stopped {
		close(ready)
	} else {
		b.waiters[id] = ready
	}
	// A newer View can supersede an unflushed one. Keep its waiters pending:
	// observing the newer marker releases them too, never before a real flush.
	return fmt.Sprintf("tele://sixel-frame/%d", id), ready
}

func (b *SixelRenderBarrier) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return 0, os.ErrClosed
	}
	out := p
	for token, value := range b.raw {
		if bytes.Contains(out, []byte(token)) {
			out = bytes.ReplaceAll(out, []byte(token), []byte(value.String()))
			delete(b.raw, token)
		}
	}
	var through uint64
	for id := range b.waiters {
		open := []byte(xansi.SetHyperlink(fmt.Sprintf("tele://sixel-frame/%d", id)))
		if bytes.Contains(out, open) {
			if id > through {
				through = id
			}
			// The terminal never receives the private link; its visible cells and
			// hyperlink state remain those of the rendered text frame.
			out = bytes.ReplaceAll(out, open, []byte(xansi.ResetHyperlink()))
		}
	}
	written := 0
	for written < len(out) {
		n, err := b.out.Write(out[written:])
		written += n
		if err == nil && n == 0 {
			err = io.ErrShortWrite
		}
		if err != nil {
			b.stopped = true
			for id, ch := range b.waiters {
				close(ch)
				delete(b.waiters, id)
			}
			return written, err
		}
	}
	for id, ch := range b.waiters {
		if id <= through {
			close(ch)
			delete(b.waiters, id)
		}
	}
	return len(p), nil
}

// WithSixelRenderBarrier connects a model to the program's output barrier.
func (m RootModel) WithSixelRenderBarrier(b *SixelRenderBarrier) RootModel {
	m.sixelInline.barrier = b
	return m
}

// Defer String's revision check until the actual writer call, not RawMsg's
// earlier formatting into Bubble Tea's independently flushed output buffer.
type deferredSixelRaw struct {
	barrier *SixelRenderBarrier
	value   fmt.Stringer
}

func (w deferredSixelRaw) String() string {
	b := w.barrier
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return ""
	}
	b.next++
	token := xansi.SetHyperlink(fmt.Sprintf("tele://sixel-write/%d", b.next))
	b.raw[token] = w.value
	return token
}
func (s *inlineSixelState) raw(value fmt.Stringer) tea.Cmd {
	if s.barrier == nil {
		return tea.Raw(value)
	}
	return tea.Raw(deferredSixelRaw{barrier: s.barrier, value: value})
}
