package components

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

	"github.com/sorokin-vladimir/tele/internal/store"
)

const (
	maxComposerLines = 5
	counterShowAt    = 200 // show remaining-char counter when remaining <= this
	counterWarnAt    = 20  // counter turns amber when remaining <= this
	sendGlyph        = "➤"
)

type Composer struct {
	ta                textarea.Model
	width             int
	replyPreview      string
	focused           bool
	hasDarkBackground bool
	attachName        string
	attachSize        int64
	attachKind        store.MediaKind // native kind (Photo/Video); labels the non-file toggle option
	attachAs          store.MediaKind // current "send as" selection
	attachOn          bool
	attachToggle      bool
	pending           []pendingMention
}

// pendingMention records a mention inserted via the autocomplete popup so its
// entity can be resolved at send time by scanning the final text.
type pendingMention struct {
	display string           // exact text inserted into the value ("@alice" or "@Ivan P")
	member  store.ChatMember // stable identity (UserID/AccessHash) for the entity
	named   bool             // true when it must emit a mention_name entity (no username)
}

func NewComposer(width int) *Composer {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = " " // one-space inset; replaces the legacy "> " prompt
	ta.MaxHeight = maxComposerLines
	ta.DynamicHeight = true
	// Modifier+Enter combos (shift+enter, alt+enter) require a terminal that supports an extended
	// key protocol (Kitty keyboard protocol, or XTerm's modifyOtherKeys). Legacy terminals such as
	// macOS Terminal.app and MinTTY (Git for Windows) silently drop these keys, so neither binding
	// fires there. Both alternatives are registered so that whichever the terminal forwards is caught.
	// Lazygit has the same limitation and handles it identically — document the requirement and list
	// multiple fallbacks. Recommended terminals: Ghostty / iTerm2 (macOS), Windows Terminal (Windows),
	// kitty, WezTerm, Alacritty. tmux users need: set -g extended-keys on
	// See: https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Custom_Keybindings.md#terminal-compatibility
	// Issue: https://github.com/sorokin-vladimir/tele/issues/9#issuecomment-4600787928
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
	ta.KeyMap.Paste = key.NewBinding() // handled at root level via readClipboardCmd → tea.PasteMsg
	ta.CharLimit = 4096
	ta.SetWidth(width - 2)
	return &Composer{ta: ta, width: width}
}

func (c *Composer) SetWidth(w int) {
	c.width = w
	c.ta.SetWidth(w - 2)
}

// Focus activates the composer cursor. Returns a blink Cmd that must be
// returned from the parent Update.
func (c *Composer) Focus() tea.Cmd {
	c.focused = true
	return c.ta.Focus()
}

func (c *Composer) Blur() {
	c.focused = false
	c.ta.Blur()
}

func (c *Composer) SetDarkBackground(isDark bool) { c.hasDarkBackground = isDark }

func (c *Composer) Value() string { return c.ta.Value() }

func (c *Composer) SetValue(v string) {
	c.ta.SetValue(v)
}

// SetPlaceholder sets the dim placeholder text shown while the composer is empty.
// The caller (chat screen) owns the wording; the composer only renders it.
func (c *Composer) SetPlaceholder(s string) { c.ta.Placeholder = s }

// Placeholder returns the current placeholder text (test accessor).
func (c *Composer) Placeholder() string { return c.ta.Placeholder }

func (c *Composer) Reset() {
	c.ta.Reset()
	c.replyPreview = ""
	c.pending = nil
}

// currentRowBeforeCursor returns the runes of the current row up to the cursor.
func (c *Composer) currentRowBeforeCursor() []rune {
	lines := strings.Split(c.ta.Value(), "\n")
	row := c.ta.Line()
	if row < 0 || row >= len(lines) {
		return nil
	}
	rs := []rune(lines[row])
	col := c.ta.Column()
	if col > len(rs) {
		col = len(rs)
	}
	return rs[:col]
}

// mentionAtStart returns the rune index of the '@' that begins the active
// mention token immediately left of the cursor, or -1 if there is none. The '@'
// must sit at the row start or right after whitespace, with no whitespace
// between it and the cursor.
func mentionAtStart(before []rune) int {
	i := len(before) - 1
	for i >= 0 {
		r := before[i]
		if r == '@' {
			if i == 0 || before[i-1] == ' ' || before[i-1] == '\t' {
				return i
			}
			return -1
		}
		if r == ' ' || r == '\t' {
			return -1
		}
		i--
	}
	return -1
}

