package components

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// SettingsModal shows everything tele can be configured with, in the order the
// config file has it, so that a row on screen is findable in the file by the
// same path.
//
// It shows and does not edit. What is on screen is what the app is running on,
// which is the same thing the file says, because the file is the only way a
// setting gets set (ADR 0009).
type SettingsModal struct {
	width, height int
	offset        int
	lines         []string
	// labelCol is the width the labels are padded to, so values line up down the
	// whole overlay rather than per section.
	labelCol int
}

const settingsMargin = 2

// settingsMaxWidth is wider than the help modal: these rows carry a label, a
// value and a marker, where a shortcut row carries a key and a phrase.
const settingsMaxWidth = 64

// NewSettingsModal builds the overlay from a store and the keymap in force.
//
// The store is asked for its entries and values rather than the config being
// read directly, because a second store - the Telegram account - is what this
// overlay is shaped for (ADR 0010).
func NewSettingsModal(store settings.Store, km, defaults keys.KeyMap, width, height int) *SettingsModal {
	s := &SettingsModal{width: width, height: height}
	s.build(store, km, defaults)
	return s
}

// settingsRow is one line of the overlay before it is padded and styled.
type settingsRow struct {
	label string
	value string
	// notes are the short words after the value. Two independent things get
	// said there: whether the value is anybody's choice, and when a change to it
	// takes hold. They are independent, so they are separate notes rather than
	// one column trying to carry both.
	notes []string
	// muted dims the value of a setting nobody chose. It says the same thing as
	// the "default" note, quieter and at a glance; the note is what makes it
	// legible when the theme's contrast is low.
	muted bool
}

func (s *SettingsModal) build(store settings.Store, km, defaults keys.KeyMap) {
	type section struct {
		title string
		rows  []settingsRow
	}
	var sections []section
	current := ""
	first := true

	defaulting, _ := store.(settings.Defaulting)

	for _, e := range store.Entries() {
		if first || e.Group != current {
			sections = append(sections, section{title: groupTitle(e.Group, store.Origin())})
			current, first = e.Group, false
		}
		value, status := store.Value(e.Key)
		for _, row := range settingRows(e, value, status, defaulting) {
			last := len(sections) - 1
			sections[last].rows = append(sections[last].rows, row)
		}
	}

	// One section per context, named the way the file nests them, so that a row
	// here is found in the file at keybindings.<context>.<action>.
	for _, ctx := range keybindingContexts {
		rows := keybindingRows(ctx, km, defaults)
		if len(rows) == 0 {
			continue
		}
		sections = append(sections, section{title: "keybindings." + string(ctx), rows: rows})
	}

	s.labelCol = 0
	for _, sec := range sections {
		for _, r := range sec.rows {
			if len(r.label) > s.labelCol {
				s.labelCol = len(r.label)
			}
		}
	}
	if s.labelCol > 26 {
		s.labelCol = 26
	}

	var lines []string
	for i, sec := range sections {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, theme.S().HelpSection.Render(sec.title))
		for _, r := range sec.rows {
			lines = append(lines, s.renderRow(r))
		}
	}
	lines = append(lines, "", theme.S().HelpSection.Render("what the marks mean"))
	for _, l := range markerLegend() {
		mark, meaning, _ := strings.Cut(l, "|")
		// canvas:ok rendered through HelpBg below, so the gap carries the modal
		// surface rather than the canvas behind it.
		gap := strings.Repeat(" ", max(1, 12-len(mark)))
		lines = append(lines, theme.S().HelpBg.Render("  ")+theme.S().HelpFaint.Render("["+mark+"]")+
			theme.S().HelpBg.Render(gap)+theme.S().HelpDesc.Render(meaning))
	}
	const editedIn = "edited in "
	origin := fitValue(shortenPath(store.Origin()), s.innerWidth()-2-len(editedIn))
	lines = append(lines, "", theme.S().HelpBg.Render("  ")+
		theme.S().HelpDesc.Render(editedIn)+theme.S().HelpKey.Render(origin))

	s.lines = lines
	s.clampOffset()
}

func (s *SettingsModal) renderRow(r settingsRow) string {
	label := r.label
	if len(label) > s.labelCol {
		label = label[:s.labelCol]
	}
	// canvas:ok these spaces are rendered through HelpBg, so they carry the
	// modal surface rather than the canvas behind it.
	pad := strings.Repeat(" ", s.labelCol-len(label))

	// A row is one line. A value too long for what is left of it - a state
	// directory buried under a temp path - is shortened rather than wrapped:
	// wrapping puts the label on a line of its own and the column disappears.
	room := s.innerWidth() - 2 - s.labelCol - 2
	for _, note := range r.notes {
		room -= len(note) + 3 // the note, its brackets and the space before it
	}
	value := fitValue(r.value, room)

	valueStyle := theme.S().HelpKey
	if r.muted {
		valueStyle = theme.S().HelpDesc
	}
	line := theme.S().HelpBg.Render("  ") + theme.S().HelpDesc.Render(label) +
		theme.S().HelpBg.Render(pad+"  ") + valueStyle.Render(value)
	for _, note := range r.notes {
		// Bracketed as well as coloured. A note is about the value beside it,
		// not more of it, and colour alone does not carry that on a low-contrast
		// theme or a terminal rendering the panel flat.
		line += theme.S().HelpBg.Render(" ") + theme.S().HelpFaint.Render("["+note+"]")
	}
	return line
}

