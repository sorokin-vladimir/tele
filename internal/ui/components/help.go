package components

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	lipcompat "charm.land/lipgloss/v2/compat"

	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

type helpRow struct {
	Key  string
	Desc string
}

type helpSection struct {
	Title string
	Rows  []helpRow
}

// helpSectionSpec names a display section and the contexts it draws from (the
// first is primary; the rest are folded in, e.g. delete-submenu under the
// message menu). Order here is the modal's top-to-bottom order.
type helpSectionSpec struct {
	title    string
	contexts []keys.Context
}

var helpSectionSpecs = []helpSectionSpec{
	{"Global", []keys.Context{keys.ContextGlobal}},
	{"Folders", []keys.Context{keys.ContextFolders}},
	{"Chat list", []keys.Context{keys.ContextChatList}},
	{"Chat", []keys.Context{keys.ContextChat}},
	{"Composer", []keys.Context{keys.ContextComposer}},
	{"Search", []keys.Context{keys.ContextSearch}},
	{"Message menu", []keys.Context{keys.ContextContextMenu, keys.ContextDeleteSubMenu}},
	{"Chat menu", []keys.Context{keys.ContextChatMenu, keys.ContextFolderSubMenu}},
	{"File picker", []keys.Context{keys.ContextFilePicker}},
}

// actionDisplayOrder is the canonical top-to-bottom order of actions within a
// section (navigation first, then mode, then message actions, then close/quit).
// Actions absent here sort after, alphabetically by rank, so a new action still
// shows.
var actionDisplayOrder = []keys.Action{
	keys.ActionFocusFolders, keys.ActionFocusChatList, keys.ActionFocusChat,
	keys.ActionFocusPrev, keys.ActionFocusNext,
	keys.ActionUp, keys.ActionDown, keys.ActionCursorUp, keys.ActionCursorDown,
	keys.ActionGoTop, keys.ActionGoBottom, keys.ActionScrollHalfUp, keys.ActionScrollHalfDown,
	keys.ActionInsert, keys.ActionNormal, keys.ActionConfirm,
	keys.ActionSearch, keys.ActionOpenContextMenu,
	keys.ActionReply, keys.ActionReact, keys.ActionEdit, keys.ActionForward,
	keys.ActionCopyMessage, keys.ActionOpenInViewer, keys.ActionOpenExternal,
	keys.ActionPlayVoice, keys.ActionDownloadFile, keys.ActionJumpToOriginal,
	keys.ActionMarkRead, keys.ActionMarkUnread, keys.ActionMute, keys.ActionUnmute,
	keys.ActionArchive, keys.ActionUnarchive, keys.ActionAddToFolder, keys.ActionAttach,
	keys.ActionPasteImage, keys.ActionToggleSendAs, keys.ActionCancelUpload,
	keys.ActionDelete, keys.ActionDeleteRevoke, keys.ActionDeleteMe,
	keys.ActionDismissToast, keys.ActionShowHelp, keys.ActionCancel, keys.ActionQuit,
}

// navPairs are (down, up) actions collapsed into one "downKey/upKey" row.
var navPairs = [][2]keys.Action{
	{keys.ActionDown, keys.ActionUp},
	{keys.ActionCursorDown, keys.ActionCursorUp},
	{keys.ActionScrollHalfDown, keys.ActionScrollHalfUp},
}

func actionRank(a keys.Action) int {
	for i, x := range actionDisplayOrder {
		if x == a {
			return i
		}
	}
	return len(actionDisplayOrder)
}

