package components_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposerAttachmentChip(t *testing.T) {
	// Wide enough that the filename and the "Send as" toggle both fit without
	// truncation (narrow-width truncation is covered separately, #162).
	c := components.NewComposer(60)
	base := c.VisualHeight()

	c.SetAttachment("pic.jpg", 2100000, domain.MediaPhoto, domain.MediaPhoto, true)
	if !c.HasAttachment() {
		t.Fatal("HasAttachment = false after SetAttachment")
	}
	if c.VisualHeight() <= base {
		t.Fatalf("VisualHeight did not grow with chip: %d <= %d", c.VisualHeight(), base)
	}
	v := c.View()
	if !strings.Contains(v, "pic.jpg") {
		t.Fatalf("chip missing filename:\n%s", v)
	}
	if !strings.Contains(v, "Send as") {
		t.Fatalf("toggleable chip missing 'Send as':\n%s", v)
	}

	c.ClearAttachment()
	if c.HasAttachment() {
		t.Fatal("HasAttachment = true after ClearAttachment")
	}
	if c.VisualHeight() != base {
		t.Fatalf("VisualHeight not restored: %d != %d", c.VisualHeight(), base)
	}
}

func TestComposerAttachmentChip_Video(t *testing.T) {
	c := components.NewComposer(60)

	// Native video, currently sending as video: the toggle must read "Video"
	// (not "Photo") and bracket the selected Video option.
	c.SetAttachment("clip.mov", 5_000_000, domain.MediaVideo, domain.MediaVideo, true)
	v := c.View()
	if !strings.Contains(v, "[Video]") {
		t.Fatalf("video chip must label+bracket the Video option:\n%s", v)
	}
	if strings.Contains(v, "Photo") {
		t.Fatalf("video chip must not mention Photo:\n%s", v)
	}

	// Toggled to file: the alternative must still read "Video", File bracketed.
	c.SetAttachment("clip.mov", 5_000_000, domain.MediaVideo, domain.MediaFile, true)
	v = c.View()
	if !strings.Contains(v, "Video") || !strings.Contains(v, "[File]") {
		t.Fatalf("toggled-to-file video chip want 'Video [File]':\n%s", v)
	}
	if strings.Contains(v, "[Video]") {
		t.Fatalf("File is selected, Video must not be bracketed:\n%s", v)
	}
}

func TestComposerAttachmentChip_TruncatesToWidth(t *testing.T) {
	const width = 40
	c := components.NewComposer(width)
	longName := "a_very_long_filename_that_overflows_the_box.png"
	c.SetAttachment(longName, 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
	v := c.View()

	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > width {
			t.Fatalf("line exceeds box width %d (got %d): %q", width, w, ln)
		}
	}
	// The toggle stays readable; the filename is what gets ellipsized.
	if !strings.Contains(v, "Send as") {
		t.Fatalf("toggle dropped on narrow width:\n%s", v)
	}
	if !strings.Contains(v, "…") {
		t.Fatalf("expected filename ellipsis on narrow width:\n%s", v)
	}
	if strings.Contains(v, longName) {
		t.Fatalf("full filename should have been truncated:\n%s", v)
	}
}

func TestComposerAttachmentChip_FitsWideNoTruncate(t *testing.T) {
	const width = 80
	c := components.NewComposer(width)
	c.SetAttachment("pic.jpg", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
	v := c.View()

	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > width {
			t.Fatalf("line exceeds box width %d (got %d): %q", width, w, ln)
		}
	}
	if !strings.Contains(v, "pic.jpg") {
		t.Fatalf("short filename must remain intact:\n%s", v)
	}
	if strings.Contains(v, "…") {
		t.Fatalf("no ellipsis expected when the chip fits:\n%s", v)
	}
}

