package components

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Severity classifies a transient message. It lives here (not statusbar.go)
// because transient messaging is now the toast component's concern.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

// ToastKind is the semantic class of a toast. The first three values match
// Severity 1:1 so ToastKindOf is a straight cast; ToastNotify extends it.
type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastWarning
	ToastError
	ToastNotify
)

// ToastKindOf maps a request-side Severity to a ToastKind.
func ToastKindOf(sev Severity) ToastKind { return ToastKind(sev) }

// ToastZone is a screen region where a stack of toasts is anchored.
type ToastZone int

const (
	ZoneBottomRight ToastZone = iota // error / warning / info
	ZoneTopRight                     // in-app message notifications
	ZoneBottomLeft                   // reserved for the future Demo keycast
)

// ToastAction is a labelled action attached to a toast. Key is the hotkey for
// the future ContextToast focus mode (informational for now). Msg is returned
// to RootModel as a command when the action is activated (mouse click today).
type ToastAction struct {
	Label string
	Key   string
	Msg   tea.Msg
}

type toast struct {
	serial  int
	kind    ToastKind
	text    string
	zone    ToastZone
	actions []ToastAction
	// clickMsg, when non-nil, makes the whole toast box a click target returning
	// this message (e.g. a notify toast that opens its chat).
	clickMsg tea.Msg
}

// ToastStack holds all active toasts in one flat slice (ordered by serial) and
// renders them grouped by zone.
type ToastStack struct {
	width, height     int
	maxVisible        int
	toasts            []toast
	serial            int
	zoneFor           map[ToastKind]ToastZone
	hasDarkBackground bool
}

// NewToastStack builds a stack. errorZone receives info/warning/error toasts;
// notifyZone receives notify toasts.
func NewToastStack(width, height, maxVisible int, errorZone, notifyZone ToastZone) *ToastStack {
	return &ToastStack{
		width:      width,
		height:     height,
		maxVisible: maxVisible,
		zoneFor: map[ToastKind]ToastZone{
			ToastInfo:    errorZone,
			ToastWarning: errorZone,
			ToastError:   errorZone,
			ToastNotify:  notifyZone,
		},
	}
}

func (s *ToastStack) SetSize(w, h int) { s.width, s.height = w, h }

// SetDarkBackground records the terminal theme so toast panel colors adapt to a
// dark vs light background.
func (s *ToastStack) SetDarkBackground(isDark bool) { s.hasDarkBackground = isDark }

// SetClick attaches a whole-box click message to the toast with the given
// serial (no-op if the serial matches nothing). Used to make a notify toast
// open its chat on click.
func (s *ToastStack) SetClick(serial int, msg tea.Msg) {
	for i := range s.toasts {
		if s.toasts[i].serial == serial {
			s.toasts[i].clickMsg = msg
			return
		}
	}
}

// Add appends a toast in the zone resolved for its kind and returns its serial.
func (s *ToastStack) Add(kind ToastKind, text string, actions ...ToastAction) int {
	s.serial++
	s.toasts = append(s.toasts, toast{
		serial:  s.serial,
		kind:    kind,
		text:    text,
		zone:    s.zoneFor[kind],
		actions: actions,
	})
	return s.serial
}

// Dismiss removes the toast with the given serial. A serial that matches no
// active toast is a no-op, so a stale auto-dismiss timer cannot remove a newer
// toast.
func (s *ToastStack) Dismiss(serial int) {
	for i, t := range s.toasts {
		if t.serial == serial {
			s.toasts = append(s.toasts[:i], s.toasts[i+1:]...)
			return
		}
	}
}

// DismissTop removes the most recently added toast. Returns false when empty.
func (s *ToastStack) DismissTop() bool {
	if len(s.toasts) == 0 {
		return false
	}
	s.toasts = s.toasts[:len(s.toasts)-1]
	return true
}

func (s *ToastStack) Empty() bool { return len(s.toasts) == 0 }

// ZoneRender is a rendered zone block with its absolute stamping origin.
type ZoneRender struct {
	Block     string
	Top, Left int
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *ToastStack) boxWidth() int { return clampInt(s.width/3, 20, 40) }

