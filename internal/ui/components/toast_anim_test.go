package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func splitLines(s string) []string { return strings.Split(s, "\n") }
func lineWidth(s string) int       { return lipgloss.Width(s) }

// settle runs the animation to completion so the stack is at rest.
func settle(s *ToastStack) {
	for i := 0; i < 100 && s.Animating(); i++ {
		s.StepToastAnim()
	}
}

func TestToastStack_AddStartsEntering(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "boom")
	if !s.Animating() {
		t.Fatal("a freshly added toast should be animating (entering)")
	}
	if got := s.toasts[0].phase; got != phaseEntering {
		t.Fatalf("phase = %v, want phaseEntering", got)
	}
	if got := s.toasts[0].animStep; got != toastAnimSteps {
		t.Fatalf("animStep = %d, want %d", got, toastAnimSteps)
	}
}

func TestToastStack_StepToastAnim_EnteringSettlesToShown(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "boom")
	for i := 0; i < toastAnimSteps; i++ {
		s.StepToastAnim()
	}
	if s.Animating() {
		t.Fatal("entering toast should be settled after toastAnimSteps frames")
	}
	if got := s.toasts[0].phase; got != phaseShown {
		t.Fatalf("phase = %v, want phaseShown", got)
	}
	if got := s.toasts[0].animStep; got != 0 {
		t.Fatalf("animStep = %d, want 0", got)
	}
}

func TestToastStack_Dismiss_StartsLeavingNotRemoved(t *testing.T) {
	s := newTestStack()
	serial := s.Add(ToastError, "boom")
	settle(s)
	s.Dismiss(serial)
	if s.count() != 1 {
		t.Fatalf("dismiss must not remove immediately, count = %d", s.count())
	}
	if got := s.toasts[0].phase; got != phaseLeaving {
		t.Fatalf("phase = %v, want phaseLeaving", got)
	}
	settle(s)
	if s.count() != 0 {
		t.Fatalf("toast should be gone after the leave animation, count = %d", s.count())
	}
}

func TestToastStack_DismissTop_StartsLeaving(t *testing.T) {
	s := newTestStack()
	s.Add(ToastInfo, "a")
	s.Add(ToastInfo, "b")
	settle(s)
	if !s.DismissTop() {
		t.Fatal("DismissTop should return true when non-empty")
	}
	if s.count() != 2 {
		t.Fatalf("DismissTop must not remove immediately, count = %d", s.count())
	}
	settle(s)
	if s.count() != 1 {
		t.Fatalf("newest toast should be gone after the leave animation, count = %d", s.count())
	}
}

func TestToastOffset_ZeroAtRestSpanWhenFull(t *testing.T) {
	if toastOffset(0, 30) != 0 {
		t.Fatal("offset at rest (animStep 0) must be 0")
	}
	if got := toastOffset(toastAnimSteps, 30); got != 32 {
		t.Fatalf("offset at full step = %d, want span 32 (boxW+2)", got)
	}
}

func TestZones_EnteringToastTruncatedToViewport(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "connection lost")
	// One frame in: the toast is partly on-screen and must not overflow width.
	s.StepToastAnim()
	for _, z := range s.Zones() {
		w := 0
		for _, line := range splitLines(z.Block) {
			if lw := lineWidth(line); lw > w {
				w = lw
			}
		}
		if z.Left+w > s.width {
			t.Fatalf("zone overflows viewport: left %d + width %d > %d", z.Left, w, s.width)
		}
	}
}

func TestZones_FullyOffscreenToastIsSkipped(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "boom") // entering at max step => fully off-screen
	if len(s.Zones()) != 0 {
		t.Fatal("a fully off-screen entering toast should render no zone")
	}
	settle(s)
	if len(s.Zones()) != 1 {
		t.Fatalf("a settled toast should render one zone, got %d", len(s.Zones()))
	}
}

func TestActionRegions_NoneWhileAnimating(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "failed", ToastAction{Label: "retry", Key: "r", Msg: toastTestMsg{id: 1}})
	if len(s.actionRects()) != 0 {
		t.Fatal("an entering toast must expose no click regions")
	}
	settle(s)
	if len(s.actionRects()) == 0 {
		t.Fatal("a settled toast with an action must expose a click region")
	}
}
