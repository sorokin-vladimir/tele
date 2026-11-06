package ui

import (
	"bytes"
	"context"
	"fmt"
	"github.com/charmbracelet/x/term"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/stretchr/testify/require"
)

// Pipes need explicit terminal semantics for renderer tests: otherwise the
// NoTTY profile intentionally removes the OSC-8 attributes being observed.
var sixelProbeEnvironment = []string{"TERM=foot", "CLICOLOR_FORCE=1", "COLORTERM=truecolor"}

// A short, slow writer forces both output backpressure and multiple Write
// calls. The renderer and Raw output must still arrive in frame/paint order.
type slowSixelOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *slowSixelOutput) Write(p []byte) (int, error) {
	time.Sleep(time.Millisecond)
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(p) > 7 {
		p = p[:7]
	}
	return w.buf.Write(p)
}
func (w *slowSixelOutput) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.buf.String() }

type barrierProbe struct {
	state *inlineSixelState
	phase int
}

func (m barrierProbe) Init() tea.Cmd { return func() tea.Msg { return "start" } }
func (m barrierProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case string:
		m.phase = 1
		return m, m.state.afterNextRender(inlineSixelTickMsg{})
	case inlineSixelTickMsg:
		return m, tea.Raw("PAINT-AFTER-FRAME")
	case tea.RawMsg:
		return m, tea.Quit
	}
	return m, nil
}
func (m barrierProbe) View() tea.View {
	v := tea.NewView(m.state.barrierContent(fmt.Sprintf("FRAME-%d  ", m.phase)))
	v.AltScreen = true
	return v
}
func TestSixelBarrierUsesRealBubbleTeaFlush(t *testing.T) {
	for _, fps := range []int{60, 5} {
		t.Run(fmt.Sprint(fps), func(t *testing.T) {
			out := &slowSixelOutput{}
			barrier := NewSixelRenderBarrier(out)
			defer func() { _ = barrier.Close() }()
			state := newInlineSixelState()
			state.barrier = barrier
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			p := tea.NewProgram(barrierProbe{state: state}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(barrier), tea.WithEnvironment(sixelProbeEnvironment), tea.WithWindowSize(40, 5), tea.WithFPS(fps), tea.WithoutSignalHandler())
			_, err := p.Run()
			require.NoError(t, err, "%q", out.String())
			output := out.String()
			frame := strings.Index(output, "FRAME-1")
			paint := strings.Index(output, "PAINT-AFTER-FRAME")
			require.GreaterOrEqual(t, frame, 0, output)
			require.Greater(t, paint, frame, output)
			require.NotContains(t, output, "tele://sixel-frame/")
		})
	}
}

func TestSixelBarrierSupersededViewsAndShutdown(t *testing.T) {
	b := NewSixelRenderBarrier(io.Discard)
	_, old := b.register()
	marker, newer := b.register()
	select {
	case <-old:
		t.Fatal("released before flush")
	default:
	}
	_, err := b.Write([]byte(xansi.SetHyperlink(marker) + "cell" + xansi.ResetHyperlink()))
	require.NoError(t, err)
	for _, ready := range []<-chan struct{}{old, newer} {
		select {
		case <-ready:
		default:
			t.Fatal("flush did not release waiter")
		}
	}
	_, pending := b.register()
	require.NoError(t, b.Close())
	select {
	case <-pending:
	default:
		t.Fatal("shutdown leaked waiter")
	}
	require.True(t, b.failed())
}

// The pinned renderer discards controls before printable cells. A trailing
// DCS survives only if a spare cell remains; full-width frames drop it.
func TestSixelInBandProbe(t *testing.T) {
	seq := "\x1b7\x1b[2;3H\x1bP0;1;0q\"1;1;1;5#0;2;100;0;0@\x1b\\\x1b8"
	_, width, _, _ := xansi.DecodeSequence("\x1bPq@\x1b\\", 0, nil)
	require.Zero(t, width)
	for _, content := range []string{seq + "TEXT", "TE" + seq + "XT", "TEXT" + seq, strings.Repeat("T", 40) + seq} {
		out := &bytes.Buffer{}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		p := tea.NewProgram(inBandProbe{content: content}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(out), tea.WithWindowSize(40, 5), tea.WithEnvironment(sixelProbeEnvironment), tea.WithoutSignalHandler())
		_, err := p.Run()
		cancel()
		require.NoError(t, err)
		t.Logf("placement=%q DCS preserved=%v output=%q", content, strings.Contains(out.String(), "\x1bP"), out.String())
		require.Equal(t, content == "TEXT"+seq, strings.Contains(out.String(), "\x1bP"))
	}
}