func TestComposerAttachmentChip_ExtremeNarrow_NoOverflow(t *testing.T) {
	// Width so small the toggle alone overflows: the whole line is truncated as
	// a fallback and the box must still not overflow (#162).
	const width = 10
	c := components.NewComposer(width)
	c.SetAttachment("photo.png", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
	v := c.View()

	for _, ln := range strings.Split(v, "\n") {
		if w := lipgloss.Width(ln); w > width {
			t.Fatalf("line exceeds box width %d (got %d): %q", width, w, ln)
		}
	}
}

func TestComposer_Value(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("hello")
	assert.Equal(t, "hello", c.Value())
}

func TestComposer_Reset(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("text")
	c.Reset()
	assert.Empty(t, c.Value())
}

func TestComposer_View_NotEmpty(t *testing.T) {
	c := components.NewComposer(60)
	assert.NotEmpty(t, c.View())
}

func TestComposer_Update_AcceptsInput(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	newC, _ := c.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	assert.Equal(t, "a", newC.Value())
}

func TestComposer_VisualHeight_Empty(t *testing.T) {
	c := components.NewComposer(60)
	assert.Equal(t, 3, c.VisualHeight()) // 1 textarea line + 2 border
}

func TestComposer_VisualHeight_WithPreview(t *testing.T) {
	c := components.NewComposer(60)
	c.SetReplyPreview("↩ Reply: some message")
	assert.Equal(t, 5, c.VisualHeight()) // 1 textarea + 2 border + 1 preview + 1 blank sep
}

func TestComposer_ClearReplyPreview(t *testing.T) {
	c := components.NewComposer(60)
	c.SetReplyPreview("something")
	c.ClearReplyPreview()
	assert.Equal(t, 3, c.VisualHeight())
}

func TestComposer_Reset_ClearsPreview(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("hello")
	c.SetReplyPreview("something")
	c.Reset()
	assert.Empty(t, c.Value())
	assert.Equal(t, 3, c.VisualHeight())
}

func TestComposer_VisualHeight_WithTwoLinePreview(t *testing.T) {
	c := components.NewComposer(60)
	c.SetReplyPreview("line1\nline2")
	assert.Equal(t, 6, c.VisualHeight()) // 1 textarea + 2 border + 2 preview lines + 1 blank sep
}

// boxWidth returns the widest visible row of a rendered view.
func boxWidth(view string) int {
	w := 0
	for _, line := range strings.Split(view, "\n") {
		if lw := lipgloss.Width(line); lw > w {
			w = lw
		}
	}
	return w
}

// typeRune feeds a single character into the focused composer.
func typeRune(c *components.Composer, ch rune) *components.Composer {
	nc, _ := c.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	return nc
}

// Regression for #50/#8: typing across the soft-wrap boundary must never make
// the bordered box wider than the composer width, and must not produce a
// visual second row before the textarea actually grows to two lines. Both
// symptoms came from lipgloss re-wrapping the textarea's already-laid-out
// content when an explicit .Width() was set on the border style.
func TestComposer_WrapBoundary_NoOverflowNoPhantomRow(t *testing.T) {
	const width = 60
	c := components.NewComposer(width)
	c.Focus()
	// Fill with space-separated words, then type a trailing word one rune at a
	// time straight through the wrap boundary.
	c.SetValue(strings.Repeat("aaa ", 12) + "dej")

	for _, ch := range "defghijklmno" {
		c = typeRune(c, ch)

		assert.LessOrEqualf(t, boxWidth(c.View()), width,
			"box overflowed composer width after typing %q (val=%q)", ch, c.Value())

		// Content rows inside the border must equal the composer's logical line
		// count (VisualHeight minus the two border rows). A phantom extra row
		// means lipgloss soft-wrapped the content on its own.
		rows := strings.Count(c.View(), "\n") + 1
		wantRows := c.VisualHeight()
		assert.Equalf(t, wantRows, rows,
			"unexpected row count after typing %q (val=%q)", ch, c.Value())
	}
}

func TestComposerFrameNoPromptAndBordered(t *testing.T) {
	c := components.NewComposer(30)
	v := c.View()
	if !strings.Contains(v, "╭") || !strings.Contains(v, "╰") {
		t.Fatalf("composer must render a rounded frame:\n%s", v)
	}
	if strings.Contains(v, "> ") {
		t.Fatalf("legacy '> ' prompt must be gone:\n%s", v)
	}
}

func TestComposerPlaceholder(t *testing.T) {
	c := components.NewComposer(30)
	c.SetPlaceholder("Message")
	if c.Placeholder() != "Message" {
		t.Fatalf("Placeholder() = %q, want %q", c.Placeholder(), "Message")
	}
	if v := stripANSI(c.View()); !strings.Contains(v, "Message") {
		t.Fatalf("empty composer must show placeholder text:\n%s", v)
	}
	c.SetValue("hi")
	if v := stripANSI(c.View()); strings.Contains(v, "Message") {
		t.Fatalf("placeholder must be hidden once text is entered:\n%s", v)
	}
}

func TestComposerFocusChangesBorder(t *testing.T) {
	c := components.NewComposer(30)
	blurred := c.View()
	c.Focus()
	focused := c.View()
	if blurred == focused {
		t.Fatalf("focused frame must differ from blurred (border color):\nblurred:\n%s\nfocused:\n%s", blurred, focused)
	}
}

func TestComposerSendGlyphPresent(t *testing.T) {
	c := components.NewComposer(30)
	if v := c.View(); !strings.Contains(v, "➤") {
		t.Fatalf("send glyph must be present when empty:\n%s", v)
	}
	c.SetValue("hello")
	if v := c.View(); !strings.Contains(v, "➤") {
		t.Fatalf("send glyph must be present with text:\n%s", v)
	}
}

func TestComposerSendGlyphColorReflectsReadiness(t *testing.T) {
	c := components.NewComposer(30)
	empty := c.View()
	c.SetValue("hello")
	withText := c.View()
	// Same glyph, different SGR styling (dim vs blue) => the rendered frames differ.
	if empty == withText {
		t.Fatalf("send glyph styling must change between empty and non-empty")
	}
}

func TestComposerCounterNearLimit(t *testing.T) {
	c := components.NewComposer(40)

	c.SetValue("short")
	if v := stripANSI(c.View()); strings.Contains(v, "4091") {
		t.Fatalf("counter must be hidden far from the limit:\n%s", v)
	}

	c.SetValue(strings.Repeat("a", 4000)) // remaining 96, <= 200
	if v := stripANSI(c.View()); !strings.Contains(v, "96") {
		t.Fatalf("counter must appear near the limit (remaining 96):\n%s", v)
	}

	c.SetValue(strings.Repeat("a", 4090)) // remaining 6, <= 20 (amber)
	if v := stripANSI(c.View()); !strings.Contains(v, "6") {
		t.Fatalf("counter must show remaining near zero:\n%s", v)
	}
}

func TestComposer_View_ReplyPreviewBlankSeparator(t *testing.T) {
	c := components.NewComposer(60)
	c.SetReplyPreview("↩ Reply: hello")
	view := stripANSI(c.View())
	lines := strings.Split(view, "\n")

	previewIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "↩ Reply: hello") {
			previewIdx = i
			break
		}
	}
	require.GreaterOrEqual(t, previewIdx, 0, "preview line not found")
	require.Less(t, previewIdx+1, len(lines), "no line after preview")

	// The line immediately after the preview must be blank inside the border.
	lineAfter := lines[previewIdx+1]
	trimmed := strings.TrimSpace(strings.Trim(lineAfter, "│ "))
	assert.Empty(t, trimmed, "expected blank line after reply preview in composer")
}

