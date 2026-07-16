package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func newTestStack() *ToastStack {
	return NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
}

func TestToastStack_AddReturnsIncreasingSerials(t *testing.T) {
	s := newTestStack()
	a := s.Add(ToastInfo, "one")
	b := s.Add(ToastError, "two")
	if a == b {
		t.Fatalf("serials must differ: %d == %d", a, b)
	}
	if b <= a {
		t.Fatalf("serials must increase: %d then %d", a, b)
	}
	if s.Empty() {
		t.Fatal("stack should not be empty after Add")
	}
}

func TestToastStack_DismissRemovesMatching(t *testing.T) {
	s := newTestStack()
	first := s.Add(ToastInfo, "one")
	s.Add(ToastError, "two")
	s.Dismiss(first)
	if s.count() != 1 {
		t.Fatalf("want 1 toast after dismiss, got %d", s.count())
	}
}

func TestToastStack_DismissStaleSerialIsNoop(t *testing.T) {
	s := newTestStack()
	s.Add(ToastInfo, "one")
	s.Dismiss(9999) // never issued
	if s.count() != 1 {
		t.Fatalf("stale dismiss must not remove anything, got %d", s.count())
	}
}

func TestToastStack_DismissTopRemovesNewest(t *testing.T) {
	s := newTestStack()
	oldest := s.Add(ToastInfo, "one")
	s.Add(ToastError, "two")
	if !s.DismissTop() {
		t.Fatal("DismissTop should return true when non-empty")
	}
	if s.count() != 1 {
		t.Fatalf("want 1 left, got %d", s.count())
	}
	// The remaining one must be the oldest.
	if !s.hasSerial(oldest) {
		t.Fatal("DismissTop removed the wrong toast")
	}
}

func TestToastStack_DismissTopEmptyReturnsFalse(t *testing.T) {
	s := newTestStack()
	if s.DismissTop() {
		t.Fatal("DismissTop on empty stack should return false")
	}
}

func TestToastKindOf_MatchesSeverity(t *testing.T) {
	if ToastKindOf(SeverityInfo) != ToastInfo ||
		ToastKindOf(SeverityWarning) != ToastWarning ||
		ToastKindOf(SeverityError) != ToastError {
		t.Fatal("ToastKindOf must map Severity 1:1 to ToastKind")
	}
}

func TestToastStack_ZonesRenderNonEmptyOnly(t *testing.T) {
	s := newTestStack()
	if len(s.Zones()) != 0 {
		t.Fatal("empty stack renders no zones")
	}
	s.Add(ToastError, "boom")
	zones := s.Zones()
	if len(zones) != 1 {
		t.Fatalf("want 1 non-empty zone, got %d", len(zones))
	}
	if zones[0].Block == "" {
		t.Fatal("zone block must not be empty")
	}
}

func TestToastStack_ZonesContainText(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "connection lost")
	block := s.Zones()[0].Block
	if !strings.Contains(block, "connection lost") {
		t.Fatalf("block missing toast text:\n%s", block)
	}
}

func TestToastStack_FooterShowsActionLabels(t *testing.T) {
	s := newTestStack()
	s.Add(ToastError, "failed", ToastAction{Label: "retry", Key: "r"}, ToastAction{Label: "close", Key: "x"})
	// Strip ANSI: the accent styling splits highlighted letters from the rest of
	// the label with escape sequences, so assert on the visible text.
	block := xansi.Strip(s.Zones()[0].Block)
	if !strings.Contains(block, "retry") || !strings.Contains(block, "close") {
		t.Fatalf("footer missing action labels:\n%s", block)
	}
}

func TestToastStack_OverflowShowsMore(t *testing.T) {
	s := NewToastStack(80, 24, 2, ZoneBottomRight, ZoneTopRight)
	s.Add(ToastInfo, "a")
	s.Add(ToastInfo, "b")
	s.Add(ToastInfo, "c")
	s.Add(ToastInfo, "d")
	block := s.Zones()[0].Block
	if !strings.Contains(block, "+2 more") {
		t.Fatalf("want overflow marker +2 more:\n%s", block)
	}
}

