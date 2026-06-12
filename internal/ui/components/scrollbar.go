package components

// ScrollInfo describes a scrollable viewport in row (or element) units.
type ScrollInfo struct {
	Total   int // total content rows
	Visible int // viewport rows
	Offset  int // index of the first visible row (0-based)
}

// Thumb returns the thumb placement within a track of trackLen rows.
// show is false when content fits (Total <= Visible) or the track is empty;
// in that case the caller draws a plain border. When shown, size >= 1 and the
// thumb is clamped to [0, trackLen]: start == 0 at the top, start+size == trackLen
// at the bottom.
func (s ScrollInfo) Thumb(trackLen int) (start, size int, show bool) {
	if trackLen <= 0 || s.Total <= 0 || s.Total <= s.Visible {
		return 0, 0, false
	}
	size = trackLen * s.Visible / s.Total
	if size < 1 {
		size = 1
	}
	if size > trackLen {
		size = trackLen
	}
	maxOffset := s.Total - s.Visible
	off := s.Offset
	if off < 0 {
		off = 0
	}
	if off > maxOffset {
		off = maxOffset
	}
	start = (trackLen - size) * off / maxOffset
	if start < 0 {
		start = 0
	}
	if start+size > trackLen {
		start = trackLen - size
	}
	return start, size, true
}

// Scrollbar describes a thumb to paint on a box's right border. TrackTop and
// TrackLen are 0-based, relative to the box's inner content area; they let the
// track cover a sub-range (e.g. only the message-list rows of the chat pane).
type Scrollbar struct {
	Info     ScrollInfo
	TrackTop int
	TrackLen int
}

// scrollThumbChar is the glyph drawn over the border where the thumb sits.
const scrollThumbChar = "█"
