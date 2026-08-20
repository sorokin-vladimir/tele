package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui"
	"github.com/sorokin-vladimir/tele/internal/ui/theme"
)

// A seam is the join between two painted runs, where one run's reset ends the
// colour and the next starts it again. Seams are ordinary — every coloured run
// in a line makes two — and a hole is what one left unpainted: a cell the
// canvas never reached, which falls through to the terminal. The failure mode
// of #214 is that a hole is visible only to whoever happens to be looking at
// the right screen state, so they are counted here instead of looked for.
//
// The check runs on the cell grid rather than on the string: bubbletea renders
// by parsing the view with uv.NewStyledString and diffing the cells that come
// out, so the grid this walks is the one the terminal is actually told about,
// escape for escape.
//
// The invariant is the absence of a background, not the presence of an expected
// one. A whitelist of colours would be brittle — highlight_accent is
// interpolated and produces arbitrary intermediate values — and it would be
// checking the wrong thing. A hole is a cell that carries no background and
// therefore falls through to the terminal.

// hole is one unpainted cell, located.
type hole struct {
	row, col int
	content  string
}

func (h hole) String() string {
	return fmt.Sprintf("row %d, col %d: %q", h.row, h.col, h.content)
}

// holes returns every cell of a w x h terminal that the view left without a
// background.
//
// The view is drawn into a cell buffer rather than split on newlines, which is
// what bubbletea does with it (cursed_renderer.go: NewStyledString, then Draw
// into the screen buffer over Rect(0, 0, width, height)). It matters for more
// than fidelity: a buffer covers the whole terminal, so a cell the view never
// wrote to at all is examined too. Those are the holes that a check over the
// emitted lines cannot see, and the ones a short row or a missing bottom line
// leaves behind.
func holes(content string, w, h int) []hole {
	buf := uv.NewScreenBuffer(w, h)
	buf.Method = ansi.GraphemeWidth
	uv.NewStyledString(content).Draw(buf, uv.Rect(0, 0, w, h))

	var out []hole
	for row := range h {
		// A grapheme wider than one column occupies the columns after it with
		// placeholder cells that carry no style of their own. The terminal
		// paints those from the one SGR that introduced the grapheme, so they
		// are not holes — skipping them is why an emoji in the reaction picker
		// does not read as eight of them.
		span := 0
		for col := range w {
			cell := buf.CellAt(col, row)
			if span > 0 {
				span--
				continue
			}
			if cell != nil && cell.Width > 1 {
				span = cell.Width - 1
			}
			if cell != nil && cell.Style.Bg != nil {
				continue
			}
			content := ""
			if cell != nil {
				content = cell.Content
			}
			out = append(out, hole{row: row, col: col, content: content})
		}
	}
	return out
}

// unowned returns every cell holding a visible glyph that the theme set no
// foreground for.
//
// It is the other half of the same defect. A theme that claims the canvas is
// refused unless it also sets text, because a claimed background under a
// foreground the app does not own is unreadable by accident — that is what the
// token dependency exists to prevent. A cell drawn from a bare literal escapes
// the dependency the same way it escapes the canvas: it has no colour of ours
// at all, so the terminal's own foreground lands on our background.
//
// Blank cells are not counted. A space has no glyph to read, so padding owes a
// background and nothing else.
//
// Box-drawing glyphs are not counted either, and that is a boundary rather than
// an oversight. An unfocused pane is drawn by passing a nil border colour to
// RenderBox (root_view.go: only the focused pane gets activeFg), so "inactive"
// is expressed by declining to colour the frame at all — around 300 cells in
// every frame, plain ones included, and the same before this scan existed as
// after. Whether a claimed canvas may leave its frame to the terminal is a real
// question and a separate one; counting it here would bury the forty-odd cells
// this issue is about.
func unowned(content string, w, h int) []hole {
	buf := uv.NewScreenBuffer(w, h)
	buf.Method = ansi.GraphemeWidth
	uv.NewStyledString(content).Draw(buf, uv.Rect(0, 0, w, h))

	var out []hole
	for row := range h {
		span := 0
		for col := range w {
			cell := buf.CellAt(col, row)
			if span > 0 {
				span--
				continue
			}
			if cell == nil {
				continue
			}
			if cell.Width > 1 {
				span = cell.Width - 1
			}
			if strings.TrimSpace(cell.Content) == "" || cell.Style.Fg != nil {
				continue
			}
			if isBoxDrawing(cell.Content) {
				continue
			}
			out = append(out, hole{row: row, col: col, content: cell.Content})
		}
	}
	return out
}

