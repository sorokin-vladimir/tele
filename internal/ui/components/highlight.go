package components

import (
	"fmt"
	"image/color"
	"time"

	"charm.land/lipgloss/v2"
)

// Highlight fade timing, shared by the message-bubble and chat-list highlights.
// A fresh highlight starts at HighlightInitialStep and counts down to 0 over
// ticks spaced HighlightFadeInterval apart. It stays at full accent for the
// hold ticks, then fades to the base color over the last HighlightFadeSteps
// ticks. Total ≈ HighlightInitialStep * HighlightFadeInterval (≈3s).
const (
	// HighlightFadeSteps is the number of trailing ticks over which the accent
	// fades back to the base color.
	HighlightFadeSteps = 5
	// HighlightHoldSteps is the number of leading ticks the highlight stays at
	// full accent before the fade begins, so the cue is easy to notice.
	HighlightHoldSteps = 10
	// HighlightInitialStep is the step value a fresh highlight starts at: the
	// full-accent hold followed by the fade.
	HighlightInitialStep = HighlightHoldSteps + HighlightFadeSteps
	// HighlightFadeInterval is the delay between successive fade ticks.
	HighlightFadeInterval = 200 * time.Millisecond
)

// Highlight accent tones a fresh highlight starts at, distinct from the
// read/unread blue (12) and the selection background (63). The light-theme tone
// is a more saturated orange so it stays vivid against a light background.
var (
	HighlightAccent      color.Color = lipgloss.Color("#ffaf00")
	HighlightAccentLight color.Color = lipgloss.Color("#ff8c00")
)

// HighlightAccentFor returns the highlight accent suited to the background: the
// amber tone on dark backgrounds, a more saturated orange on light ones.
func HighlightAccentFor(isDark bool) color.Color {
	if isDark {
		return HighlightAccent
	}
	return HighlightAccentLight
}

// HighlightKind selects which accent a highlight fades from: the amber info tone
// (jump-to) or the red error tone (optimistic-action rollback).
type HighlightKind int

const (
	HighlightInfo HighlightKind = iota
	HighlightError
)

// Error accent tones a rollback highlight starts at. The dark-background tone is
// the truecolor form of xterm 203 — the same red as the toast error border, so
// the row highlight and the failure toast read as one color. The light-theme
// tone is a deeper red for readability on a light background.
var (
	ErrorAccent      color.Color = lipgloss.Color("#ff5f5f") // dark bg; xterm 203
	ErrorAccentLight color.Color = lipgloss.Color("#d70000") // light bg; xterm 160
)

// ErrorAccentFor returns the error accent suited to the background.
func ErrorAccentFor(isDark bool) color.Color {
	if isDark {
		return ErrorAccent
	}
	return ErrorAccentLight
}

// FadeAccentColor linearly interpolates RGB from base (step 0) toward accent
// (step == total), returning a truecolor lipgloss color. lipgloss downsamples
// on limited-color terminals. step is clamped to [0, total].
func FadeAccentColor(accent, base color.Color, step, total int) color.Color {
	if total <= 0 {
		return accent
	}
	if step < 0 {
		step = 0
	}
	if step > total {
		step = total
	}
	ar, ag, ab := rgb8(accent)
	br, bg, bb := rgb8(base)
	t := float64(step) / float64(total)
	lerp := func(from, to uint8) uint8 {
		return uint8(float64(from) + (float64(to)-float64(from))*t)
	}
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", lerp(br, ar), lerp(bg, ag), lerp(bb, ab)))
}

func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}
