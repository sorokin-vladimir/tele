package components

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/ui/keys"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
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
	version      string // build version shown at the right edge, "" hides it
	// clockSkew says the clock is too far off for Telegram (#277). It is apart
	// from status because status is cleared on every key press, and this lasts
	// until the skew is over.
	clockSkew string
}

func NewStatusBar(width int) *StatusBar {
	return &StatusBar{width: width, mode: keys.ModeNormal}
}

func (sb *StatusBar) SetWidth(w int)           { sb.width = w }
func (sb *StatusBar) SetMode(m keys.VimMode)   { sb.mode = m }
func (sb *StatusBar) SetStatus(s string)       { sb.status = s }
func (sb *StatusBar) Status() string           { return sb.status }
func (sb *StatusBar) SetVerbose(v bool)        { sb.verbose = v }
func (sb *StatusBar) SetLastKey(k string)      { sb.lastKey = k }
func (sb *StatusBar) SetActivePane(p string)   { sb.activePane = p }
func (sb *StatusBar) SetKeyMap(km keys.KeyMap) { sb.keyMap = km }
func (sb *StatusBar) SetAttachStaged(v bool)   { sb.attachStaged = v }
func (sb *StatusBar) SetPickerOpen(v bool)     { sb.pickerOpen = v }
func (sb *StatusBar) SetVersion(v string)      { sb.version = v }

// SetClockSkew shows s right after the mode label until it is set to "".
func (sb *StatusBar) SetClockSkew(s string) { sb.clockSkew = s }

// StartTransfer shows a transient, animated transfer indicator with label and
// returns the serial identifying it, so a later UpdateTransfer/ClearTransfer
// only touches this exact transfer (a newer StartTransfer supersedes it).
// Downloads and uploads share the slot: only one long transfer is shown at a
// time.
func (sb *StatusBar) StartTransfer(label string) int {
	sb.dlSerial++
	sb.dlText = label
	return sb.dlSerial
}

// UpdateTransfer replaces the label of the active transfer, ignoring a stale or
// superseded serial. Used to tick a percentage without restarting the spinner.
func (sb *StatusBar) UpdateTransfer(serial int, label string) {
	if serial == sb.dlSerial {
		sb.dlText = label
	}
}

// ClearTransfer clears the indicator only when serial matches the current one,
// so a stale or superseded completion cannot wipe a newer transfer's indicator.
func (sb *StatusBar) ClearTransfer(serial int) {
	if serial == sb.dlSerial {
		sb.dlText = ""
	}
}

// StartDownload is the download-side name for StartTransfer.
func (sb *StatusBar) StartDownload(label string) int { return sb.StartTransfer(label) }

// DownloadActive reports whether a download indicator (animated spinner) is
// currently shown. Drives the spinner tick loop (issue #147).
func (sb *StatusBar) DownloadActive() bool { return sb.dlText != "" }

// ClearDownload is the download-side name for ClearTransfer.
func (sb *StatusBar) ClearDownload(serial int) { sb.ClearTransfer(serial) }

// TickDownloadSpinner advances the download indicator's spinner one frame.
func (sb *StatusBar) TickDownloadSpinner() {
	sb.dlSpinner.Tick()
}

func (sb *StatusBar) View() string {
	modeStyle := theme.S().ModeNormal
	label := "NORMAL"
	if sb.mode == keys.ModeInsert {
		modeStyle = theme.S().ModeInsert
		label = "INSERT"
	}

	segs := []string{modeStyle.Render(label)}

	// First after the mode, because it explains why nothing else on screen is
	// moving. In the bar's own colours: the theme's warning colour is chosen
	// against the canvas, and the bar is a surface of its own.
	if sb.clockSkew != "" {
		segs = append(segs, theme.S().Bar.Render(sb.clockSkew))
	}
	if sb.dlText != "" {
		segs = append(segs, theme.S().Bar.Render(sb.dlSpinner.View()+" "+sb.dlText))
	} else if sb.status != "" {
		segs = append(segs, theme.S().Bar.Render(sb.status))
	}
	if h := sb.hints(); h != "" {
		segs = append(segs, theme.S().Bar.Render(h))
	}
	if sb.verbose {
		segs = append(segs, theme.S().Bar.Render(fmt.Sprintf("pane:%s key:%s", sb.activePane, sb.lastKey)))
	}

	sep := theme.S().BarSep.Render(" │ ")
	left := strings.Join(segs, sep)

	// The version sits flush right, separated from the hints by at least one
	// space. It is dropped first when the bar runs out of room: hints and
	// transfer progress are worth more than the build number.
	if ver := versionLabel(sb.version); ver != "" {
		if gap := sb.width - lipgloss.Width(left) - lipgloss.Width(ver); gap >= 1 {
			// Filler and version each set their own colors: the segments before
			// them end with a reset, so an enclosing style would not survive.
			// canvas:ok rendered through Bar, which paints the status-bar surface.
			return left + theme.S().Bar.Render(strings.Repeat(" ", gap)) + theme.S().Bar.Render(ver)
		}
	}
	return theme.S().Bar.Width(sb.width).Render(left)
}

// versionLabel formats the build version for the bar: local builds keep their
// "dev" marker, releases get a "v" prefix (goreleaser injects a bare "1.2.3").
func versionLabel(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
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
	return applyAccent(text, spans, theme.S().Bar, accent)
}

func hintNav(downKey, upKey, desc string, accent lipgloss.Style) string {
	text, spans := navLayout(downKey, upKey, desc)
	if text == "" {
		return ""
	}
	return applyAccent(text, spans, theme.S().Bar, accent)
}

// hintLiteral renders a non-key keyword (e.g. the picker's "type" filter hint)
// as an accented indicator followed by the plain description.
func hintLiteral(keyword, desc string, accent lipgloss.Style) string {
	return applyAccent(keyword+" "+desc, []span{{0, utf8.RuneCountInString(keyword)}}, theme.S().Bar, accent)
}

func joinHints(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, theme.S().Bar.Render(" · "))
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

// accentStyle returns the key-accent style for the current vim mode: the accent
// in NORMAL, the insert accent in INSERT, both over the bar background.
func (sb *StatusBar) accentStyle() lipgloss.Style {
	fg := theme.T().AccentStatusBar
	if sb.mode == keys.ModeInsert {
		fg = theme.T().AccentInsert
	}
	return theme.NewStyle().Background(theme.T().SurfaceStatusBar).Foreground(fg)
}
