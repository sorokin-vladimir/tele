package components

import (
	"fmt"
	"os"
	"sort"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/sorokin-vladimir/tele/internal/settings"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

// SettingsModal shows everything tele can be configured with, in the order the
// config file has it, so that a row on screen is findable in the file by the
// same path - and changes what can be changed.
//
// Every change goes to the file and comes back: the overlay writes through the
// store and the running app is reloaded from the file, so editing here and
// editing in an editor are the same act rather than two acts that agree (ADR
// 0009).
type SettingsModal struct {
	width, height int
	offset        int
	cursor        int
	labelCol      int

	store    settings.Store
	km       keys.KeyMap
	defaults keys.KeyMap

	items []settingsItem
	// editing is the buffer for a value being typed. Numbers and text are
	// committed on confirm rather than per keystroke: a per-keystroke write
	// would put 1, 12 and 128 through the file in turn and apply each of them.
	editing *editState
	// status is what the overlay says about the last thing that happened - a
	// refusal, most usefully. It is shown where the hint is, and cleared by the
	// next key.
	status string
}

type editState struct {
	item   int
	buffer string
	// fresh means nothing has been typed yet, so the value that was already
	// there is still selected in the sense that typing replaces it. Changing 50
	// to 120 is typing 120, not backspacing twice first; correcting 120 to 125
	// is backspacing once, which is what clears fresh.
	fresh bool
}

const settingsMargin = 2

// settingsMaxWidth is wider than the help modal: these rows carry a label, a
// value and its notes, where a shortcut row carries a key and a phrase.
const settingsMaxWidth = 64

// settingsItem is one line of the overlay. Headings and blanks are items too, so
// that scrolling and cursor movement work on one list rather than on a rendering
// and a model that have to be kept in step.
type settingsItem struct {
	heading string
	blank   bool
	row     settingsRow

	// entry is the setting a row is about, when it is about one. Keybinding
	// rows and the legend have none.
	entry *settings.Entry
	// slot names which part of a setting written as a mapping this row is - the
	// dark half of a theme pair. Empty for everything else.
	slot string
}

// settingsRow is a row's content before it is padded and styled.
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

// key is what this row writes to, which is the setting's key or - for one half
// of a mapping - the slot under it.
func (i settingsItem) key() string {
	if i.entry == nil {
		return ""
	}
	if i.slot != "" {
		return i.entry.Key + "." + i.slot
	}
	return i.entry.Key
}

// selectable reports whether the cursor stops here. Read-only settings are
// included: being unable to change one is worth saying when somebody tries,
// rather than by silently refusing to be reached.
func (i settingsItem) selectable() bool { return i.entry != nil }

// NewSettingsModal builds the overlay from a store and the keymap in force.
//
// The store is asked for its entries and values rather than the config being
// read directly, because a second store - the Telegram account - is what this
// overlay is shaped for (ADR 0010).
func NewSettingsModal(store settings.Store, km, defaults keys.KeyMap, width, height int) *SettingsModal {
	s := &SettingsModal{width: width, height: height, store: store, km: km, defaults: defaults}
	s.build()
	s.cursor = s.nextSelectable(-1, 1)
	return s
}

// Refresh rebuilds the rows from the store, keeping where the person was
// looking. Called after a write, because the value that changed is on screen.
func (s *SettingsModal) Refresh() {
	cursor, offset := s.cursor, s.offset
	s.build()
	s.cursor, s.offset = cursor, offset
	s.clampCursor()
	s.clampOffset()
}

func (s *SettingsModal) build() {
	defaulting, _ := s.store.(settings.Defaulting)

	var items []settingsItem
	group, first := "", true
	for _, e := range s.store.Entries() {
		if first || e.Group != group {
			if !first {
				items = append(items, settingsItem{blank: true})
			}
			items = append(items, settingsItem{heading: groupTitle(e.Group, s.store.Origin())})
			group, first = e.Group, false
		}
		value, status := s.store.Value(e.Key)
		items = append(items, settingRows(e, value, status, defaulting)...)
	}

	// One section per context, named the way the file nests them, so that a row
	// here is found in the file at keybindings.<context>.<action>.
	for _, ctx := range keybindingContexts {
		rows := keybindingRows(ctx, s.km, s.defaults)
		if len(rows) == 0 {
			continue
		}
		items = append(items, settingsItem{blank: true},
			settingsItem{heading: "keybindings." + string(ctx)})
		for _, r := range rows {
			items = append(items, settingsItem{row: r})
		}
	}

	items = append(items, settingsItem{blank: true},
		settingsItem{heading: "what the marks mean"})
	for _, l := range markerLegend() {
		mark, meaning, _ := strings.Cut(l, "|")
		items = append(items, settingsItem{row: settingsRow{label: "[" + mark + "]", value: meaning}})
	}

	// Standing at the bottom rather than only appearing when a read-only row is
	// pressed: the file is where all of this is kept, and somebody who wants to
	// edit it in an editor should not have to press something to be told where.
	items = append(items, settingsItem{blank: true},
		settingsItem{row: settingsRow{label: "edited in", value: shortenPath(s.store.Origin())}})

	s.items = items
	s.labelCol = 0
	for _, it := range items {
		if it.heading != "" || it.blank {
			continue
		}
		if n := len(it.row.label); n > s.labelCol && n <= 26 {
			s.labelCol = n
		}
	}
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
// from a single name and the overlay has no business flattening it.
func settingRows(e settings.Entry, value any, status settings.Status, defaulting settings.Defaulting) []settingsItem {
	isDefault := defaulting != nil && defaulting.IsDefault(e.Key)

	var notes []string
	// A read-only setting is not anybody's choice to have made here, so saying
	// it is at its default would answer a question nobody asked.
	if isDefault && !e.ReadOnly {
		notes = append(notes, "default")
	}
	if marker := settingMarker(e); marker != "" {
		notes = append(notes, marker)
	}

	if pair, ok := value.(map[string]any); ok && len(e.Slots) > 0 {
		var items []settingsItem
		for _, slot := range e.Slots {
			v, present := pair[slot]
			if !present {
				continue
			}
			items = append(items, settingsItem{
				entry: &e,
				slot:  slot,
				row: settingsRow{
					label: e.Label + " (" + slot + ")",
					value: renderValue(e, v, status),
					notes: notes,
					muted: isDefault,
				},
			})
		}
		if len(items) > 0 {
			return items
		}
	}
	return []settingsItem{{
		entry: &e,
		row: settingsRow{
			label: e.Label,
			value: renderValue(e, value, status),
			notes: notes,
			muted: isDefault,
		},
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
	n, ok := asInt64(value)
	if !ok {
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

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
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