func TestMentionQueryDetection(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("hello @ali")
	q, active := c.MentionQuery()
	if !active || q != "ali" {
		t.Fatalf("want active 'ali', got active=%v q=%q", active, q)
	}

	c.SetValue("hello @ali bob") // whitespace after token -> not active at end
	if _, active := c.MentionQuery(); active {
		t.Fatal("query must be inactive when cursor is past a completed token")
	}

	c.SetValue("plain text")
	if _, active := c.MentionQuery(); active {
		t.Fatal("no @ -> inactive")
	}

	c.SetValue("@ali") // at start of line
	if q, active := c.MentionQuery(); !active || q != "ali" {
		t.Fatalf("start-of-line @: want 'ali', got active=%v q=%q", active, q)
	}
}

func TestApplyMentionUsername(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("hi @al")
	c.ApplyMention(domain.ChatMember{UserID: 5, Username: "alice", DisplayName: "Alice A"})
	if v := c.Value(); v != "hi @alice " {
		t.Fatalf("want 'hi @alice ', got %q", v)
	}
	text, ents := c.ResolveEntities()
	if text != "hi @alice" {
		t.Fatalf("want trimmed 'hi @alice', got %q", text)
	}
	if len(ents) != 0 {
		t.Fatalf("username mention must not emit an entity, got %d", len(ents))
	}
}

func TestApplyMentionNameAndResolve(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("hi @iv")
	c.ApplyMention(domain.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
	if v := c.Value(); v != "hi @Ivan P " {
		t.Fatalf("want 'hi @Ivan P ', got %q", v)
	}
	text, ents := c.ResolveEntities()
	if len(ents) != 1 {
		t.Fatalf("want 1 entity, got %d", len(ents))
	}
	e := ents[0]
	// "hi " = 3 UTF-16 units; "@Ivan P" length = 7.
	if e.Type != "mention_name" || e.Offset != 3 || e.Length != 7 || e.UserID != 7 || e.AccessHash != 88 {
		t.Fatalf("unexpected entity: %+v (text=%q)", e, text)
	}
}

func TestResolveDroppedMention(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("@iv")
	c.ApplyMention(domain.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
	c.SetValue("nothing here") // user erased the mention
	_, ents := c.ResolveEntities()
	if len(ents) != 0 {
		t.Fatalf("erased mention must produce no entity, got %d", len(ents))
	}
}

func TestPastedTokenNotAnEntity(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("hey @Ivan P here") // pasted, never selected from popup
	_, ents := c.ResolveEntities()
	if len(ents) != 0 {
		t.Fatalf("pasted @token must not become a name mention, got %d", len(ents))
	}
}

func TestResetClearsPending(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("@iv")
	c.ApplyMention(domain.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
	c.Reset()
	c.SetValue("Ivan P") // same text, but pending cleared
	_, ents := c.ResolveEntities()
	if len(ents) != 0 {
		t.Fatalf("Reset must clear pending, got %d", len(ents))
	}
}

func TestResolvePrefixNameNotMatchedInsideLonger(t *testing.T) {
	c := components.NewComposer(60)
	// Insert the longer name first, then type a distinct trailing word.
	c.SetValue("@Iv")
	c.ApplyMention(domain.ChatMember{UserID: 20, AccessHash: 2, DisplayName: "Ivan Petrov"})
	// Value is now "@Ivan Petrov "; a pending "@Ivan P" prefix must NOT match here.
	text, ents := c.ResolveEntities()
	if len(ents) != 1 {
		t.Fatalf("want 1 entity for the full name, got %d (%q)", len(ents), text)
	}
	if ents[0].UserID != 20 || ents[0].Length != len([]rune("@Ivan Petrov")) {
		t.Fatalf("entity should cover the full name: %+v", ents[0])
	}
}

func TestResolveTwoPrefixNamesMapToCorrectUsers(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue("@An")
	c.ApplyMention(domain.ChatMember{UserID: 1, AccessHash: 1, DisplayName: "Anna"})
	c.SetValue(c.Value() + "@An")
	c.ApplyMention(domain.ChatMember{UserID: 2, AccessHash: 2, DisplayName: "Ann"})
	// Value: "@Anna @Ann "
	text, ents := c.ResolveEntities()
	if len(ents) != 2 {
		t.Fatalf("want 2 entities, got %d (%q)", len(ents), text)
	}
	// First entity is "@Anna" (offset 0), second is "@Ann" after it.
	if ents[0].UserID != 1 || ents[0].Offset != 0 || ents[0].Length != 5 {
		t.Fatalf("first entity should be @Anna: %+v", ents[0])
	}
	if ents[1].UserID != 2 || ents[1].Length != 4 {
		t.Fatalf("second entity should be @Ann: %+v", ents[1])
	}
	// The second (@Ann) must not have matched the "@Ann" prefix inside "@Anna".
	if ents[1].Offset < ents[0].Offset+ents[0].Length {
		t.Fatalf("@Ann matched inside @Anna: %+v", ents[1])
	}
}

// pressNewline feeds the InsertNewline binding (alt+enter) into the composer.
func pressNewline(c *components.Composer) *components.Composer {
	nc, _ := c.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt})
	return nc
}

// Regression for #159: the textarea's height cap must bound only the visible
// viewport, not the draft itself. The content limit is the CharLimit, so short
// lines must keep accepting newlines well past the cap.
func TestComposer_AcceptsNewlinesPastHeightCap(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()

	const wantLines = 12 // comfortably past maxComposerLines (5)
	c = typeRune(c, 'a')
	for i := 1; i < wantLines; i++ {
		c = pressNewline(c)
		c = typeRune(c, 'a')
	}

	got := strings.Count(c.Value(), "\n") + 1
	assert.Equalf(t, wantLines, got,
		"newline insert blocked past the height cap (value=%q)", c.Value())
}

// Regression for #159: once the draft outgrows the cap the viewport must follow
// the cursor, so the row being typed stays on screen and the scrolled-off head
// leaves the view.
func TestComposer_ScrollsToCursorPastHeightCap(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()

	// Markers are zero-padded so no label is a prefix of another ("L1" would
	// match inside "L12").
	for i := 1; i <= 12; i++ {
		if i > 1 {
			c = pressNewline(c)
		}
		for _, ch := range fmt.Sprintf("L%02d", i) {
			c = typeRune(c, ch)
		}
	}

	v := c.View()
	assert.Containsf(t, v, "L12", "row under the cursor is not visible:\n%s", v)
	assert.NotContainsf(t, v, "L01", "view did not scroll past the first row:\n%s", v)
}

// Companion to #159: accepting more lines must not let the box grow past the
// cap — the overflow scrolls inside the capped viewport instead.
func TestComposer_HeightStaysCappedPastLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()

	capped := 0
	for i := range 12 {
		if i > 0 {
			c = pressNewline(c)
		}
		c = typeRune(c, 'a')
		if i == 4 { // at maxComposerLines, the box has reached its cap
			capped = c.VisualHeight()
		}
	}

	require.NotZero(t, capped, "composer never reached the height cap")
	assert.Equalf(t, capped, c.VisualHeight(),
		"composer grew past its height cap (value=%q)", c.Value())
}