// helpSections builds the ordered reference from the effective keymap. Keys come
// from KeyFor; descriptions from Describe(...).Long.
func helpSections(km keys.KeyMap) []helpSection {
	var out []helpSection
	for _, spec := range helpSectionSpecs {
		// Collect the distinct actions bound across this section's contexts,
		// remembering the context each action's key/label should come from
		// (primary context wins on conflict).
		seen := map[keys.Action]keys.Context{}
		var order []keys.Action
		for _, ctx := range spec.contexts {
			binds := km[ctx]
			acts := map[keys.Action]bool{}
			for _, a := range binds {
				acts[a] = true
			}
			var list []keys.Action
			for a := range acts {
				list = append(list, a)
			}
			sort.Slice(list, func(i, j int) bool { return list[i] < list[j] })
			for _, a := range list {
				if _, ok := seen[a]; !ok {
					seen[a] = ctx
					order = append(order, a)
				}
			}
		}

		// Drop the "up" halves of nav pairs whose "down" is also present; they
		// render as one collapsed row keyed on the down action.
		suppressed := map[keys.Action]bool{}
		for _, p := range navPairs {
			if _, hasDown := seen[p[0]]; hasDown {
				if _, hasUp := seen[p[1]]; hasUp {
					suppressed[p[1]] = true
				}
			}
		}

		var acts []keys.Action
		for _, a := range order {
			if suppressed[a] {
				continue
			}
			acts = append(acts, a)
		}
		sort.SliceStable(acts, func(i, j int) bool {
			return actionRank(acts[i]) < actionRank(acts[j])
		})

		var rows []helpRow
		for _, a := range acts {
			ctx := seen[a]
			key := km.KeyFor(ctx, a)
			if key == "" {
				continue
			}
			// Collapse a nav pair into "downKey/upKey".
			for _, p := range navPairs {
				if a == p[0] {
					if up := km.KeyFor(ctx, p[1]); up != "" {
						key = key + "/" + up
					}
				}
			}
			lbl, ok := keys.Describe(ctx, a)
			desc := string(a)
			if ok {
				desc = lbl.Long
			}
			rows = append(rows, helpRow{Key: key, Desc: desc})
		}
		if len(rows) > 0 {
			out = append(out, helpSection{Title: spec.title, Rows: rows})
		}
	}
	return out
}

// helpMargin is the terminal cells reserved around the modal on each axis.
const helpMargin = 2

// The modal is an opaque overlay: it carries its own background so text stays
// readable on both light and dark terminals (a transparent overlay left the
// light gray descriptions invisible on a light background). All colors are
// adaptive.
var (
	helpBg = lipcompat.AdaptiveColor{
		Light: lipgloss.Color("254"),
		Dark:  lipgloss.Color("235"),
	}
	helpAccentFg = lipcompat.AdaptiveColor{
		Light: lipgloss.Color("25"), // blue, dark enough on a light fill
		Dark:  lipgloss.Color("39"),
	}
	helpTextFg = lipcompat.AdaptiveColor{
		Light: lipgloss.Color("238"), // dark gray on light, light gray on dark
		Dark:  lipgloss.Color("250"),
	}
	helpBorderFg = lipcompat.AdaptiveColor{
		Light: lipgloss.Color("245"),
		Dark:  lipgloss.Color("240"),
	}

	helpBgStyle      = lipgloss.NewStyle().Background(helpBg)
	helpTitleStyle   = lipgloss.NewStyle().Background(helpBg).Foreground(helpAccentFg).Bold(true)
	helpKeyStyle     = lipgloss.NewStyle().Background(helpBg).Foreground(helpAccentFg)
	helpSectionStyle = lipgloss.NewStyle().Background(helpBg).Foreground(helpTextFg).Bold(true)
	helpDescStyle    = lipgloss.NewStyle().Background(helpBg).Foreground(helpTextFg)
)

// HelpModal is a scrollable, centered overlay listing every keyboard shortcut,
// generated from the effective keymap. It owns scroll (j/k, arrows) and closes
// on esc or '?'.
type HelpModal struct {
	lines  []string // fully rendered body lines (styled)
	width  int
	height int
	offset int
	keyCol int // left column width for keys
}

// NewHelpModal builds the modal from km at the given terminal size.
func NewHelpModal(km keys.KeyMap, width, height int) *HelpModal {
	h := &HelpModal{width: width, height: height}
	h.build(km)
	return h
}

func (h *HelpModal) SetSize(w, height int) {
	h.width = w
	h.height = height
	h.clampOffset()
}

