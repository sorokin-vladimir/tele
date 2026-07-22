package components

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
)

var (
	barBg    = lipgloss.Color("236")
	barFg    = lipgloss.Color("252")
	barStyle = lipgloss.NewStyle().Background(barBg).Foreground(barFg)

	barSepStyle = lipgloss.NewStyle().Background(barBg).Foreground(lipgloss.Color("240"))

	modeBase   = lipgloss.NewStyle().Bold(true).Padding(0, 1).Foreground(lipgloss.Color("231"))
	normalMode = modeBase.Background(lipgloss.Color("33")) // blue
	insertMode = modeBase.Background(lipgloss.Color("35")) // green
)

type StatusBar struct {
	width      int
	mode       keys.VimMode
	status     string
	verbose    bool
	lastKey    string
	activePane string
	keyMap     keys.KeyMap
	dlText     string  // active download indicator label, "" when idle
	dlSerial   int     // identifies the active download, for matched clears
	dlSpinner  Spinner // ping-pong spinner animated by TickDownloadSpinner
	// attachStaged is true while a file is staged in the composer (chip shown);
	// pickerOpen is true while the file-picker overlay is open. Both drive hints.
	attachStaged bool
	pickerOpen   bool
}

func NewStatusBar(width int) *StatusBar {
	return &StatusBar{width: width, mode: keys.ModeNormal}
}

func (sb *StatusBar) SetWidth(w int)           { sb.width = w }
func (sb *StatusBar) SetMode(m keys.VimMode)   { sb.mode = m }
func (sb *StatusBar) SetStatus(s string)       { sb.status = s }
func (sb *StatusBar) SetVerbose(v bool)        { sb.verbose = v }
func (sb *StatusBar) SetLastKey(k string)      { sb.lastKey = k }
func (sb *StatusBar) SetActivePane(p string)   { sb.activePane = p }
func (sb *StatusBar) SetKeyMap(km keys.KeyMap) { sb.keyMap = km }
func (sb *StatusBar) SetAttachStaged(v bool)   { sb.attachStaged = v }
func (sb *StatusBar) SetPickerOpen(v bool)     { sb.pickerOpen = v }

// StartDownload shows a transient, animated download indicator with label and
// returns the serial identifying it, so a later ClearDownload only clears this
// exact download (a newer StartDownload supersedes it).
func (sb *StatusBar) StartDownload(label string) int {
	sb.dlSerial++
	sb.dlText = label
	return sb.dlSerial
}

// DownloadActive reports whether a download indicator (animated spinner) is
// currently shown. Drives the spinner tick loop (issue #147).
func (sb *StatusBar) DownloadActive() bool { return sb.dlText != "" }

// ClearDownload clears the indicator only when serial matches the current one,
// so a stale or superseded completion cannot wipe a newer download's indicator.
func (sb *StatusBar) ClearDownload(serial int) {
	if serial == sb.dlSerial {
		sb.dlText = ""
	}
}

// TickDownloadSpinner advances the download indicator's spinner one frame.
func (sb *StatusBar) TickDownloadSpinner() {
	sb.dlSpinner.Tick()
}

func (sb *StatusBar) View() string {
	modeStyle := normalMode
	label := "NORMAL"
	if sb.mode == keys.ModeInsert {
		modeStyle = insertMode
		label = "INSERT"
	}

	segs := []string{modeStyle.Render(label)}

	if sb.dlText != "" {
		segs = append(segs, barStyle.Render(sb.dlSpinner.View()+" "+sb.dlText))
	} else if sb.status != "" {
		segs = append(segs, barStyle.Render(sb.status))
	}
	if h := sb.hints(); h != "" {
		segs = append(segs, barStyle.Render(h))
	}
	if sb.verbose {
		segs = append(segs, barStyle.Render(fmt.Sprintf("pane:%s key:%s", sb.activePane, sb.lastKey)))
	}

	sep := barSepStyle.Render(" │ ")
	return barStyle.Width(sb.width).Render(strings.Join(segs, sep))
}

// footerKind classifies a footer hint item.
type footerKind int

const (
	fiSingle footerKind = iota
	fiNav
	fiLiteral
)

