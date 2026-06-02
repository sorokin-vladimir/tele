package components

// SpinnerTickMsg is sent by the root tick chain every 150ms while on the main screen.
type SpinnerTickMsg struct{}

var spinnerFrames = [6]string{"[=   ]", "[==  ]", "[=== ]", "[ ===]", "[  ==]", "[   =]"}

// Spinner is a ping-pong bar. Call Tick() on each SpinnerTickMsg. Call View() to render.
type Spinner struct {
	frame int
}

func NewSpinner() Spinner { return Spinner{} }

// Tick advances the spinner by one frame.
func (s *Spinner) Tick() {
	s.frame = (s.frame + 1) % len(spinnerFrames)
}

// View returns the current frame string (always 6 chars wide).
func (s Spinner) View() string {
	return spinnerFrames[s.frame]
}

// TypingDotsTickMsg is sent every 400ms while a typing indicator is active.
type TypingDotsTickMsg struct{}

// dotsFrames is a ping-pong animation. Each frame is exactly 3 chars wide
// so the border width never changes as dots appear and disappear.
var dotsFrames = [4]string{".  ", ".. ", "...", ".. "}

// TypingDots animates a "base typing..." label. Call Tick() on each
// TypingDotsTickMsg and View(base) to render.
type TypingDots struct {
	frame int
}

// Tick advances to the next animation frame.
func (d *TypingDots) Tick() {
	d.frame = (d.frame + 1) % len(dotsFrames)
}

// View returns base followed by the current animated dots suffix.
// Returns "" when base is empty (no typing active).
func (d TypingDots) View(base string) string {
	if base == "" {
		return ""
	}
	return base + dotsFrames[d.frame]
}