func TestToastStack_BottomRightAnchoredAboveStatusBar(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	s.Add(ToastError, "x")
	z := s.Zones()[0]
	blockH := strings.Count(z.Block, "\n") + 1
	if z.Top != 24-blockH-1 {
		t.Fatalf("bottom-right top = %d, want %d", z.Top, 24-blockH-1)
	}
	if z.Left < 0 || z.Left >= 80 {
		t.Fatalf("left out of range: %d", z.Left)
	}
}

func TestToastStack_TopRightAnchoredAtTop(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	s.Add(ToastNotify, "new message")
	z := s.Zones()[0]
	if z.Top != 1 {
		t.Fatalf("top-right top = %d, want 1", z.Top)
	}
}

func TestToastStack_HitTestMissWhenEmpty(t *testing.T) {
	s := newTestStack()
	if _, ok := s.HitTest(10, 10); ok {
		t.Fatal("empty stack should hit nothing")
	}
}

func TestToastStack_HitTestReturnsActionMsg(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	want := toastTestMsg{id: 7}
	s.Add(ToastError, "failed", ToastAction{Label: "retry", Key: "r", Msg: want})

	// Find the region the component itself reports and click its center.
	rects := s.actionRects()
	if len(rects) == 0 {
		t.Fatal("expected at least one action region")
	}
	r := rects[0].Rect
	cx := r.Left + r.Width/2
	cy := r.Top + r.Height/2
	msg, ok := s.HitTest(cx, cy)
	if !ok {
		t.Fatalf("expected a hit at (%d,%d)", cx, cy)
	}
	if msg != tea.Msg(want) {
		t.Fatalf("wrong msg: got %#v want %#v", msg, want)
	}
}

func TestToastStack_HitTestMissOffTarget(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	s.Add(ToastError, "failed", ToastAction{Label: "close", Key: "x", Msg: toastTestMsg{id: 1}})
	if _, ok := s.HitTest(0, 0); ok {
		t.Fatal("click at (0,0) should miss the bottom-right toast")
	}
}

func TestToastStack_ClickMsgWholeBoxHits(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	want := toastTestMsg{id: 99}
	serial := s.Add(ToastNotify, "Alice\nhey there")
	s.SetClick(serial, want)

	z := s.Zones()[0] // notify -> top-right, the only non-empty zone
	// A point inside the box body (not on the border) must return the click msg.
	cx, cy := z.Left+3, z.Top+1
	msg, ok := s.HitTest(cx, cy)
	if !ok {
		t.Fatalf("expected whole-box click hit at (%d,%d)", cx, cy)
	}
	if msg != tea.Msg(want) {
		t.Fatalf("wrong click msg: got %#v want %#v", msg, want)
	}
}

func TestToastStack_NoClickMsgNoWholeBoxHit(t *testing.T) {
	s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
	s.Add(ToastNotify, "Alice\nhey there") // no SetClick
	z := s.Zones()[0]
	if _, ok := s.HitTest(z.Left+3, z.Top+1); ok {
		t.Fatal("a toast without a click msg must not be a whole-box target")
	}
}

func TestToastStack_DarkBackgroundChangesRender(t *testing.T) {
	mk := func(dark bool) string {
		s := NewToastStack(80, 24, 3, ZoneBottomRight, ZoneTopRight)
		s.SetDarkBackground(dark)
		s.Add(ToastError, "boom")
		return s.Zones()[0].Block
	}
	if mk(true) == mk(false) {
		t.Fatal("dark and light backgrounds should render differently")
	}
}

// test-only helper Msg used across toast tests.
type toastTestMsg struct{ id int }

var _ tea.Msg = toastTestMsg{}