// footerItem is one hint in a status-bar profile. For fiSingle/fiNav the label
// comes from keys.Describe(ctx, action) unless labelOverride is set; keys come
// from KeyFor. For fiLiteral, keyword is the accented word and text its
// description.
type footerItem struct {
	kind          footerKind
	ctx           keys.Context
	action        keys.Action // fiSingle
	down, up      keys.Action // fiNav
	keyword, text string      // fiLiteral
	labelOverride string      // optional wording override (state-specific)
}

// hints selects the profile for the current state and renders it. Wording is
// sourced from keys.Describe so it never drifts from the bindings.
func (sb *StatusBar) hints() string {
	if sb.keyMap == nil {
		return ""
	}
	a := sb.accentStyle()
	items := sb.profile()
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, sb.renderFooterItem(it, a))
	}
	return joinHints(parts...)
}

func (sb *StatusBar) profile() []footerItem {
	switch {
	case sb.pickerOpen:
		return []footerItem{
			{kind: fiLiteral, keyword: "type", text: "filter"},
			{kind: fiSingle, ctx: keys.ContextFilePicker, action: keys.ActionConfirm},
			{kind: fiSingle, ctx: keys.ContextFilePicker, action: keys.ActionCancel},
		}
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert && sb.attachStaged:
		return []footerItem{
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionConfirm},
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionToggleSendAs},
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionNormal},
		}
	case sb.activePane == "chat" && sb.attachStaged:
		return []footerItem{
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionInsert, labelOverride: "caption"},
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionCancelUpload},
		}
	case sb.activePane == "folders":
		return []footerItem{
			{kind: fiNav, ctx: keys.ContextFolders, down: keys.ActionDown, up: keys.ActionUp},
			{kind: fiSingle, ctx: keys.ContextFolders, action: keys.ActionConfirm},
			{kind: fiSingle, ctx: keys.ContextGlobal, action: keys.ActionQuit},
		}
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert:
		return []footerItem{
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionConfirm},
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionPasteImage},
			{kind: fiSingle, ctx: keys.ContextComposer, action: keys.ActionNormal},
		}
	case sb.activePane == "chat":
		return []footerItem{
			{kind: fiNav, ctx: keys.ContextChat, down: keys.ActionDown, up: keys.ActionUp},
			{kind: fiNav, ctx: keys.ContextChat, down: keys.ActionCursorDown, up: keys.ActionCursorUp},
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionInsert},
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionAttach},
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionOpenInViewer},
			{kind: fiSingle, ctx: keys.ContextChat, action: keys.ActionCopyMessage},
			{kind: fiSingle, ctx: keys.ContextGlobal, action: keys.ActionQuit},
		}
	case sb.activePane == "chatlist":
		return []footerItem{
			{kind: fiNav, ctx: keys.ContextChatList, down: keys.ActionDown, up: keys.ActionUp},
			{kind: fiSingle, ctx: keys.ContextChatList, action: keys.ActionConfirm},
			{kind: fiSingle, ctx: keys.ContextChatList, action: keys.ActionSearch},
			{kind: fiSingle, ctx: keys.ContextGlobal, action: keys.ActionQuit},
		}
	}
	return nil
}

func (sb *StatusBar) renderFooterItem(it footerItem, accent lipgloss.Style) string {
	switch it.kind {
	case fiLiteral:
		return hintLiteral(it.keyword, it.text, accent)
	case fiNav:
		desc := it.labelOverride
		if desc == "" {
			if lbl, ok := keys.Describe(it.ctx, it.down); ok {
				desc = lbl.Short
			}
		}
		downKey := sb.keyMap.KeyFor(it.ctx, it.down)
		upKey := sb.keyMap.KeyFor(it.ctx, it.up)
		return hintNav(downKey, upKey, desc, accent)
	default: // fiSingle
		desc := it.labelOverride
		if desc == "" {
			if lbl, ok := keys.Describe(it.ctx, it.action); ok {
				desc = lbl.Short
			}
		}
		key := sb.keyMap.KeyFor(it.ctx, it.action)
		return hintKey(key, desc, accent)
	}
}

func hintKey(key, desc string, accent lipgloss.Style) string {
	if key == "" {
		return ""
	}
	text, spans := hintLayout(key, desc)
	return applyAccent(text, spans, barStyle, accent)
}

func hintNav(downKey, upKey, desc string, accent lipgloss.Style) string {
	text, spans := navLayout(downKey, upKey, desc)
	if text == "" {
		return ""
	}
	return applyAccent(text, spans, barStyle, accent)
}