// utf16Count measures a draft in Telegram's unit: UTF-16 code units.
func utf16Count(s string) int { return len(utf16.Encode([]rune(s))) }

// With an attachment staged the composer is the caption field, so the counter
// must track Telegram's 1024 caption cap rather than the 4096 message cap.
func TestComposer_CounterUsesCaptionLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachment("photo.png", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	c.SetValue(strings.Repeat("a", 900)) // remaining 124 of 1024, within counterShowAt

	assert.Contains(t, stripANSI(c.View()), "124")
}

// Attaching a file to a long draft drops the limit under the existing text.
// The counter shows a negative remaining rather than clamping at 0, so the size
// of the overshoot is visible.
func TestComposer_CounterGoesNegativeOverLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue(strings.Repeat("a", 2000))
	c.SetAttachment("photo.png", 1024, domain.MediaPhoto, domain.MediaPhoto, true)

	require.True(t, c.OverLimit())
	assert.Contains(t, stripANSI(c.View()), "-976")
}

// An emoji is a surrogate pair: two code units per rune. Counting runes would
// overstate the remaining budget by one per emoji.
func TestComposer_CounterCountsCodeUnitsNotRunes(t *testing.T) {
	c := components.NewComposer(60)
	c.SetValue(strings.Repeat("😀", 2000)) // 4000 code units, remaining 96

	assert.Contains(t, stripANSI(c.View()), "96")
}

// Regression for #126: enforcement must count UTF-16 code units like Telegram,
// not display width. A CJK rune is width 2 but a single code unit, so the old
// display-width guard stopped this draft at half the allowance.
func TestComposer_WideCharsGetFullLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("中", 4095))

	c = typeRune(c, '中')
	require.Equal(t, 4096, utf16Count(c.Value()), "the 4096th code unit must be accepted")

	c = typeRune(c, '中')
	assert.Equal(t, 4096, utf16Count(c.Value()), "the 4097th code unit must be rejected")
}

// An emoji costs two code units, so one must not squeeze into a one-unit gap.
func TestComposer_EmojiCountsAsTwoCodeUnits(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4095))

	c = typeRune(c, '😀') // would land at 4097 code units
	assert.Equal(t, 4095, utf16Count(c.Value()), "an emoji that does not fit must be rejected whole")
}