type inBandProbe struct{ content string }

func (m inBandProbe) Init() tea.Cmd {
	return tea.Tick(20*time.Millisecond, func(time.Time) tea.Msg { return tea.QuitMsg{} })
}
func (m inBandProbe) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (m inBandProbe) View() tea.View                      { return tea.NewView(m.content) }

func TestSixelBarrierPreservesTerminalIdentity(t *testing.T) {
	var _ term.File = (*SixelRenderBarrier)(nil)
	b := NewSixelRenderBarrier(os.Stdout)
	require.Equal(t, os.Stdout.Fd(), b.Fd())
	require.Equal(t, term.IsTerminal(os.Stdout.Fd()), term.IsTerminal(b.Fd()))
}

func TestSixelWidthMethodOnlyObservesRenderer(t *testing.T) {
	for _, mode := range []media.Mode{media.ModeBlocks, media.ModeKitty, media.ModeSixel} {
		m := newSizedModel(t).WithImageMode(mode)
		require.Equal(t, xansi.WcWidth, m.sixelInline.widthMethod)
		// Bubble Tea consumes this report before calling model.Update.
		next, _ := m.Update(tea.ModeReportMsg{Mode: xansi.ModeUnicodeCore, Value: xansi.ModeReset})
		m = next.(RootModel)
		want := xansi.WcWidth
		if mode == media.ModeSixel {
			want = xansi.GraphemeWidth
		}
		require.Equal(t, want, m.sixelInline.widthMethod)
	}
}

type repeatedBarrierProbe struct {
	state  *inlineSixelState
	paints int
}

func (m repeatedBarrierProbe) Init() tea.Cmd { return func() tea.Msg { return "start" } }
func (m repeatedBarrierProbe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case string:
		return m, m.state.afterNextRender(inlineSixelTickMsg{})
	case inlineSixelTickMsg:
		return m, tea.Raw("REPEATED-PAINT")
	case tea.RawMsg:
		m.paints++
		if m.paints == 2 {
			return m, tea.Quit
		}
		return m, m.state.afterNextRender(inlineSixelTickMsg{})
	}
	return m, nil
}
func (m repeatedBarrierProbe) View() tea.View {
	return tea.NewView(m.state.barrierContent("UNCHANGED TEXT  "))
}
func TestSixelBarrierReleasesIdenticalVisibleFrames(t *testing.T) {
	out := &slowSixelOutput{}
	b := NewSixelRenderBarrier(out)
	defer func() { _ = b.Close() }()
	s := newInlineSixelState()
	s.barrier = b
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	p := tea.NewProgram(repeatedBarrierProbe{state: s}, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(b), tea.WithWindowSize(40, 5), tea.WithEnvironment(sixelProbeEnvironment), tea.WithoutSignalHandler())
	_, err := p.Run()
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(out.String(), "REPEATED-PAINT"))
	require.NotContains(t, out.String(), "tele://sixel-frame/")
}

func TestSixelBarrierRejectsRawInvalidatedAfterQueueing(t *testing.T) {
	for _, modal := range []bool{false, true} {
		out := &bytes.Buffer{}
		b := NewSixelRenderBarrier(out)
		s := newInlineSixelState()
		s.barrier = b
		s.frame = []string{"frame"}
		var value fmt.Stringer = inlineSixelWrite{state: s, seq: "OBSOLETE"}
		if modal {
			value = modalSixelWrite{state: s, seq: "OBSOLETE"}
		}
		// RawMsg has been formatted into Bubble Tea's output buffer already.
		queued := fmt.Sprint(deferredSixelRaw{barrier: b, value: value})
		s.invalidatePaints()
		_, err := b.Write([]byte(queued))
		require.NoError(t, err)
		require.Empty(t, out.String(), "writer must reject stale raw bytes, including modal")
		require.NoError(t, b.Close())
	}
}