// hintLiteral renders a non-key keyword (e.g. the picker's "type" filter hint)
// as an accented indicator followed by the plain description.
func hintLiteral(keyword, desc string, accent lipgloss.Style) string {
	return applyAccent(keyword+" "+desc, []span{{0, utf8.RuneCountInString(keyword)}}, barStyle, accent)
}

func joinHints(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, barStyle.Render(" · "))
}

// span is a rune range [lo,hi) within a hint's visible text that should be
// rendered in the accent color.
type span struct{ lo, hi int }

const enterGlyph = "↵"

// hintLayout computes the visible text for a single-key hint plus the accent
// spans, implementing the btop rules: a single letter present in the word is
// highlighted in place; enter/return becomes a trailing glyph; otherwise the
// key is rendered as an accented prefix.
func hintLayout(key, desc string) (string, []span) {
	switch key {
	case "":
		return desc, nil
	case "enter", "return":
		text := desc + " " + enterGlyph
		lo := utf8.RuneCountInString(desc) + 1
		return text, []span{{lo, lo + 1}}
	}
	if utf8.RuneCountInString(key) == 1 {
		r, _ := utf8.DecodeRuneInString(key)
		if unicode.IsLetter(r) {
			if i := wordRuneIndex(desc, r); i >= 0 {
				// Show the highlighted letter in the key's exact case so it reads as
				// the actual keystroke: "Reply" with key r renders "reply", while
				// "Open photo externally" with the Shift key O keeps its capital O.
				rs := []rune(desc)
				rs[i] = r
				return string(rs), []span{{i, i + 1}}
			}
		}
	}
	// Prefix form: accented key, then the plain word.
	return key + " " + desc, []span{{0, utf8.RuneCountInString(key)}}
}

// wordRuneIndex returns the rune index of the first case-insensitive match of
// r in word, or -1 when absent.
func wordRuneIndex(word string, r rune) int {
	target := unicode.ToLower(r)
	for i, c := range []rune(word) {
		if unicode.ToLower(c) == target {
			return i
		}
	}
	return -1
}

// navLayout computes the visible text and accent spans for a navigation pair
// (down/up keys sharing one description). A vertical arrow pair renders as
// "↑ desc ↓" glyphs; any other pair renders as an accented "down/up" prefix
// with a collapsed shared modifier (ctrl+j / ctrl+k -> ctrl+j/k).
func navLayout(downKey, upKey, desc string) (string, []span) {
	if downKey == "" && upKey == "" {
		return "", nil
	}
	if downKey == "down" && upKey == "up" {
		text := "↑ " + desc + " ↓"
		hi := utf8.RuneCountInString(text)
		return text, []span{{0, 1}, {hi - 1, hi}}
	}
	combo := downKey + "/" + upKey
	if i := strings.LastIndex(downKey, "+"); i >= 0 {
		prefix := downKey[:i+1]
		if strings.HasPrefix(upKey, prefix) {
			combo = downKey + "/" + upKey[len(prefix):]
		}
	}
	return combo + " " + desc, []span{{0, utf8.RuneCountInString(combo)}}
}

// applyAccent renders each accent span of text in the accent style and every
// other run in the base style. Each run sets its own colors so they survive the
// reset sequences emitted between runs. Spans must be sorted and non-overlapping.
func applyAccent(text string, spans []span, base, accent lipgloss.Style) string {
	if len(spans) == 0 {
		return base.Render(text)
	}
	rs := []rune(text)
	var b strings.Builder
	i := 0
	for _, sp := range spans {
		if sp.lo > i {
			b.WriteString(base.Render(string(rs[i:sp.lo])))
		}
		b.WriteString(accent.Render(string(rs[sp.lo:sp.hi])))
		i = sp.hi
	}
	if i < len(rs) {
		b.WriteString(base.Render(string(rs[i:])))
	}
	return b.String()
}

// accentStyle returns the key-accent style for the current vim mode: bright
// blue in NORMAL, bright green in INSERT, both over the bar background.
func (sb *StatusBar) accentStyle() lipgloss.Style {
	fg := lipgloss.Color("39") // bright blue — NORMAL
	if sb.mode == keys.ModeInsert {
		fg = lipgloss.Color("40") // bright green — INSERT
	}
	return lipgloss.NewStyle().Background(barBg).Foreground(fg)
}