// The counter exists to explain the stop, so it must read 0 exactly when input
// stops. Measured in runes it read "2048 remaining" on CJK that was already
// blocked — the counter lying about the very stop it explains.
func TestComposer_CounterReachesZeroExactlyWhenInputStops(t *testing.T) {
	for _, tc := range []struct {
		name string
		fill string
		n    int
	}{
		{"ascii", "a", 4096},
		{"cjk", "中", 4096},
		{"emoji", "😀", 2048},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := components.NewComposer(60)
			c.Focus()
			c.SetValue(strings.Repeat(tc.fill, tc.n))
			require.Contains(t, stripANSI(c.View()), "0", "counter must read 0 at the limit")

			before := c.Value()
			c = typeRune(c, 'x')
			assert.Equal(t, before, c.Value(), "input must be blocked when the counter reads 0")
		})
	}
}

// With an attachment staged the composer is the caption field: 1024, not 4096.
func TestComposer_CaptionLimitIsEnforcedAt1024(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetAttachment("photo.png", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	c.SetValue(strings.Repeat("a", 1023))

	c = typeRune(c, 'a')
	require.Equal(t, 1024, utf16Count(c.Value()), "the 1024th caption character must be accepted")

	c = typeRune(c, 'a')
	assert.Equal(t, 1024, utf16Count(c.Value()), "the 1025th caption character must be rejected")
}

// Only growth is rejected. Attaching a file to a long draft drops the limit
// under the existing text; deleting back down must always work, or the
// over-limit state would be inescapable.
func TestComposer_DeletionWorksWhileOverLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 2000))
	c.SetAttachment("photo.png", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	require.True(t, c.OverLimit(), "2000 chars with an attachment is over the 1024 caption limit")

	c, _ = c.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	assert.Equal(t, 1999, utf16Count(c.Value()), "backspace must work while over the limit")
}

// A paste is content the user already has: truncating it would lose data, so it
// is applied in full and the draft goes over the limit.
func TestComposer_PasteAppliedInFullPastLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()

	c, _ = c.Update(tea.PasteMsg{Content: strings.Repeat("a", 5000)})
	assert.Equal(t, 5000, utf16Count(c.Value()), "paste must not be truncated")
	assert.True(t, c.OverLimit())
}

// A rejected keystroke must not disturb the draft or the cursor.
func TestComposer_RejectedKeystrokeKeepsCursorPosition(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4090) + "\n" + "bbbbb") // 4096 code units
	c.SetCursorForTest(1, 2)                               // mid-way through the last row

	before := c.Value()
	c = typeRune(c, 'z')
	require.Equal(t, before, c.Value(), "draft must be unchanged")
	assert.Equal(t, 1, c.Line(), "cursor row must be restored")
	assert.Equal(t, 2, c.Column(), "cursor column must be restored")
}

// collectMsgs runs a Cmd, unwrapping tea.Batch, and returns the msgs produced.
// SignalLimit batches a tea.Tick, so each call sleeps for flashDuration before
// yielding its ComposerFlashOffMsg. That cost is accepted here: it keeps the
// helper honest about what the Cmd actually emits.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var out []tea.Msg
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, sub := range m {
			out = append(out, collectMsgs(sub)...)
		}
	case nil:
	default:
		out = append(out, m)
	}
	return out
}

// limitMsgs filters for ComposerLimitMsg.
func limitMsgs(msgs []tea.Msg) []components.ComposerLimitMsg {
	var out []components.ComposerLimitMsg
	for _, m := range msgs {
		if lm, ok := m.(components.ComposerLimitMsg); ok {
			out = append(out, lm)
		}
	}
	return out
}

// A rejected keystroke must flash the border and warn.
func TestComposer_RejectedKeystrokeSignalsLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4096))

	c, cmd := c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.NotNil(t, cmd, "a rejected keystroke must signal")
	assert.True(t, c.FlashActive(), "border must flash on rejection")
	assert.Contains(t, collectMsgs(cmd), components.ComposerLimitMsg{
		Kind: components.ComposerLimitTyping, Limit: 4096, Caption: false,
	})
}

// The toast fires once per episode: a held key must not queue one per keystroke.
func TestComposer_WarnsOncePerEpisode(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4096))

	c, cmd1 := c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.Len(t, limitMsgs(collectMsgs(cmd1)), 1, "first rejection warns")

	c, cmd2 := c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Empty(t, limitMsgs(collectMsgs(cmd2)), "second rejection must not warn again")
	assert.True(t, c.FlashActive(), "but it must still flash")
}

// Returning under the limit re-arms the warning.
func TestComposer_WarningReArmsAfterGoingUnderLimit(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4096))
	c, _ = c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) // warns, warned = true

	c, _ = c.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})  // 4095: back under, re-arms
	c, _ = c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})    // 4096: accepted, at the limit
	_, cmd := c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"}) // rejected

	assert.Len(t, limitMsgs(collectMsgs(cmd)), 1, "a new episode must warn again")
}

// The flash decays back, and a stale tick must not clear a newer flash.
func TestComposer_FlashClearedBySerialOnly(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	c.SetValue(strings.Repeat("a", 4096))
	c, _ = c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.True(t, c.FlashActive())

	c, _ = c.Update(components.ComposerFlashOffMsg{Serial: -1})
	assert.True(t, c.FlashActive(), "a stale serial must not clear the flash")

	c, _ = c.Update(components.ComposerFlashOffMsg{Serial: c.FlashSerialForTest()})
	assert.False(t, c.FlashActive(), "the current serial must clear the flash")
}

