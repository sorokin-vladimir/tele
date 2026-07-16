package components_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sorokin-vladimir/tele/internal/store"
	"github.com/sorokin-vladimir/tele/internal/ui/components"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposerAttachmentChip(t *testing.T) {
	// Wide enough that the filename and the "Send as" toggle both fit without
	// truncation (narrow-width truncation is covered separately, #162).
	c := components.NewComposer(60)
	base := c.VisualHeight()

	c.SetAttachment("pic.jpg", 2100000, store.MediaPhoto, store.MediaPhoto, true)
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
	c.SetAttachment("clip.mov", 5_000_000, store.MediaVideo, store.MediaVideo, true)
	v := c.View()
	if !strings.Contains(v, "[Video]") {
		t.Fatalf("video chip must label+bracket the Video option:\n%s", v)
	}
	if strings.Contains(v, "Photo") {
		t.Fatalf("video chip must not mention Photo:\n%s", v)
	}

	// Toggled to file: the alternative must still read "Video", File bracketed.
	c.SetAttachment("clip.mov", 5_000_000, store.MediaVideo, store.MediaFile, true)
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
	c.SetAttachment(longName, 2_100_000, store.MediaPhoto, store.MediaPhoto, true)
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
	c.SetAttachment("pic.jpg", 2_100_000, store.MediaPhoto, store.MediaPhoto, true)
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
	c.SetAttachment("photo.png", 2_100_000, store.MediaPhoto, store.MediaPhoto, true)
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
	c.ApplyMention(store.ChatMember{UserID: 5, Username: "alice", DisplayName: "Alice A"})
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
	c.ApplyMention(store.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
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
	c.ApplyMention(store.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
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
	c.ApplyMention(store.ChatMember{UserID: 7, AccessHash: 88, DisplayName: "Ivan P"})
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
	c.ApplyMention(store.ChatMember{UserID: 20, AccessHash: 2, DisplayName: "Ivan Petrov"})
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
	c.ApplyMention(store.ChatMember{UserID: 1, AccessHash: 1, DisplayName: "Anna"})
	c.SetValue(c.Value() + "@An")
	c.ApplyMention(store.ChatMember{UserID: 2, AccessHash: 2, DisplayName: "Ann"})
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