// isBoxDrawing reports whether a cell holds nothing but box-drawing and block
// glyphs, which is how a pane frame and a scrollbar are told from text.
func isBoxDrawing(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x2500 || r > 0x259F {
			return false
		}
	}
	return true
}

// report renders the first few holes; a broken frame produces thousands, and
// the first handful say where to look as well as all of them would. what names
// what the cells are missing, since the same shape reports both halves.
func report(found []hole, what string) string {
	const show = 12
	s := fmt.Sprintf("%d cells carry no %s\n", len(found), what)
	for i, f := range found {
		if i == show {
			s += fmt.Sprintf("  ... and %d more\n", len(found)-show)
			break
		}
		s += "  " + f.String() + "\n"
	}
	return s
}

// paintedSlots installs a theme claiming the canvas, for the duration of the
// test. Both slots hold it so the check does not depend on what the terminal
// reported.
func paintedSlots(t *testing.T) {
	t.Helper()
	bg, err := theme.ParseColor("#1e1e2e")
	require.NoError(t, err)
	fg, err := theme.ParseColor("#cdd6f4")
	require.NoError(t, err)

	painted := theme.TeleDark
	painted.Name = "canvas-test"
	painted.Background, painted.Text = bg, fg

	t.Cleanup(func() { theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight}) })
	theme.SetSlots(theme.Slots{Dark: painted, Light: painted})
	theme.Apply(true)
}

// The sizes are the ones that behave differently rather than a sweep. 80x24 is
// the floor; 41 columns is narrow enough that the panes hit their minimum
// widths, which is where the arithmetic that makes them sum has to be trusted;
// odd widths and heights catch a split that rounds a column away.
var scanSizes = []struct{ w, h int }{
	{80, 24},
	{81, 25},
	{41, 12},
	{120, 40},
	{201, 61},
}

func TestCanvas_MainScreenHasNoHoles(t *testing.T) {
	paintedSlots(t)

	for _, size := range scanSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := newPopulatedRoot(t, size.w, size.h)
			found := holes(m.View().Content, size.w, size.h)
			require.Empty(t, found, report(found, "background"))
		})
	}
}

// Overlays are where the seams are: each one is stamped into the composed screen
// by hand, and the stamping pads the base row out to meet it.
//
// The list is the one the issue audited for selection fills that stop short of
// the row edge. A fill that ends early used to blend into the terminal and was
// invisible; over a canvas it reads as a ragged highlight, so each is opened
// here rather than reasoned about.
func TestCanvas_OverlaysHaveNoHoles(t *testing.T) {
	paintedSlots(t)

	for _, tc := range []struct {
		name string
		open func(testing.TB, ui.RootModel) ui.RootModel
	}{
		{"help", pressKey('?')},
		{"search", pressKey('/')},
		{"context-menu", func(t testing.TB, m ui.RootModel) ui.RootModel {
			return pressKey(' ')(t, focusChat(t, m))
		}},
		{"chat-menu", func(t testing.TB, m ui.RootModel) ui.RootModel {
			return pressKey(' ')(t, m.WithFocus(ui.FocusChatList))
		}},
		{"reaction-picker", func(t testing.TB, m ui.RootModel) ui.RootModel {
			return pressKey('t')(t, focusChat(t, m))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.open(t, newPopulatedRoot(t, 120, 40))
			found := holes(m.View().Content, 120, 40)
			require.Empty(t, found, report(found, "background"))
		})
	}
}