// MentionQuery returns the active @-token text immediately left of the cursor
// on the current row, and whether such a token is present.
func (c *Composer) MentionQuery() (string, bool) {
	before := c.currentRowBeforeCursor()
	at := mentionAtStart(before)
	if at < 0 {
		return "", false
	}
	return string(before[at+1:]), true
}

// ApplyMention replaces the active @-query with the member's mention text plus a
// trailing space and records it for entity resolution. The inserted text is
// "@username" when the member has a public username, otherwise "@"+display name.
// The text is always correct; the cursor is left at the end of the value.
func (c *Composer) ApplyMention(m store.ChatMember) {
	before := c.currentRowBeforeCursor()
	at := mentionAtStart(before)
	if at < 0 {
		return
	}
	named := m.Username == ""
	display := "@" + m.Username
	if named {
		display = "@" + m.DisplayName
	}

	lines := strings.Split(c.ta.Value(), "\n")
	row := c.ta.Line()
	rs := []rune(lines[row])
	col := c.ta.Column()
	if col > len(rs) {
		col = len(rs)
	}
	newRow := string(rs[:at]) + display + " " + string(rs[col:])
	lines[row] = newRow
	c.ta.SetValue(strings.Join(lines, "\n")) // cursor lands at end (accepted MVP)

	c.pending = append(c.pending, pendingMention{display: display, member: m, named: named})
}

