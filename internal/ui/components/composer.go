package components

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	runewidth "github.com/mattn/go-runewidth"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/markup"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

const (
	maxComposerLines = 5
	counterShowAt    = 200 // show remaining-char counter when remaining <= this
	counterWarnAt    = 20  // counter turns amber when remaining <= this
	sendGlyph        = "➤"

	// Telegram measures message length in UTF-16 code units.
	maxTextUTF16    = 4096 // plain text message
	maxCaptionUTF16 = 1024 // media caption
)

type Composer struct {
	ta           textarea.Model
	width        int
	replyPreview string
	focused      bool
	attachments  []AttachmentChip
	attachToggle bool
	pending      []pendingMention
	flash        bool // border is flashing red after a limit event
	flashSerial  int  // guards against a stale flash-off tick
	warned       bool // a limit toast already fired for this over-limit episode
}

// pendingMention records a mention inserted via the autocomplete popup so its
// entity can be resolved at send time by scanning the final text.
type pendingMention struct {
	display string            // exact text inserted into the value ("@alice" or "@Ivan P")
	member  domain.ChatMember // stable identity (UserID/AccessHash) for the entity
	named   bool              // true when it must emit a mention_name entity (no username)
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
	// Telegram counts UTF-16 code units; the textarea's own guard counts display
	// width (uniseg.StringWidth), which halves the budget for wide characters —
	// CJK is width 2 but one code unit. The unit is not correctable, so the guard
	// is disabled and Update enforces the real limit (#126).
	ta.CharLimit = 0
	// MaxHeight alone caps the draft itself: the textarea's content guard falls back
	// to blocking at MaxHeight logical lines, so newlines hit a hard wall at the
	// visible cap (#159). Setting MaxContentHeight scopes MaxHeight to the viewport
	// and moves the guard onto visual lines; one row per code unit is the most the
	// budget can ever produce (a newline costs a unit), so the budget always binds
	// first and the overflow scrolls inside the capped viewport.
	ta.MaxContentHeight = maxTextUTF16 + 1
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

func (c *Composer) Value() string { return c.ta.Value() }

func (c *Composer) SetValue(v string) {
	c.ta.SetValue(v)
}

// SetSource fills the draft with an existing message rendered back into source
// form, so an edit shows the markers that produced its formatting. Name-based
// mentions are seeded into the pending list rather than rendered as markup:
// they carry an identity no marker can express, and the normal resolve path
// re-finds them by text at send time.
func (c *Composer) SetSource(text string, entities []domain.MessageEntity) {
	c.pending = nil
	runes := []rune(text)
	for _, e := range entities {
		if e.Type != "mention_name" {
			continue
		}
		start := markup.UTF16ToRuneIndex(text, e.Offset)
		end := markup.UTF16ToRuneIndex(text, e.Offset+e.Length)
		if start >= end || end > len(runes) {
			continue
		}
		c.pending = append(c.pending, pendingMention{
			display: string(runes[start:end]),
			member:  domain.ChatMember{UserID: e.UserID, AccessHash: e.AccessHash},
			named:   true,
		})
	}
	c.ta.SetValue(markup.Render(text, entities))
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

// limit returns the draft's maximum length in UTF-16 code units. With an
// attachment staged the composer is the caption field, which Telegram caps
// lower than a plain text message.
func (c *Composer) limit() int {
	if len(c.attachments) > 0 {
		return maxCaptionUTF16
	}
	return maxTextUTF16
}

// OverLimit reports whether the draft is longer than Telegram accepts. Typing
// cannot cross the limit; the state is reached by pasting past it or by
// attaching a file to a draft longer than a caption may be.
func (c *Composer) OverLimit() bool {
	return markup.UTF16Len(c.ta.Value()) > c.limit()
}

// Line and Column expose the cursor position (test accessors).
func (c *Composer) Line() int   { return c.ta.Line() }
func (c *Composer) Column() int { return c.ta.Column() }

// SetCursorForTest places the cursor at a logical row and column (test helper).
func (c *Composer) SetCursorForTest(row, col int) { c.setCursor(row, col) }

// setCursor moves the cursor to a logical row and column. SetValue parks the
// cursor at the end of the draft, so the row is reached by walking up: CursorUp
// steps one visual line and so lowers the logical row by at most one per call,
// which cannot overshoot the target.
func (c *Composer) setCursor(row, col int) {
	for c.ta.Line() > row {
		c.ta.CursorUp()
	}
	c.ta.SetCursorColumn(col)
}

// restore puts back a draft that was rejected for exceeding the limit, leaving
// the cursor where the user had it.
func (c *Composer) restore(v string, row, col int) {
	c.ta.SetValue(v)
	c.setCursor(row, col)
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
func (c *Composer) ApplyMention(m domain.ChatMember) {
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

// ResolveEntities trims the draft, parses its markup into plain text plus
// entities, then resolves pending name-mentions against that stripped text.
// The order matters: stripping markers shifts every offset, so the mention scan
// must see the final text. Username mentions and edited-away mentions produce
// no entity.
func (c *Composer) ResolveEntities() (string, []domain.MessageEntity) {
	text, entities := markup.Parse(strings.TrimSpace(c.ta.Value()))
	if len(c.pending) == 0 {
		return text, entities
	}
	runes := []rune(text)
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
		entities = append(entities, domain.MessageEntity{
			Type:       "mention_name",
			Offset:     markup.UTF16Len(string(runes[:idx])),
			Length:     markup.UTF16Len(p.display),
			UserID:     p.member.UserID,
			AccessHash: p.member.AccessHash,
		})
		searchFrom = idx + len(sub)
	}
	return text, entities
}

func (c *Composer) SetReplyPreview(preview string) { c.replyPreview = preview }
func (c *Composer) ClearReplyPreview()             { c.replyPreview = "" }

// AttachmentChip is one staged file in the composer's attachment list. Kind is
// the MIME-detected kind (it labels the non-file toggle option); SendAs is the
// current "send as" selection.
type AttachmentChip struct {
	Name   string
	Size   int64
	Kind   domain.MediaKind
	SendAs domain.MediaKind
}

// attachChipMaxRows is the largest attachment list rendered one file per line.
// Beyond it the list collapses to a single summary line so the composer does not
// eat the chat pane.
const attachChipMaxRows = 3

// SetAttachments stages files as chips above the textarea. toggleable controls
// whether the album-wide "Send as: Photo|Video / File" affordance is shown; the
// caller decides that, since only a set where every part is a photo or a video
// has a meaningful single choice.
func (c *Composer) SetAttachments(items []AttachmentChip, toggleable bool) {
	c.attachments = items
	c.attachToggle = toggleable
}

// SetAttachment stages a single file, the one-attachment form of SetAttachments.
// nativeKind is the file's detected media kind (Photo/Video), used to label the
// non-file toggle option; sendAs is the current "send as" selection.
func (c *Composer) SetAttachment(name string, size int64, nativeKind, sendAs domain.MediaKind, toggleable bool) {
	c.SetAttachments([]AttachmentChip{
		{Name: name, Size: size, Kind: nativeKind, SendAs: sendAs},
	}, toggleable)
}

func (c *Composer) ClearAttachment() {
	c.attachments = nil
	c.attachToggle = false
}

func (c *Composer) HasAttachment() bool { return len(c.attachments) > 0 }

// attachmentLines renders the staged attachment chips shown above the textarea,
// or nil if none. One line per file up to attachChipMaxRows; beyond that a
// single summary line. Every line is clamped to the composer's inner width so it
// never overflows the box border (#162).
func (c *Composer) attachmentLines() []string {
	if len(c.attachments) == 0 {
		return nil
	}
	suffix := c.sendAsSuffix()
	if len(c.attachments) > attachChipMaxRows {
		var total int64
		for _, a := range c.attachments {
			total += a.Size
		}
		label := fmt.Sprintf("%d files", len(c.attachments))
		return []string{c.chipLine(label, "  "+humanSize(total), suffix)}
	}
	lines := make([]string, 0, len(c.attachments))
	for i, a := range c.attachments {
		name := a.Name
		if len(c.attachments) > 1 {
			name = fmt.Sprintf("%d %s", i+1, a.Name)
		}
		// The album-wide toggle belongs to the list, not to a file: show it once,
		// on the last line.
		lineSuffix := ""
		if i == len(c.attachments)-1 {
			lineSuffix = suffix
		}
		lines = append(lines, c.chipLine(name, "  "+humanSize(a.Size), lineSuffix))
	}
	return lines
}

// sendAsSuffix renders the album-wide "Send as" affordance, or "" when the set
// is not toggleable. With several parts of differing native kinds the non-file
// option is labelled generically.
func (c *Composer) sendAsSuffix() string {
	if !c.attachToggle || len(c.attachments) == 0 {
		return ""
	}
	kindLabel := "Photo"
	switch {
	case allChipsOfKind(c.attachments, domain.MediaVideo):
		kindLabel = "Video"
	case !allChipsOfKind(c.attachments, domain.MediaPhoto):
		kindLabel = "Media"
	}
	file := "File"
	if c.attachments[0].SendAs == domain.MediaFile {
		file = "[File]"
	} else {
		kindLabel = "[" + kindLabel + "]"
	}
	return fmt.Sprintf("   Send as: %s %s", kindLabel, file)
}

func allChipsOfKind(items []AttachmentChip, kind domain.MediaKind) bool {
	for _, a := range items {
		if a.Kind != kind {
			return false
		}
	}
	return true
}

// chipLine assembles one clamped chip line: the filename is ellipsized first to
// keep the "Send as" toggle readable, and the whole line is truncated only as a
// last resort on very narrow widths.
func (c *Composer) chipLine(name, sizePart, suffix string) string {
	const prefix = "📎 "
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
	parts = append(parts, c.attachmentLines()...)
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
	c.applyTheme()
	content := c.buildContent()
	h := strings.Count(content, "\n") + 1 + 2

	var borderFg color.Color
	if c.focused {
		// Green = INSERT (the composer is focused iff we are in insert mode),
		// matching the status bar's insert accent.
		borderFg = theme.T().BorderComposerFocused
	}
	if c.flash {
		// Red: input refused, or the draft is over the limit.
		borderFg = theme.T().BorderComposerFlash
	}

	return RenderBox(content, "", "", "", c.sendAffordance(), lipgloss.RoundedBorder(), borderFg, c.width, h)
}

// applyTheme refreshes the textarea styles for the active terminal scheme, then
// puts the optional canvas behind them.
//
// textarea.New starts with dark styles and does not observe tele's theme changes.
// Rebuilding its defaults also clears a canvas from an earlier custom theme.
//
// It runs per frame because the theme can change under a running session, and
// the styles are values rather than a live reference to it.
func (c *Composer) applyTheme() {
	s := textarea.DefaultStyles(theme.IsDark())
	bg := theme.T().Background
	if !theme.IsNone(bg) {
		paint := func(s textarea.StyleState) textarea.StyleState {
			s.Base = s.Base.Background(bg)
			s.Text = s.Text.Background(bg)
			s.LineNumber = s.LineNumber.Background(bg)
			s.CursorLineNumber = s.CursorLineNumber.Background(bg)
			s.CursorLine = s.CursorLine.Background(bg)
			s.EndOfBuffer = s.EndOfBuffer.Background(bg)
			s.Placeholder = s.Placeholder.Background(bg)
			s.Prompt = s.Prompt.Background(bg)
			return s
		}
		s.Focused = paint(s.Focused)
		s.Blurred = paint(s.Blurred)
	}
	c.ta.SetStyles(s)
}

// sendAffordance renders the bottom-border send indicator: a dim glyph when the
// composer is empty, a blue glyph (Telegram send-button association) once there
// is text, plus a remaining-character counter when near the limit. The counter
// measures with markup.UTF16Len — the same function Update enforces with — so it cannot
// disagree with the point at which input actually stops (#126).
func (c *Composer) sendAffordance() string {
	used := markup.UTF16Len(c.ta.Value())
	remaining := c.limit() - used

	glyphColor := theme.T().ComposerGlyphIdle // dim: nothing to send
	if used > 0 {
		glyphColor = theme.T().ComposerGlyphReady // blue: ready
	}
	glyph := theme.NewStyle().Foreground(glyphColor).Render(sendGlyph)

	if remaining <= counterShowAt {
		counterColor := theme.T().ComposerCounterDim
		switch {
		case remaining < 0:
			counterColor = theme.T().StatusError // over the limit, send is blocked
		case remaining <= counterWarnAt:
			counterColor = theme.T().StatusWarning
		}
		counter := theme.NewStyle().Foreground(counterColor).Render(fmt.Sprintf("%d", remaining))
		return counter + theme.Pad(1) + glyph
	}
	return glyph
}

func (c *Composer) Init() tea.Cmd { return nil }

func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
	if off, ok := msg.(ComposerFlashOffMsg); ok {
		if off.Serial == c.flashSerial {
			c.flash = false
		}
		return c, nil
	}

	before := markup.UTF16Len(c.ta.Value())
	prev, row, col := c.ta.Value(), c.ta.Line(), c.ta.Column()

	var cmd tea.Cmd
	c.ta, cmd = c.ta.Update(msg)

	_, isPaste := msg.(tea.PasteMsg)
	after := markup.UTF16Len(c.ta.Value())
	grewPastLimit := after > c.limit() && after > before

	switch {
	case grewPastLimit && !isPaste:
		// A refused keystroke loses nothing, so typing is rejected at the
		// boundary. Only growth is rejected: deleting must always work while over
		// the limit (#126).
		c.restore(prev, row, col)
		return c, tea.Batch(cmd, c.SignalLimit(ComposerLimitTyping))
	case grewPastLimit && isPaste:
		// A paste is content the user already has: truncating it would lose data,
		// so it is applied in full and the draft goes over the limit.
		return c, tea.Batch(cmd, c.SignalLimit(ComposerLimitPaste))
	}

	if after <= c.limit() {
		c.warned = false // back within the limit: re-arm the warning
	}
	return c, cmd
}
