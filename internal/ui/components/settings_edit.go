package components

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// SettingsResult is what the overlay tells the app after a key.
type SettingsResult struct {
	// Open is false once the overlay has been dismissed.
	Open bool
	// Changed reports that a setting was written, so the app should apply what
	// is now on disk. The overlay has already refreshed itself; what it cannot
	// do is reach the rest of the running app.
	Changed bool
}

// Update handles moving, editing and closing.
//
// A toggle and a choice commit as soon as they are pressed: there is no
// half-pressed state to confirm. A number or a piece of text is typed and then
// confirmed, because a per-keystroke write would put 1, 12 and 128 through the
// file in turn and apply each one on the way - and, once a setting can live on
// the account, earn a flood wait for typing a number.
func (s *SettingsModal) Update(msg tea.KeyPressMsg) (*SettingsModal, SettingsResult) {
	key := keys.NormalizeKey(msg.String())
	if s.editing != nil {
		return s.updateEditing(msg, key)
	}
	s.status = ""

	switch key {
	case "esc", ",":
		return s, SettingsResult{}
	case "j", "down", "ctrl+j":
		s.moveCursor(1)
	case "k", "up", "ctrl+k":
		s.moveCursor(-1)
	case "ctrl+d", "pgdown":
		s.scroll(s.viewportH() / 2)
	case "ctrl+u", "pgup":
		s.scroll(-s.viewportH() / 2)
	case "g":
		s.cursor = s.nextSelectable(-1, 1)
		s.offset = 0
	case "G":
		s.cursor = s.nextSelectable(len(s.items), -1)
		s.offset = len(s.items)
		s.clampOffset()
	case "enter", " ", "space":
		return s.activate()
	case "left", "h":
		return s.cycle(-1)
	case "right", "l":
		return s.cycle(1)
	case "r":
		return s.reset()
	}
	s.showCursor()
	return s, SettingsResult{Open: true}
}

// activate changes the setting under the cursor: a toggle flips, a choice moves
// on, and anything typed opens for typing.
func (s *SettingsModal) activate() (*SettingsModal, SettingsResult) {
	it, ok := s.current()
	if !ok {
		return s, SettingsResult{Open: true}
	}
	if it.entry.ReadOnly {
		s.status = "kept in " + s.store.Origin()
		return s, SettingsResult{Open: true}
	}
	switch it.entry.Widget {
	case settings.Toggle:
		on, _ := s.currentValue().(bool)
		return s.commit(!on)
	case settings.Choice:
		return s.cycle(1)
	default:
		s.editing = &editState{item: s.cursor, buffer: s.editableValue(*it), fresh: true}
		return s, SettingsResult{Open: true}
	}
}

// cycle moves a choice along its list, wrapping, so one key gets to any of them.
func (s *SettingsModal) cycle(by int) (*SettingsModal, SettingsResult) {
	it, ok := s.current()
	if !ok || it.entry.ReadOnly || it.entry.Widget != settings.Choice {
		return s, SettingsResult{Open: true}
	}
	choices := it.entry.Choices
	if len(choices) == 0 {
		return s, SettingsResult{Open: true}
	}
	at := slices.Index(choices, fmt.Sprintf("%v", s.currentValue()))
	next := ((at+by)%len(choices) + len(choices)) % len(choices)
	return s.commit(choices[next])
}

// reset writes absence. A setting the file does not name takes its default, so
// removing the key is what "put this back" means - and it keeps the file from
// filling up with explicit copies of every default.
func (s *SettingsModal) reset() (*SettingsModal, SettingsResult) {
	it, ok := s.current()
	if !ok {
		return s, SettingsResult{Open: true}
	}
	if it.entry.ReadOnly {
		s.status = "kept in " + s.store.Origin()
		return s, SettingsResult{Open: true}
	}
	return s.commit(nil)
}

// updateEditing runs the little text field a number or a name is typed into.
func (s *SettingsModal) updateEditing(msg tea.KeyPressMsg, key string) (*SettingsModal, SettingsResult) {
	switch key {
	case "esc":
		// The value goes back to what it was. Nothing was written, because
		// nothing is written until it is confirmed.
		s.editing = nil
		s.status = ""
		return s, SettingsResult{Open: true}
	case "enter":
		// The field stays open if what is in it is refused. The person is in
		// the middle of typing a value; taking the field away and making them
		// start again is a punishment for a typo.
		m, res := s.commitTyped(s.editing.buffer)
		if res.Changed {
			m.editing = nil
		}
		return m, res
	case "backspace":
		s.editing.fresh = false
		if b := s.editing.buffer; b != "" {
			s.editing.buffer = b[:len(b)-1]
		}
		return s, SettingsResult{Open: true}
	}
	if msg.Text != "" {
		if s.editing.fresh {
			s.editing.buffer = ""
			s.editing.fresh = false
		}
		s.editing.buffer += msg.Text
	}
	return s, SettingsResult{Open: true}
}

