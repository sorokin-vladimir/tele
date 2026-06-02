package components_test

import (
	"strings"
	"testing"

	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

func TestSpinner_FrameLength(t *testing.T) {
	s := components.NewSpinner()
	assert.Len(t, s.View(), 6)
}

func TestSpinner_TickAdvancesFrame(t *testing.T) {
	s := components.NewSpinner()
	first := s.View()
	s.Tick()
	assert.NotEqual(t, first, s.View())
}

func TestSpinner_FramesWrap(t *testing.T) {
	s := components.NewSpinner()
	first := s.View()
	for i := 0; i < 6; i++ {
		s.Tick()
	}
	assert.Equal(t, first, s.View())
}

func TestSpinner_AllFramesDistinct(t *testing.T) {
	s := components.NewSpinner()
	seen := make(map[string]struct{})
	for i := 0; i < 6; i++ {
		seen[s.View()] = struct{}{}
		s.Tick()
	}
	assert.Len(t, seen, 6)
}

func TestTypingDots_ViewEmpty_ReturnsEmpty(t *testing.T) {
	var d components.TypingDots
	assert.Equal(t, "", d.View(""))
}

func TestTypingDots_DotsFrameIsAlways3Chars(t *testing.T) {
	var d components.TypingDots
	for i := 0; i < 4; i++ {
		v := d.View("x")
		// base "x" (1 char) + dots suffix (3 chars) = 4 chars total
		assert.Equal(t, 4, len(v), "frame %d: %q", i, v)
		d.Tick()
	}
}

func TestTypingDots_PingPong_WrapsAfter4Ticks(t *testing.T) {
	var d components.TypingDots
	first := d.View("typing")
	for i := 0; i < 4; i++ {
		d.Tick()
	}
	assert.Equal(t, first, d.View("typing"))
}

func TestTypingDots_Tick_ChangesSuffix(t *testing.T) {
	var d components.TypingDots
	before := d.View("typing")
	d.Tick()
	assert.NotEqual(t, before, d.View("typing"))
}

func TestTypingDots_BasePreserved(t *testing.T) {
	var d components.TypingDots
	assert.True(t, strings.HasPrefix(d.View("recording audio"), "recording audio"))
}