// toastBorderColor returns the border color for a kind.
func toastBorderColor(k ToastKind) color.Color {
	switch k {
	case ToastError:
		return lipgloss.Color("203") // red
	case ToastWarning:
		return lipgloss.Color("214") // amber
	case ToastNotify:
		return lipgloss.Color("75") // blue
	default:
		return lipgloss.Color("75") // info blue
	}
}

// renderToast renders one toast box (border + wrapped text + optional footer
// hints) to a multi-line string of exactly boxWidth() display cells wide. A
// theme-aware panel background makes the toast stand out from the app behind it.
func (s *ToastStack) renderToast(t toast) string {
	innerW := s.boxWidth() - 2 // account for left/right border
	bg := s.panelBg()
	fg := s.panelFg()
	body := lipgloss.NewStyle().Width(innerW).Background(bg).Foreground(fg).Render(t.text)
	if footer := s.footer(t, innerW); footer != "" {
		body = body + "\n" + footer
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(toastBorderColor(t.kind)).
		BorderBackground(bg).
		Background(bg).
		Foreground(fg).
		Width(innerW).
		Render(body)
}

// panelBg returns the toast panel background for the current theme: a shade a
// step off the usual terminal background so the toast reads as a raised panel.
func (s *ToastStack) panelBg() color.Color {
	return lipgloss.LightDark(s.hasDarkBackground)(lipgloss.Color("254"), lipgloss.Color("237"))
}

// panelFg returns readable body text for the panel background.
func (s *ToastStack) panelFg() color.Color {
	return lipgloss.LightDark(s.hasDarkBackground)(lipgloss.Color("236"), lipgloss.Color("252"))
}

// footer renders the accented action hints for a toast, or "" if none.
func (s *ToastStack) footer(t toast, innerW int) string {
	if len(t.actions) == 0 {
		return ""
	}
	bg := s.panelBg()
	base := lipgloss.NewStyle().Background(bg).Foreground(s.panelFg())
	accent := lipgloss.NewStyle().Background(bg).Foreground(lipgloss.Color("39")).Bold(true)
	parts := make([]string, 0, len(t.actions))
	for _, a := range t.actions {
		text, spans := hintLayout(a.Key, a.Label)
		parts = append(parts, applyAccent(text, spans, base, accent))
	}
	return lipgloss.NewStyle().Width(innerW).Background(bg).Render(strings.Join(parts, "  "))
}

// visibleFor returns the toasts to show for a zone (most recent maxVisible) and
// the count hidden by overflow.
func (s *ToastStack) visibleFor(zone ToastZone) (visible []toast, hidden int) {
	var all []toast
	for _, t := range s.toasts {
		if t.zone == zone {
			all = append(all, t)
		}
	}
	if len(all) <= s.maxVisible {
		return all, 0
	}
	return all[len(all)-s.maxVisible:], len(all) - s.maxVisible
}

// zoneEntry is one stacked item in a zone, in top-to-bottom draw order: either a
// rendered toast (t != nil) or the overflow "+N more" marker (t == nil).
type zoneEntry struct {
	t     *toast
	lines []string
}

// zoneLayout returns a zone's stacked entries plus the absolute origin (top,left)
// where the block is stamped. Both Zones (drawing) and actionRegions
// (hit-testing) build on it, so what is drawn and what is clickable share one
// geometry — no string/border scanning.
func (s *ToastStack) zoneLayout(zone ToastZone) (entries []zoneEntry, top, left int) {
	visible, hidden := s.visibleFor(zone)
	if len(visible) == 0 {
		return nil, 0, 0
	}
	more := zoneEntry{lines: []string{
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).
			Render(fmt.Sprintf("+%d more", hidden)),
	}}
	toEntry := func(t toast) zoneEntry {
		return zoneEntry{t: &t, lines: strings.Split(s.renderToast(t), "\n")}
	}
	switch zone {
	case ZoneBottomRight:
		// newest lowest: overflow marker on top, then oldest..newest.
		if hidden > 0 {
			entries = append(entries, more)
		}
		for _, t := range visible {
			entries = append(entries, toEntry(t))
		}
	case ZoneTopRight:
		// newest highest: newest..oldest, overflow marker at bottom.
		for i := len(visible) - 1; i >= 0; i-- {
			entries = append(entries, toEntry(visible[i]))
		}
		if hidden > 0 {
			entries = append(entries, more)
		}
	}
	blockH := 0
	for _, e := range entries {
		blockH += len(e.lines)
	}
	left = clampInt(s.width-s.boxWidth()-2, 0, s.width)
	if zone == ZoneBottomRight {
		top = s.height - blockH - 1
		if top < 0 {
			top = 0
		}
	} else {
		top = 1
	}
	return entries, top, left
}