// fitValue shortens a value to the room it has. A path loses its middle rather
// than its end: the last part is what names the thing, and the first part is
// what says where it lives, so the part worth dropping is between them.
func fitValue(value string, room int) string {
	if room < 8 || xansi.StringWidth(value) <= room {
		return value
	}
	if !strings.ContainsAny(value, "/\\") {
		return xansi.Truncate(value, room, "…")
	}
	head := room/2 - 1
	tail := room - head - 1
	return xansi.Truncate(value, head, "") + "…" + value[len(value)-tail:]
}

// shortenPath writes a path the way a person says it, so that the interesting
// part of it is what survives being shortened.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + path[len(home):]
}

// groupTitle names a section on screen. The file's root has no section name, so
// it is titled after the file: those keys really are at the top of it.
func groupTitle(group, origin string) string {
	if group != "" {
		return group
	}
	if i := strings.LastIndexAny(origin, "/\\"); i >= 0 {
		return origin[i+1:]
	}
	if origin == "" {
		return "general"
	}
	return origin
}

// settingRows renders one setting. Usually one row - except a theme written as a
// dark/light pair, which is two, because that spelling means something different
// from a single name and the overlay has no business flattening it (#221).
func settingRows(e settings.Entry, value any, status settings.Status, defaulting settings.Defaulting) []settingsRow {
	isDefault := defaulting != nil && defaulting.IsDefault(e.Key)

	var notes []string
	// A read-only setting is not anybody's choice to have made here, so saying
	// it is at its default would be answering a question nobody asked.
	if isDefault && !e.ReadOnly {
		notes = append(notes, "default")
	}
	if marker := settingMarker(e); marker != "" {
		notes = append(notes, marker)
	}

	if pair, ok := value.(map[string]any); ok && e.Widget == settings.Text {
		var rows []settingsRow
		for _, slot := range []string{"dark", "light"} {
			if v, ok := pair[slot]; ok {
				rows = append(rows, settingsRow{
					label: e.Label + " (" + slot + ")",
					value: renderValue(e, v, status),
					notes: notes,
					muted: isDefault,
				})
			}
		}
		if len(rows) > 0 {
			return rows
		}
	}
	return []settingsRow{{
		label: e.Label,
		value: renderValue(e, value, status),
		notes: notes,
		muted: isDefault,
	}}
}

// settingMarker is the note beside a row. Immediate settings carry none: taking
// effect at once is what a person already expects, and marking every such row
// would put a mark on most of the overlay to say that nothing is unusual.
func settingMarker(e settings.Entry) string {
	if e.ReadOnly {
		return "read-only"
	}
	switch e.Applies {
	case settings.NextUse:
		return "next"
	case settings.Startup:
		return "restart"
	}
	return ""
}

// markerLegend pairs each mark with what it means, separated by a pipe so the
// two halves can be styled apart the way they are on a row.
func markerLegend() []string {
	return []string{
		"default|nobody chose this; it is what tele picked",
		"next|takes effect the next time tele does that thing",
		"restart|takes effect the next time tele starts",
		"read-only|changed by editing the file, not from here",
	}
}

// renderValue puts a value into the words the setting is thought about in: a
// toggle is on or off, a size is megabytes, and a credential is not shown at
// all.
func renderValue(e settings.Entry, value any, status settings.Status) string {
	if status == settings.Unknown {
		return "…"
	}
	if e.Secret {
		if value == nil || value == "" {
			return "not set"
		}
		return "••••••••"
	}
	switch e.Widget {
	case settings.Toggle:
		if b, ok := value.(bool); ok && b {
			return "on"
		}
		return "off"
	case settings.Bytes:
		return humanBytes(value)
	case settings.Number:
		if e.Unit != "" {
			return fmt.Sprintf("%v %s", value, e.Unit)
		}
		return fmt.Sprintf("%v", value)
	}
	if value == nil || value == "" {
		return "not set"
	}
	if s, ok := value.(string); ok && strings.ContainsAny(s, "/\\") {
		return shortenPath(s)
	}
	return fmt.Sprintf("%v", value)
}