// A selected message is drawn differently from every other: the bubble carries
// an indicator bar in the margin beside it, spliced into the row rather than
// appended to it. That splice used to assume the margin was plain spaces.
func TestCanvas_SelectedMessageHasNoHoles(t *testing.T) {
	paintedSlots(t)

	for _, size := range scanSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := focusChat(t, newPopulatedRoot(t, size.w, size.h))
			found := holes(m.View().Content, size.w, size.h)
			require.Empty(t, found, report(found, "background"))
		})
	}
}

// The folder bar only exists when the account has folders, so the three-pane
// layout it produces is a different split of the screen from the one every other
// test here renders.
func TestCanvas_FolderBarHasNoHoles(t *testing.T) {
	paintedSlots(t)

	for _, size := range scanSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := withFolders(t, newPopulatedRoot(t, size.w, size.h))
			found := holes(m.View().Content, size.w, size.h)
			require.Empty(t, found, report(found, "background"))
		})
	}
}

// pressKey returns an opener that sends one key press.
func pressKey(c rune) func(testing.TB, ui.RootModel) ui.RootModel {
	return func(t testing.TB, m ui.RootModel) ui.RootModel {
		t.Helper()
		next, _ := m.Update(tea.KeyPressMsg{Code: c, Text: string(c)})
		return next.(ui.RootModel)
	}
}

// focusChat moves focus to the message list, which is what selects a message and
// draws the indicator beside it.
func focusChat(t testing.TB, m ui.RootModel) ui.RootModel {
	t.Helper()
	return pressKey('2')(t, m)
}

// withFolders gives the account folders, which switches the layout to three
// panes with the folder bar on the left.
func withFolders(t testing.TB, m ui.RootModel) ui.RootModel {
	t.Helper()
	next, _ := m.Update(ui.FolderFiltersMsg{Filters: []domain.FolderFilter{
		{ID: 0, Title: "All Chats"},
		{ID: 2, Title: "Work"},
		{ID: 3, Title: "Personal"},
	}})
	return next.(ui.RootModel)
}

// The login screen is almost entirely whitespace around a centred block, which
// is emitted by lipgloss.Place rather than by anything the app padded itself.
func TestCanvas_LoginScreenHasNoHoles(t *testing.T) {
	paintedSlots(t)

	m := newRoot(nil, 50, false)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(ui.RootModel)

	found := holes(m.View().Content, 100, 30)
	require.Empty(t, found, report(found, "background"))
}

// With no canvas nothing is painted, and that has to stay true: the built-ins
// must render exactly as they did before the token existed. This is the same
// claim as TestCanvas_UnsetPaintsNothing, made against a whole frame.
func TestCanvas_UnsetLeavesEveryCellBare(t *testing.T) {
	theme.SetSlots(theme.Slots{Dark: theme.TeleDark, Light: theme.TeleLight})
	theme.Apply(true)

	const w, h = 120, 40
	m := newPopulatedRoot(t, w, h)

	bare := len(holes(m.View().Content, w, h))
	// The status bar is a surface the built-ins do paint, so not every cell is
	// bare. What this pins is that the canvas machinery painted nothing: the
	// field around the surfaces is still the terminal's, as it always was.
	require.Greater(t, bare, w*h/2,
		"with no canvas most of the screen must still fall through to the terminal")
	require.Less(t, bare, w*h,
		"the built-ins paint the status bar, so some cells carry a surface")
}