// Paste past the limit is applied in full, and warns with its own kind.
func TestComposer_PastePastLimitWarns(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()

	c, cmd := c.Update(tea.PasteMsg{Content: strings.Repeat("a", 5000)})
	require.Equal(t, 5000, utf16Count(c.Value()), "paste must still not be truncated")
	assert.Contains(t, collectMsgs(cmd), components.ComposerLimitMsg{
		Kind: components.ComposerLimitPaste, Limit: 4096, Caption: false,
	})
}

// The flashing border must be visibly distinct from the normal focused border.
func TestComposer_FlashChangesBorderColour(t *testing.T) {
	c := components.NewComposer(60)
	c.Focus()
	normal := c.View()

	c.SetValue(strings.Repeat("a", 4096))
	c, _ = c.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

	assert.NotEqual(t, normal, c.View(), "the flashing border must differ from the focused border")
}

// The counter must adapt to the theme: the bright amber/red read well on a dark
// background but wash out on a light one.
func TestComposer_CounterColourIsThemeAware(t *testing.T) {
	render := func(dark bool) string {
		theme.Apply(dark)
		c := components.NewComposer(60)
		c.SetValue(strings.Repeat("a", 4090)) // remaining 6: amber
		return c.View()
	}
	assert.NotEqual(t, render(true), render(false),
		"the counter must not use the same colour on both themes")
}

// The textarea keeps a palette of its own and does not watch tele's theme, so a
// slot change can leave it rendering the previous slot's colours (#242).
//
// Both directions are asserted through the same property: a composer that lived
// through the change must look exactly like one built after it. What that
// catches is a style held over, which is the defect; what it avoids is naming
// the vendored component's colours here, which are not tele's to pin.
func TestComposer_TextareaStylesDoNotOutliveASlotChange(t *testing.T) {
	t.Cleanup(func() {
		theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})
		theme.Apply(true)
	})

	draft := func() *components.Composer {
		c := components.NewComposer(60)
		c.Focus()
		c.SetValue("draft")
		return c
	}

	// The built-ins, dark to light: the direction that shipped, where the
	// textarea kept a dark cursor line under a terminal gone light.
	theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})
	theme.Apply(true)
	switched := draft()
	switched.View()
	theme.Apply(false)
	assert.Equal(t, draft().View(), switched.View(), "the dark slot's textarea styles outlived the switch to light")

	// A light theme that paints a canvas, back to a dark one that does not: the
	// inverse, where the painted background had nothing to clear it.
	bg, err := theme.ParseColor("#dadada")
	require.NoError(t, err)
	fg, err := theme.ParseColor("#4e4e4e")
	require.NoError(t, err)
	painted := theme.TeleLight
	painted.Name = "canvas-light-test"
	painted.Background, painted.Text = bg, fg

	theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: painted})
	theme.Apply(false)
	switched = draft()
	switched.View()
	theme.Apply(true)
	assert.Equal(t, draft().View(), switched.View(), "the painted canvas outlived the switch to dark")
}

// A theme that paints a canvas covers the terminal background over, so the
// textarea's variant has to follow the canvas rather than the slot. Otherwise a
// light-canvas theme in the dark slot draws dark-variant text on a light field.
func TestComposer_TextareaVariantFollowsTheCanvasNotTheSlot(t *testing.T) {
	t.Cleanup(func() {
		theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})
		theme.Apply(true)
	})

	bg, err := theme.ParseColor("#dadada")
	require.NoError(t, err)
	fg, err := theme.ParseColor("#4e4e4e")
	require.NoError(t, err)
	painted := theme.TeleLight
	painted.Name = "canvas-light-test"
	painted.Background, painted.Text = bg, fg

	// The same painted theme in both slots, which is what `ui.theme: <name>`
	// produces. Only the terminal background differs between the two renders,
	// and it is the one thing the canvas makes irrelevant.
	theme.SetSlots(theme.Slots{Dark: painted, Light: painted})

	render := func(dark bool) string {
		theme.Apply(dark)
		c := components.NewComposer(60)
		c.SetValue("draft")
		return c.View()
	}
	assert.Equal(t, render(false), render(true), "the terminal background changed a composer the canvas fully covers")
}

func TestResolveEntitiesParsesMarkup(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("привет **важно**")
	text, ents := c.ResolveEntities()
	assert.Equal(t, "привет важно", text)
	require.Len(t, ents, 1)
	assert.Equal(t, "bold", ents[0].Type)
	assert.Equal(t, 7, ents[0].Offset)
	assert.Equal(t, 5, ents[0].Length)
}

func TestResolveEntitiesPlainTextStaysEntityFree(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("обычный текст")
	text, ents := c.ResolveEntities()
	assert.Equal(t, "обычный текст", text)
	assert.Empty(t, ents)
}

func TestResolveEntitiesMentionOffsetsSurviveMarkerStripping(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("@")
	c.ApplyMention(domain.ChatMember{UserID: 7, AccessHash: 8, DisplayName: "Ivan P"})
	c.SetValue("**жирно** " + c.Value())
	text, ents := c.ResolveEntities()
	assert.Equal(t, "жирно @Ivan P", text)

	var mention domain.MessageEntity
	for _, e := range ents {
		if e.Type == "mention_name" {
			mention = e
		}
	}
	// Offset must index the stripped text ("жирно @Ivan P"), not the source.
	assert.Equal(t, 6, mention.Offset)
	assert.Equal(t, 7, mention.Length)
	assert.Equal(t, int64(7), mention.UserID)
}