// humanBytes shows a cache bound in the unit a person picks it in. Zero is a
// real answer and says what it means rather than showing "0 MB".
func humanBytes(value any) string {
	var n int64
	switch v := value.(type) {
	case int64:
		n = v
	case int:
		n = int64(v)
	default:
		return fmt.Sprintf("%v", value)
	}
	switch {
	case n == 0:
		return "nothing kept"
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%d MB", n/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// keybindingContexts is the top-to-bottom order the bindings are listed in.
var keybindingContexts = []keys.Context{
	keys.ContextGlobal, keys.ContextFolders, keys.ContextChatList, keys.ContextChat,
	keys.ContextComposer, keys.ContextSearch, keys.ContextContextMenu,
	keys.ContextDeleteSubMenu, keys.ContextChatMenu, keys.ContextFolderSubMenu,
	keys.ContextFilePicker,
}

// keybindingRows lists what is bound to what, marking the ones this config
// changed and saying what they used to be.
//
// The whole resolved set rather than only the overrides: "what is bound to what"
// is a question about all of it, and an overlay showing three lines of a config
// that has forty would be describing a part as if it were the whole.
func keybindingRows(ctx keys.Context, km, defaults keys.KeyMap) []settingsRow {
	bound := keysByAction(km[ctx])
	if len(bound) == 0 {
		return nil
	}
	was := keysByAction(defaults[ctx])

	actions := make([]keys.Action, 0, len(bound))
	for a := range bound {
		actions = append(actions, a)
	}
	sort.Slice(actions, func(i, j int) bool { return actions[i] < actions[j] })

	rows := make([]settingsRow, 0, len(actions))
	for _, a := range actions {
		row := settingsRow{
			label: string(a),
			value: strings.Join(bound[a], " "),
		}
		switch before, ok := was[a]; {
		case ok && !equalKeys(before, bound[a]):
			row.notes = []string{"was " + strings.Join(before, " ")}
		case !ok:
			row.notes = []string{"added"}
		}
		rows = append(rows, row)
	}
	return rows
}

// keysByAction inverts a context's bindings: the keymap is keyed by key because
// that is how a press is resolved, and this overlay reads by action.
func keysByAction(bindings map[string]keys.Action) map[keys.Action][]string {
	if len(bindings) == 0 {
		return nil
	}
	out := make(map[keys.Action][]string, len(bindings))
	for key, action := range bindings {
		out[action] = append(out[action], key)
	}
	for a := range out {
		sort.Strings(out[a])
	}
	return out
}

func equalKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// viewportH is the number of body rows visible inside the box.
func (s *SettingsModal) viewportH() int {
	vh := s.height - 2*settingsMargin - 2 /*borders*/ - 1 /*bottom hint*/
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (s *SettingsModal) clampOffset() {
	max := len(s.lines) - s.viewportH()
	if max < 0 {
		max = 0
	}
	if s.offset > max {
		s.offset = max
	}
	if s.offset < 0 {
		s.offset = 0
	}
}

// Update handles scroll and close keys. It returns (self, stayOpen), the same
// shape the help modal uses, because an overlay that only reads has the same
// job.
func (s *SettingsModal) Update(msg tea.KeyPressMsg) (*SettingsModal, bool) {
	switch keys.NormalizeKey(msg.String()) {
	case "esc", ",":
		return s, false
	case "j", "down", "ctrl+j":
		s.offset++
		s.clampOffset()
	case "k", "up", "ctrl+k":
		s.offset--
		s.clampOffset()
	case "ctrl+d", "pgdown":
		s.offset += s.viewportH() / 2
		s.clampOffset()
	case "ctrl+u", "pgup":
		s.offset -= s.viewportH() / 2
		s.clampOffset()
	case "g":
		s.offset = 0
	case "G":
		s.offset = len(s.lines)
		s.clampOffset()
	}
	return s, true
}

// innerWidth is the room inside the box, which both the rows and the box itself
// are built to.
func (s *SettingsModal) innerWidth() int {
	w := s.width - 2*settingsMargin - 2
	if w > settingsMaxWidth {
		w = settingsMaxWidth
	}
	if w < 20 {
		w = 20
	}
	return w
}

func (s *SettingsModal) View() string {
	vh := s.viewportH()
	innerW := s.innerWidth()

	end := s.offset + vh
	if end > len(s.lines) {
		end = len(s.lines)
	}
	visible := make([]string, 0, vh)
	for i := s.offset; i < end; i++ {
		visible = append(visible, theme.S().HelpBg.Width(innerW).MaxWidth(innerW).Render(s.lines[i]))
	}
	for len(visible) < vh {
		visible = append(visible, theme.S().HelpBg.Width(innerW).Render(""))
	}

	scrollNote := ""
	if len(s.lines) > vh {
		scrollNote = theme.S().HelpDesc.Render(fmt.Sprintf(" %d-%d/%d", s.offset+1, end, len(s.lines)))
	}
	hint := OverlayHint([][2]string{{"j/k", "scroll"}, {"esc", "close"}}, theme.T().SurfaceHelp) + scrollNote

	box := RenderBox(strings.Join(visible, "\n"), theme.S().HelpTitle.Render("Settings"), "", hint, "",
		lipgloss.RoundedBorder(), theme.T().BorderOverlay, innerW+2, vh+2)
	boxLines := strings.Split(box, "\n")
	for i, l := range boxLines {
		boxLines[i] = theme.S().HelpBg.Render(l)
	}
	return strings.Join(boxLines, "\n")
}

// LinesForTest exposes the rendered rows so a test can read what is on screen
// without parsing a box.
func (s *SettingsModal) LinesForTest() []string { return s.lines }