// utf16Len returns the number of UTF-16 code units in s (Telegram entity unit).
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// indexRunes returns the rune index of sub within runes at/after from, or -1.
func indexRunes(runes, sub []rune, from int) int {
	if len(sub) == 0 {
		return -1
	}
	for i := from; i+len(sub) <= len(runes); i++ {
		match := true
		for j := range sub {
			if runes[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// isMentionWordRune reports whether r continues a mention token. Used for
// boundary detection so a name that is a prefix of another (e.g. "@Ivan P" vs
// "@Ivan Petrov", "@Ann" vs "@Anna") is not matched inside the longer one.
func isMentionWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// findMention returns the rune index of a whole-token occurrence of sub in runes
// at/after from — one bounded by a non-word rune (or string edge) on both sides —
// or -1 when none exists.
func findMention(runes, sub []rune, from int) int {
	for {
		idx := indexRunes(runes, sub, from)
		if idx < 0 {
			return -1
		}
		leftOK := idx == 0 || !isMentionWordRune(runes[idx-1])
		right := idx + len(sub)
		rightOK := right >= len(runes) || !isMentionWordRune(runes[right])
		if leftOK && rightOK {
			return idx
		}
		from = idx + 1
	}
}

// ResolveEntities trims the draft and produces mention_name entities for each
// pending name-mention still present in the text, matched in insertion order so
// duplicate display names map to successive occurrences. Username mentions and
// edited-away mentions produce no entity.
func (c *Composer) ResolveEntities() (string, []store.MessageEntity) {
	text := strings.TrimSpace(c.ta.Value())
	if len(c.pending) == 0 {
		return text, nil
	}
	runes := []rune(text)
	var entities []store.MessageEntity
	searchFrom := 0
	for _, p := range c.pending {
		if !p.named {
			continue // username mentions are resolved server-side
		}
		sub := []rune(p.display)
		idx := findMention(runes, sub, searchFrom)
		if idx < 0 {
			continue // mention was edited/removed
		}
		entities = append(entities, store.MessageEntity{
			Type:       "mention_name",
			Offset:     utf16Len(string(runes[:idx])),
			Length:     utf16Len(p.display),
			UserID:     p.member.UserID,
			AccessHash: p.member.AccessHash,
		})
		searchFrom = idx + len(sub)
	}
	return text, entities
}

func (c *Composer) SetReplyPreview(preview string) { c.replyPreview = preview }
func (c *Composer) ClearReplyPreview()             { c.replyPreview = "" }

// SetAttachment stages a file as a chip above the textarea. nativeKind is the
// file's detected media kind (Photo/Video), used to label the non-file toggle
// option; sendAs is the current "send as" selection. toggleable controls whether
// the "Send as: Photo|Video / File" affordance is shown (image/video only).
func (c *Composer) SetAttachment(name string, size int64, nativeKind, sendAs store.MediaKind, toggleable bool) {
	c.attachName = name
	c.attachSize = size
	c.attachKind = nativeKind
	c.attachAs = sendAs
	c.attachToggle = toggleable
	c.attachOn = true
}

func (c *Composer) ClearAttachment() {
	c.attachOn = false
	c.attachName = ""
	c.attachToggle = false
}

func (c *Composer) HasAttachment() bool { return c.attachOn }

// attachmentLine renders the chip shown above the textarea, or "" if none.
// The line is clamped to the composer's inner width so it never overflows the
// box border (RenderBox pads short lines but does not truncate long ones): the
// filename is ellipsized first to keep the "Send as" toggle readable, and the
// whole line is truncated only as a last resort on very narrow widths (#162).
func (c *Composer) attachmentLine() string {
	if !c.attachOn {
		return ""
	}
	suffix := ""
	if c.attachToggle {
		kindLabel := "Photo"
		if c.attachKind == store.MediaVideo {
			kindLabel = "Video"
		}
		file := "File"
		if c.attachAs == store.MediaFile {
			file = "[File]"
		} else {
			kindLabel = "[" + kindLabel + "]"
		}
		suffix = fmt.Sprintf("   Send as: %s %s", kindLabel, file)
	}

	const prefix = "📎 "
	sizePart := "  " + humanSize(c.attachSize)
	name := c.attachName

	inner := c.width - 2
	// Width left for the filename once the fixed parts (icon, size, toggle) are placed.
	nameBudget := inner - runewidth.StringWidth(prefix) - runewidth.StringWidth(sizePart) - runewidth.StringWidth(suffix)
	if nameBudget < runewidth.StringWidth(name) {
		if nameBudget < 1 {
			// Even an empty filename overflows (extremely narrow pane or a very
			// long toggle): truncate the assembled line as a whole.
			return runewidth.Truncate(prefix+name+sizePart+suffix, max(inner, 0), "…")
		}
		name = runewidth.Truncate(name, nameBudget, "…")
	}
	return prefix + name + sizePart + suffix
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// buildContent assembles the composer's inner content: optional attachment chip,
// optional reply/edit preview (plus a blank spacer line), then the textarea.
func (c *Composer) buildContent() string {
	var parts []string
	if line := c.attachmentLine(); line != "" {
		parts = append(parts, line)
	}
	if c.replyPreview != "" {
		parts = append(parts, c.replyPreview, "")
	}
	parts = append(parts, c.ta.View())
	return strings.Join(parts, "\n")
}

// VisualHeight returns the total number of terminal rows that View() occupies:
// content lines + 2 border rows.
func (c *Composer) VisualHeight() int {
	return strings.Count(c.buildContent(), "\n") + 1 + 2
}

func (c *Composer) View() string {
	content := c.buildContent()
	h := strings.Count(content, "\n") + 1 + 2

	var borderFg color.Color
	if c.focused {
		// Green = INSERT (the composer is focused iff we are in insert mode),
		// matching the status bar's insert accent.
		borderFg = lipgloss.LightDark(c.hasDarkBackground)(lipgloss.Color("28"), lipgloss.Color("40"))
	}

	return RenderBox(content, "", "", "", c.sendAffordance(), lipgloss.RoundedBorder(), borderFg, c.width, h)
}

// sendAffordance renders the bottom-border send indicator: a dim glyph when the
// composer is empty, a blue glyph (Telegram send-button association) once there
// is text, plus a remaining-character counter when near the CharLimit.
func (c *Composer) sendAffordance() string {
	remaining := c.ta.CharLimit - utf8.RuneCountInString(c.ta.Value())
	hasText := remaining < c.ta.CharLimit

	glyphColor := lipgloss.Color("240") // dim: nothing to send
	if hasText {
		glyphColor = lipgloss.LightDark(c.hasDarkBackground)(lipgloss.Color("27"), lipgloss.Color("39")) // blue: ready
	}
	glyph := lipgloss.NewStyle().Foreground(glyphColor).Render(sendGlyph)

	if remaining <= counterShowAt {
		counterColor := lipgloss.Color("240")
		if remaining <= counterWarnAt {
			counterColor = lipgloss.Color("214") // amber
		}
		counter := lipgloss.NewStyle().Foreground(counterColor).Render(fmt.Sprintf("%d", remaining))
		return counter + " " + glyph
	}
	return glyph
}

func (c *Composer) Init() tea.Cmd { return nil }

func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)
	return c, cmd
}