func TestResolveEntitiesMentionInsideBold(t *testing.T) {
	c := components.NewComposer(40)
	c.SetValue("@")
	c.ApplyMention(domain.ChatMember{UserID: 7, AccessHash: 8, DisplayName: "Ivan P"})
	c.SetValue("**" + strings.TrimSpace(c.Value()) + "**")
	_, ents := c.ResolveEntities()
	require.Len(t, ents, 2)
	assert.Equal(t, "bold", ents[0].Type)
	assert.Equal(t, "mention_name", ents[1].Type)
	assert.Equal(t, 0, ents[1].Offset)
}

func TestSetSourceShowsMarkers(t *testing.T) {
	c := components.NewComposer(40)
	c.SetSource("привет важно", []domain.MessageEntity{{Type: "bold", Offset: 7, Length: 5}})
	assert.Equal(t, "привет **важно**", c.Value())
}

func TestSetSourceRoundTripsThroughResolve(t *testing.T) {
	c := components.NewComposer(40)
	ents := []domain.MessageEntity{{Type: "bold", Offset: 7, Length: 5}}
	c.SetSource("привет важно", ents)
	text, got := c.ResolveEntities()
	assert.Equal(t, "привет важно", text)
	require.Len(t, got, 1)
	assert.Equal(t, ents[0], got[0])
}

func TestSetSourceSeedsMentionsSoTheySurviveEditing(t *testing.T) {
	c := components.NewComposer(40)
	c.SetSource("@Ivan P привет", []domain.MessageEntity{
		{Type: "mention_name", Offset: 0, Length: 7, UserID: 7, AccessHash: 8},
	})
	assert.Equal(t, "@Ivan P привет", c.Value())
	_, ents := c.ResolveEntities()
	require.Len(t, ents, 1)
	assert.Equal(t, "mention_name", ents[0].Type)
	assert.Equal(t, int64(7), ents[0].UserID)
	assert.Equal(t, int64(8), ents[0].AccessHash)
}

