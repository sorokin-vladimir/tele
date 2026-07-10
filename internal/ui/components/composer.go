package components

import (
	"fmt"
	"image/color"
	"strings"
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
