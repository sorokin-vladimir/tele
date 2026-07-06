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

// Severity classifies a transient status-bar message.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
)

type StatusBar struct {
	width      int
	mode       keys.VimMode
	status     string
	verbose    bool
	lastKey    string
	activePane string
	keyMap     keys.KeyMap
	errText    string
	errSev     Severity
	errSerial  int
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

// SetError shows a transient, severity-tagged message and returns the serial
// identifying it, so a later ClearError only clears this exact message.
func (sb *StatusBar) SetError(text string, sev Severity) int {
	sb.errSerial++
	sb.errText = text
	sb.errSev = sev
	return sb.errSerial
}

// ClearError clears the error only when serial matches the current one, so a
// stale auto-clear timer cannot wipe a newer error.
func (sb *StatusBar) ClearError(serial int) {
	if serial == sb.errSerial {
		sb.errText = ""
	}
}

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

	if sb.errText != "" {
		segs = append(segs, errStyle(sb.errSev).Render(sb.errText))
	} else if sb.dlText != "" {
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

func errStyle(sev Severity) lipgloss.Style {
	base := lipgloss.NewStyle().Background(barBg).Bold(true)
	switch sev {
	case SeverityError:
		return base.Foreground(lipgloss.Color("203")) // red
	case SeverityWarning:
		return base.Foreground(lipgloss.Color("214")) // amber
	default:
		return base.Foreground(lipgloss.Color("75")) // blue/info
	}
}

func (sb *StatusBar) hints() string {
	if sb.keyMap == nil {
		return ""
	}
	a := sb.accentStyle()
	switch {
	case sb.pickerOpen:
		confirm := sb.keyMap.KeyFor(keys.ContextFilePicker, keys.ActionConfirm)
		cancel := sb.keyMap.KeyFor(keys.ContextFilePicker, keys.ActionCancel)
		return joinHints(
			hintLiteral("type", "filter", a),
			hintKey(confirm, "open/select", a),
			hintKey(cancel, "cancel", a),
		)
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert && sb.attachStaged:
		send := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionConfirm)
		toggle := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionToggleSendAs)
		normal := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionNormal)
		return joinHints(
			hintKey(send, "send", a),
			hintKey(toggle, "photo/file", a),
			hintKey(normal, "normal", a),
		)
	case sb.activePane == "chat" && sb.attachStaged:
		write := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionInsert)
		drop := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionCancelUpload)
		return joinHints(
			hintKey(write, "caption", a),
			hintKey(drop, "drop file", a),
		)
	case sb.activePane == "folders":
		down := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionUp)
		sel := sb.keyMap.KeyFor(keys.ContextFolders, keys.ActionConfirm)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "move", a),
			hintKey(sel, "select", a),
			hintKey(quit, "quit", a),
		)
	case sb.activePane == "chat" && sb.mode == keys.ModeInsert:
		send := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionConfirm)
		normal := sb.keyMap.KeyFor(keys.ContextComposer, keys.ActionNormal)
		return joinHints(hintKey(send, "send", a), hintKey(normal, "normal", a))
	case sb.activePane == "chat":
		down := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionUp)
		curDown := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionCursorDown)
		curUp := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionCursorUp)
		write := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionInsert)
		attach := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionAttach)
		open := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionOpenInViewer)
		copyKey := sb.keyMap.KeyFor(keys.ContextChat, keys.ActionCopyMessage)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "scroll", a),
			hintNav(curDown, curUp, "select", a),
			hintKey(write, "write", a),
			hintKey(attach, "upload", a),
			hintKey(open, "open", a),
			hintKey(copyKey, "copy", a),
			hintKey(quit, "quit", a),
		)
	case sb.activePane == "chatlist":
		down := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionDown)
		up := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionUp)
		open := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionConfirm)
		search := sb.keyMap.KeyFor(keys.ContextChatList, keys.ActionSearch)
		quit := sb.keyMap.KeyFor(keys.ContextGlobal, keys.ActionQuit)
		return joinHints(
			hintNav(down, up, "move", a),
			hintKey(open, "open", a),
			hintKey(search, "search", a),
			hintKey(quit, "quit", a),
		)
	}
	return ""
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