// scanShape draws msgs in a group chat at every scan size and reports what the
// canvas did not reach. Each shape gets its own short chat rather than a place
// in one long one: the message list renders from the bottom, so at 41x12 a
// crowded chat shows only its last rows and the shape under test is scrolled
// out of the frame that is supposedly checking it.
func scanShape(t *testing.T, msgs []domain.Message) {
	t.Helper()
	for _, size := range scanSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			view := richChat(t, size.w, size.h, msgs).View().Content
			found := holes(view, size.w, size.h)
			require.Empty(t, found, report(found, "background"))
			bare := unowned(view, size.w, size.h)
			require.Empty(t, bare, report(bare, "foreground"))
		})
	}
}

// Every media kind draws a placeholder until art is available, and the label is
// a bare literal at its source. The loop is bounded by the enum rather than by a
// list, so a kind added to domain is scanned here without anyone remembering.
func TestCanvas_MediaPlaceholdersHaveNoHoles(t *testing.T) {
	paintedSlots(t)

	for kind := domain.MediaPhoto; kind < domain.MediaKindCount; kind++ {
		t.Run(fmt.Sprintf("kind%d", int(kind)), func(t *testing.T) {
			scanShape(t, []domain.Message{
				incoming(1, "look at this"),
				mediaOf(incoming(2, ""), kind),
				outgoing(3, "nice"),
			})
		})
	}
}

// An album is drawn as one item with a badge line per part, which is a second
// producer of the same labels and does not go through labelLine.
func TestCanvas_AlbumHasNoHoles(t *testing.T) {
	paintedSlots(t)

	msgs := []domain.Message{incoming(1, "sending a few")}
	msgs = append(msgs, album(2, 4)...)
	scanShape(t, msgs)
}

// Entities are the runs inside a bubble that carry their own style, so each one
// is a pair of seams. code and pre also paint surface_code, which is the one
// legitimate light patch in a painted frame and the reason a screenshot cannot
// triage this on its own.
func TestCanvas_EntitiesHaveNoHoles(t *testing.T) {
	paintedSlots(t)

	for _, kind := range entityTypes {
		t.Run(kind, func(t *testing.T) {
			scanShape(t, []domain.Message{
				incoming(1, "plain"),
				withEntity(incoming(2, "marked up text"), kind),
				outgoing(3, "plain"),
			})
		})
	}
}

// The two separators are drawn from a label and the dashes flanking it, and the
// spaces between them belonged to neither run.
func TestCanvas_SeparatorsHaveNoHoles(t *testing.T) {
	paintedSlots(t)

	scanShape(t, []domain.Message{
		yesterday(incoming(1, "before midnight")),
		incoming(2, "the next day"),
		incoming(3, "and unread"),
	})
}

// An edited message carries a marker before its timestamp, and reactions hang
// under the bubble. Both sit on the bottom border, between runs that each end in
// a reset.
func TestCanvas_EditedAndReactedHaveNoHoles(t *testing.T) {
	paintedSlots(t)

	scanShape(t, []domain.Message{
		reacted(incoming(1, "reacted to")),
		edited(outgoing(2, "edited afterwards")),
		edited(reacted(incoming(3, "both at once"))),
	})
}

// A selected message is drawn with an indicator bar in the margin beside it, and
// the margin is on the other side for an incoming bubble than for an outgoing
// one. TestCanvas_SelectedMessageHasNoHoles only ever selected an outgoing
// message, so one of the two branches of alignBubbleLines was never drawn (#227).
func TestCanvas_SelectedIncomingHasNoHoles(t *testing.T) {
	paintedSlots(t)

	msgs := []domain.Message{
		outgoing(1, "mine"),
		incoming(2, "theirs"),
		outgoing(3, "mine again"),
	}
	for _, size := range scanSizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := focusChat(t, richChat(t, size.w, size.h, msgs))
			// Focus selects the last message, which is outgoing. One step up is
			// the incoming one, which is the branch that was never scanned.
			m = pressKey('k')(t, m)
			found := holes(m.View().Content, size.w, size.h)
			require.Empty(t, found, report(found, "background"))
		})
	}
}