// commitTyped turns what was typed into the value the setting is kept in.
func (s *SettingsModal) commitTyped(buffer string) (*SettingsModal, SettingsResult) {
	it, ok := s.current()
	if !ok {
		return s, SettingsResult{Open: true}
	}
	buffer = strings.TrimSpace(buffer)
	if buffer == "" {
		// Emptying the field is how a setting is put back to its default from
		// the keyboard, and it means the same thing as the reset key.
		return s.commit(nil)
	}

	switch it.entry.Widget {
	case settings.Number:
		n, err := strconv.ParseInt(buffer, 10, 64)
		if err != nil {
			s.status = "must be a whole number"
			return s, SettingsResult{Open: true}
		}
		return s.commit(int(n))
	case settings.Bytes:
		mb, err := strconv.ParseInt(buffer, 10, 64)
		if err != nil {
			s.status = "must be a whole number of megabytes"
			return s, SettingsResult{Open: true}
		}
		if mb < 0 {
			s.status = "must be at least 0"
			return s, SettingsResult{Open: true}
		}
		return s.commit(mb << 20)
	}
	return s.commit(buffer)
}

// commit writes a value through the store and refreshes what is on screen.
//
// A refusal stops here and is said here, on the row it is about, rather than
// being written and complained about afterwards. The store refuses before it
// opens the file, so an illegal value never reaches the disk.
func (s *SettingsModal) commit(value any) (*SettingsModal, SettingsResult) {
	it, ok := s.current()
	if !ok {
		return s, SettingsResult{Open: true}
	}
	if err := s.store.Set(it.key(), value); err != nil {
		s.status = refusalText(err, it.key())
		return s, SettingsResult{Open: true}
	}
	s.Refresh()
	s.showCursor()
	return s, SettingsResult{Open: true, Changed: true}
}

// refusalText drops the key from the front of a refusal. The row it is shown on
// already says which setting this is about, and repeating the dotted path there
// spends the width on something already on screen.
func refusalText(err error, key string) string {
	text := err.Error()
	return strings.TrimPrefix(text, key+": ")
}

// currentValue is what the row under the cursor is currently worth, following
// into a mapping when the row is one half of one.
func (s *SettingsModal) currentValue() any {
	it, ok := s.current()
	if !ok {
		return nil
	}
	value, _ := s.store.Value(it.entry.Key)
	if it.slot == "" {
		return value
	}
	if pair, ok := value.(map[string]any); ok {
		return pair[it.slot]
	}
	return nil
}

// editableValue is what a typed field starts with: the value as it is written
// rather than as it is displayed, because what is typed replaces it. A size is
// the exception - it is displayed and typed in megabytes.
func (s *SettingsModal) editableValue(it settingsItem) string {
	value := s.currentValue()
	if value == nil {
		return ""
	}
	if it.entry.Widget == settings.Bytes {
		if n, ok := asInt64(value); ok {
			return strconv.FormatInt(n>>20, 10)
		}
	}
	return fmt.Sprintf("%v", value)
}

func (s *SettingsModal) current() (*settingsItem, bool) {
	if s.cursor < 0 || s.cursor >= len(s.items) || !s.items[s.cursor].selectable() {
		return nil, false
	}
	return &s.items[s.cursor], true
}

// nextSelectable finds the next row the cursor can sit on, searching in the
// given direction. Headings, blanks, keybindings and the legend are read, not
// visited.
func (s *SettingsModal) nextSelectable(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(s.items); i += dir {
		if s.items[i].selectable() {
			return i
		}
	}
	return from
}

func (s *SettingsModal) moveCursor(dir int) {
	s.cursor = s.nextSelectable(s.cursor, dir)
	s.showCursor()
}

func (s *SettingsModal) clampCursor() {
	if s.cursor >= len(s.items) {
		s.cursor = len(s.items) - 1
	}
	if s.cursor < 0 || !s.items[s.cursor].selectable() {
		s.cursor = s.nextSelectable(s.cursor, 1)
	}
}

// scroll moves the view and takes the cursor with it, so the cursor is never
// somewhere the person cannot see.
func (s *SettingsModal) scroll(by int) {
	s.offset += by
	s.clampOffset()
	if s.cursor < s.offset || s.cursor >= s.offset+s.viewportH() {
		dir := 1
		if by < 0 {
			dir = -1
			s.cursor = s.offset + s.viewportH()
		} else {
			s.cursor = s.offset - 1
		}
		s.cursor = s.nextSelectable(s.cursor, dir)
	}
}

// showCursor scrolls just enough to bring the cursor into view.
func (s *SettingsModal) showCursor() {
	vh := s.viewportH()
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+vh {
		s.offset = s.cursor - vh + 1
	}
	s.clampOffset()
}
