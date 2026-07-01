package components_test

import (
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
	c := components.NewComposer(40)
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