// Zones renders every non-empty zone with its absolute stamping origin.
func (s *ToastStack) Zones() []ZoneRender {
	var out []ZoneRender
	for _, zone := range []ToastZone{ZoneBottomRight, ZoneTopRight} {
		entries, top, left := s.zoneLayout(zone)
		if len(entries) == 0 {
			continue
		}
		var lines []string
		for _, e := range entries {
			lines = append(lines, e.lines...)
		}
		out = append(out, ZoneRender{Block: strings.Join(lines, "\n"), Top: top, Left: left})
	}
	return out
}

// ClickRegion maps an absolute footer label rectangle to its action Msg.
type ClickRegion struct {
	Rect Rect
	Msg  tea.Msg
}

// actionRegions computes absolute click regions for every visible toast that
// carries actions, walking each zone's entries in draw order and summing their
// heights. A box's footer is its second-to-last line (last is the bottom
// border), so footerRow = entryTop + len(lines) - 2.
func (s *ToastStack) actionRegions() []ClickRegion {
	boxW := s.boxWidth()
	var regions []ClickRegion
	for _, zone := range []ToastZone{ZoneBottomRight, ZoneTopRight} {
		entries, top, left := s.zoneLayout(zone)
		row := top
		for _, e := range entries {
			// Footer action labels are more specific than a whole-box click, so
			// they are collected first and win in HitTest's first-match scan.
			if e.t != nil && len(e.t.actions) > 0 {
				footerRow := row + len(e.lines) - 2
				regions = append(regions, layoutActionRegions(*e.t, left+1, footerRow)...)
			}
			if e.t != nil && e.t.clickMsg != nil {
				regions = append(regions, ClickRegion{
					Rect: Rect{Top: row, Left: left, Height: len(e.lines), Width: boxW},
					Msg:  e.t.clickMsg,
				})
			}
			row += len(e.lines)
		}
	}
	return regions
}

// layoutActionRegions places each action label as a Rect on the footer row,
// left-to-right separated by two spaces (matching footer()).
func layoutActionRegions(t toast, left, row int) []ClickRegion {
	var out []ClickRegion
	x := left
	for _, a := range t.actions {
		text, _ := hintLayout(a.Key, a.Label)
		w := lipgloss.Width(text)
		out = append(out, ClickRegion{
			Rect: Rect{Top: row, Left: x, Height: 1, Width: w},
			Msg:  a.Msg,
		})
		x += w + 2 // two-space separator, matches footer()
	}
	return out
}

// HitTest returns the action Msg at absolute cell (x, y), if any. Regions may
// span multiple rows (a whole-box click target), so containment checks the full
// vertical range.
func (s *ToastStack) HitTest(x, y int) (tea.Msg, bool) {
	for _, r := range s.actionRegions() {
		rect := r.Rect
		if y >= rect.Top && y < rect.Top+rect.Height && x >= rect.Left && x < rect.Left+rect.Width {
			return r.Msg, true
		}
	}
	return nil, false
}

// HitTestRects returns the current absolute action click regions. Exposed so the
// mouse handler and tests can inspect hit targets.
func (s *ToastStack) HitTestRects() []ClickRegion { return s.actionRegions() }

// --- test-only helpers (unexported, used by toast_test.go) ---

func (s *ToastStack) actionRects() []ClickRegion { return s.actionRegions() }

func (s *ToastStack) count() int { return len(s.toasts) }

func (s *ToastStack) hasSerial(serial int) bool {
	for _, t := range s.toasts {
		if t.serial == serial {
			return true
		}
	}
	return false
}