func (h *HelpModal) build(km keys.KeyMap) {
	secs := helpSections(km)
	// Key column = widest key string, capped.
	keyCol := 3
	for _, s := range secs {
		for _, r := range s.Rows {
			if len(r.Key) > keyCol {
				keyCol = len(r.Key)
			}
		}
	}
	if keyCol > 10 {
		keyCol = 10
	}
	h.keyCol = keyCol

	var lines []string
	for i, s := range secs {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, helpSectionStyle.Render(s.Title))
		for _, r := range s.Rows {
			key := r.Key
			if len(key) > keyCol {
				key = key[:keyCol]
			}
			pad := strings.Repeat(" ", keyCol-len(key))
			// Every segment (including gaps) carries the modal background so the
			// row fill stays solid across the reset sequences between runs.
			line := helpBgStyle.Render("  ") + helpKeyStyle.Render(key) +
				helpBgStyle.Render(pad+"  ") + helpDescStyle.Render(r.Desc)
			lines = append(lines, line)
		}
	}
	h.lines = lines
	h.clampOffset()
}

// viewportH is the number of body rows visible inside the box (terminal height
// minus margins, borders, and the bottom hint).
func (h *HelpModal) viewportH() int {
	vh := h.height - 2*helpMargin - 2 /*borders*/ - 1 /*bottom hint*/
	if vh < 1 {
		vh = 1
	}
	return vh
}

func (h *HelpModal) clampOffset() {
	max := len(h.lines) - h.viewportH()
	if max < 0 {
		max = 0
	}
	if h.offset > max {
		h.offset = max
	}
	if h.offset < 0 {
		h.offset = 0
	}
}

// Update handles scroll and close keys. It returns (self, stayOpen).
func (h *HelpModal) Update(msg tea.KeyPressMsg) (*HelpModal, bool) {
	switch keys.NormalizeKey(msg.String()) {
	case "esc", "?":
		return h, false
	case "j", "down", "ctrl+j":
		h.offset++
		h.clampOffset()
	case "k", "up", "ctrl+k":
		h.offset--
		h.clampOffset()
	case "ctrl+d", "pgdown":
		h.offset += h.viewportH() / 2
		h.clampOffset()
	case "ctrl+u", "pgup":
		h.offset -= h.viewportH() / 2
		h.clampOffset()
	case "g":
		h.offset = 0
	case "G":
		h.offset = len(h.lines)
		h.clampOffset()
	}
	return h, true
}

func (h *HelpModal) View() string {
	vh := h.viewportH()
	// Inner width: terminal minus margins and borders, capped for readability.
	innerW := h.width - 2*helpMargin - 2
	if innerW > 56 {
		innerW = 56
	}
	if innerW < 20 {
		innerW = 20
	}

	end := h.offset + vh
	if end > len(h.lines) {
		end = len(h.lines)
	}
	visible := make([]string, 0, vh)
	for i := h.offset; i < end; i++ {
		// Pad each row to the inner width with the modal background.
		visible = append(visible, helpBgStyle.Width(innerW).MaxWidth(innerW).Render(h.lines[i]))
	}
	for len(visible) < vh {
		visible = append(visible, helpBgStyle.Width(innerW).Render(""))
	}

	scrollNote := ""
	if len(h.lines) > vh {
		scrollNote = helpDescStyle.Render(fmt.Sprintf(" %d-%d/%d", h.offset+1, end, len(h.lines)))
	}
	hint := OverlayHint([][2]string{{"j/k", "scroll"}, {"esc", "close"}}, helpBg) + scrollNote

	content := strings.Join(visible, "\n")
	outerW := innerW + 2
	outerH := vh + 2
	box := RenderBox(content, helpTitleStyle.Render("Keyboard shortcuts"), "", hint, "",
		lipgloss.RoundedBorder(), helpBorderFg, outerW, outerH)
	// Bake the modal background onto every box line so the border and any gaps
	// share the fill (each inner run already sets its own bg, so it survives).
	boxLines := strings.Split(box, "\n")
	for i, l := range boxLines {
		boxLines[i] = helpBgStyle.Render(l)
	}
	return strings.Join(boxLines, "\n")
}

// DescribeShort returns the Short label for action in ctx, or the raw action
// name when unlabeled (which the keys completeness test prevents in practice).
// Overlay hint bars use it so an action reads the same wording everywhere.
func DescribeShort(ctx keys.Context, action keys.Action) string {
	if lbl, ok := keys.Describe(ctx, action); ok {
		return lbl.Short
	}
	return string(action)
}