func TestAttachments_SingleFileKeepsOneLine(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachments([]components.AttachmentChip{
		{Name: "photo.jpg", Size: 2100000, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
	}, true)

	view := c.View()
	assert.Contains(t, view, "photo.jpg")
	assert.Contains(t, view, "Send as:")
	assert.NotContains(t, view, "1 photo.jpg", "a single attachment is not numbered")
}

func TestAttachments_ThreeFilesAreListedAndNumbered(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachments([]components.AttachmentChip{
		{Name: "a.jpg", Size: 10, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
		{Name: "b.jpg", Size: 20, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
		{Name: "c.jpg", Size: 30, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
	}, true)

	view := c.View()
	for _, want := range []string{"1 a.jpg", "2 b.jpg", "3 c.jpg"} {
		assert.Contains(t, view, want)
	}
	assert.Equal(t, 1, strings.Count(view, "Send as:"), "the toggle is album-wide, shown once")
}

func TestAttachments_FourFilesCollapseToSummary(t *testing.T) {
	c := components.NewComposer(60)
	items := []components.AttachmentChip{}
	for _, n := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
		items = append(items, components.AttachmentChip{Name: n, Size: 1024, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto})
	}
	c.SetAttachments(items, true)

	view := c.View()
	assert.Contains(t, view, "4 files")
	assert.NotContains(t, view, "a.jpg", "the collapsed summary lists no names")
}

func TestAttachments_MixedKindsHideToggle(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachments([]components.AttachmentChip{
		{Name: "a.jpg", Size: 10, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
		{Name: "b.pdf", Size: 20, Kind: domain.MediaFile, SendAs: domain.MediaFile},
	}, false)

	assert.NotContains(t, c.View(), "Send as:")
}

func TestAttachments_NarrowWidthDoesNotTearBorder(t *testing.T) {
	c := components.NewComposer(24)
	c.SetAttachments([]components.AttachmentChip{
		{Name: "a-very-long-file-name-indeed.jpg", Size: 2100000, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
		{Name: "another-very-long-name.jpg", Size: 2100000, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
	}, true)

	for _, line := range strings.Split(c.View(), "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 24, "line %q overflows the composer width", line)
	}
}

func TestAttachments_VisualHeightGrowsWithList(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachments([]components.AttachmentChip{
		{Name: "a.jpg", Size: 10, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
	}, true)
	one := c.VisualHeight()

	c.SetAttachments([]components.AttachmentChip{
		{Name: "a.jpg", Size: 10, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
		{Name: "b.jpg", Size: 20, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
	}, true)

	assert.Equal(t, one+1, c.VisualHeight())
}

func TestAttachments_SeparatorBeforeReplyPreview(t *testing.T) {
	c := components.NewComposer(60)
	c.SetAttachment("pic.jpg", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	c.SetReplyPreview("▌ Reply to message")

	view := stripANSI(c.View())
	lines := strings.Split(view, "\n")

	chipIdx, previewIdx := -1, -1
	for i, l := range lines {
		if strings.Contains(l, "pic.jpg") {
			chipIdx = i
		}
		if strings.Contains(l, "▌ Reply to message") {
			previewIdx = i
		}
	}
	require.GreaterOrEqual(t, chipIdx, 0, "attachment chip not found")
	require.GreaterOrEqual(t, previewIdx, 0, "reply preview not found")
	require.Equal(t, chipIdx+2, previewIdx, "expected exactly one line between chip and preview")

	// The line between chip and preview must be blank inside the box borders.
	sep := strings.TrimSpace(strings.Trim(lines[chipIdx+1], "│ "))
	assert.Empty(t, sep, "line between attachment chip and preview must be blank")

	// Without preview, chip is immediately followed by textarea (no extra separator).
	c.ClearReplyPreview()
	assert.Equal(t, 4, c.VisualHeight(), "chip without preview VisualHeight should be 4 (1 chip + 1 textarea + 2 borders)")
}

func TestAttachments_VisualHeightWithAndWithoutSeparator(t *testing.T) {
	c := components.NewComposer(60)
	base := c.VisualHeight() // 3 (1 textarea line + 2 border rows)

	// Attachment only: adds 1 row.
	c.SetAttachment("pic.jpg", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	assert.Equal(t, base+1, c.VisualHeight(), "one attachment chip adds 1 row")

	// With reply preview added: adds chip (1) + separator (1) + preview (1) + preview spacer (1) = +4.
	c.SetReplyPreview("▌ Reply")
	assert.Equal(t, base+4, c.VisualHeight(), "chip and single-line preview have separator between them")

	// With two-line reply preview: chip (1) + separator (1) + preview (2) + preview spacer (1) = +5.
	c.SetReplyPreview("line1\nline2")
	assert.Equal(t, base+5, c.VisualHeight(), "chip and two-line preview have separator between them")

	// Clear attachment: preview (2) + preview spacer (1) = +3.
	c.ClearAttachment()
	assert.Equal(t, base+3, c.VisualHeight(), "preview only without attachment has no leading separator")

	// Restore attachment and clear preview: chip (1) = +1.
	c.SetAttachment("pic.jpg", 1024, domain.MediaPhoto, domain.MediaPhoto, true)
	c.ClearReplyPreview()
	assert.Equal(t, base+1, c.VisualHeight(), "attachment only restored without preview")
}

func TestAttachments_ChipLinePaintsCanvas(t *testing.T) {
	t.Cleanup(func() {
		theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})
		theme.Apply(true)
	})

	bg, err := theme.ParseColor("#282c34")
	require.NoError(t, err)
	fg, err := theme.ParseColor("#abb2bf")
	require.NoError(t, err)

	painted := theme.TeleDark
	painted.Name = "canvas-test"
	painted.Background, painted.Text = bg, fg
	theme.SetSlots(theme.Slots{Dark: painted, Light: painted})
	theme.Apply(true)

	cases := []struct {
		name  string
		width int
		setup func(*components.Composer)
	}{
		{
			name:  "single-file",
			width: 60,
			setup: func(c *components.Composer) {
				c.SetAttachment("pic.jpg", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, false)
			},
		},
		{
			name:  "single-file-with-send-as",
			width: 60,
			setup: func(c *components.Composer) {
				c.SetAttachment("pic.jpg", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
			},
		},
		{
			name:  "numbered-album",
			width: 60,
			setup: func(c *components.Composer) {
				c.SetAttachments([]components.AttachmentChip{
					{Name: "a.jpg", Size: 10, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
					{Name: "b.jpg", Size: 20, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto},
				}, true)
			},
		},
		{
			name:  "files-summary",
			width: 60,
			setup: func(c *components.Composer) {
				items := []components.AttachmentChip{}
				for _, n := range []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"} {
					items = append(items, components.AttachmentChip{Name: n, Size: 1024, Kind: domain.MediaPhoto, SendAs: domain.MediaPhoto})
				}
				c.SetAttachments(items, true)
			},
		},
		{
			name:  "truncated-name",
			width: 40,
			setup: func(c *components.Composer) {
				c.SetAttachment("a_very_long_file_name_that_overflows_the_box.png", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
			},
		},
		{
			name:  "extreme-narrow",
			width: 10,
			setup: func(c *components.Composer) {
				c.SetAttachment("photo.png", 2_100_000, domain.MediaPhoto, domain.MediaPhoto, true)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := components.NewComposer(tc.width)
			tc.setup(c)
			view := c.View()
			h := c.VisualHeight()

			buf := uv.NewScreenBuffer(tc.width, h)
			buf.Method = ansi.GraphemeWidth
			uv.NewStyledString(view).Draw(buf, uv.Rect(0, 0, tc.width, h))

			for row := 0; row < h; row++ {
				span := 0
				for col := 0; col < tc.width; col++ {
					cell := buf.CellAt(col, row)
					if span > 0 {
						span--
						continue
					}
					if cell != nil && cell.Width > 1 {
						span = cell.Width - 1
					}
					require.NotNil(t, cell, "row %d col %d: cell is nil", row, col)
					require.NotNil(t, cell.Style.Bg, "row %d col %d (%q): cell missing background", row, col, cell.Content)
					if strings.TrimSpace(cell.Content) != "" && !isBoxChar(cell.Content) {
						require.NotNil(t, cell.Style.Fg, "row %d col %d (%q): glyph missing foreground", row, col, cell.Content)
					}
				}
			}
		})
	}
}

func isBoxChar(s string) bool {
	for _, r := range s {
		if r >= 0x2500 && r <= 0x259F {
			return true
		}
	}
	return false
}
