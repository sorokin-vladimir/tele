package ui

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sorokin-vladimir/tele/internal/core/project"
	"github.com/sorokin-vladimir/tele/internal/domain"
	"github.com/sorokin-vladimir/tele/internal/ui/media"
	"github.com/sorokin-vladimir/tele/internal/ui/screens"
)

// markedRow is one frame line carrying the marker of image id at art row row.
func markedRow(id int64, coverCols, coverRows, winCols, winRows, row int, text string) string {
	return media.InlineMarker(media.InlineRef{
		ID: id, CoverCols: coverCols, CoverRows: coverRows,
		WinCols: winCols, WinRows: winRows, Row: row,
	}) + text
}

func TestInlineRegionsClipTopAndBottom(t *testing.T) {
	// A six-row image of which rows 2..5 are on screen, starting at frame row 10.
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "row"
	}
	for i := 2; i < 6; i++ {
		lines = append(lines, markedRow(7, 10, 6, 10, 6, i, "          ▄▄▄▄"))
	}
	regions := inlineRegions(lines)
	require.Len(t, regions, 1)
	r := regions[0]
	assert.Equal(t, int64(7), r.ref.ID)
	assert.Equal(t, 11, r.row, "1-based frame row of the first visible art row")
	assert.Equal(t, 1, r.col, "marker at the start of the line")
	assert.Equal(t, 2, r.skipTop, "two art rows scrolled off the top")
	assert.Equal(t, 4, r.visible)
}

func TestInlineRegionsUsesMarkerColumn(t *testing.T) {
	lines := []string{
		"│ pane │" + markedRow(3, 4, 2, 4, 2, 0, "▄▄▄▄"),
		"│ pane │" + markedRow(3, 4, 2, 4, 2, 1, "▄▄▄▄"),
	}
	regions := inlineRegions(lines)
	require.Len(t, regions, 1)
	assert.Equal(t, 9, regions[0].col, "1-based column after the 8-cell prefix")
	assert.Equal(t, 2, regions[0].visible)
}

func TestInlineRegionsSameImageTwice(t *testing.T) {
	// The same photo in two bubbles with identical geometry must paint twice.
	lines := []string{
		markedRow(5, 4, 2, 4, 2, 0, "▄▄▄▄") + "   " + markedRow(5, 4, 2, 4, 2, 0, "▄▄▄▄"),
		markedRow(5, 4, 2, 4, 2, 1, "▄▄▄▄") + "   " + markedRow(5, 4, 2, 4, 2, 1, "▄▄▄▄"),
	}
	regions := inlineRegions(lines)
	require.Len(t, regions, 2)
	assert.NotEqual(t, regions[0].key, regions[1].key, "placements are keyed apart")
}

func TestInlineRegionsSameImageStacked(t *testing.T) {
	lines := []string{
		markedRow(5, 4, 2, 4, 2, 0, "art"),
		markedRow(5, 4, 2, 4, 2, 1, "art"),
		"gap",
		markedRow(5, 4, 2, 4, 2, 0, "art"),
		markedRow(5, 4, 2, 4, 2, 1, "art"),
	}
	regions := inlineRegions(lines)
	require.Len(t, regions, 2)
	assert.NotEqual(t, regions[0].key, regions[1].key)
	for _, region := range regions {
		assert.Equal(t, 2, region.visible)
	}
}

func TestInlineRegionsStopAtGap(t *testing.T) {
	lines := []string{
		markedRow(9, 4, 4, 4, 4, 0, "▄▄▄▄"),
		"unrelated",
		markedRow(9, 4, 4, 4, 4, 2, "▄▄▄▄"),
	}
	regions := inlineRegions(lines)
	require.Len(t, regions, 1)
	assert.Equal(t, 1, regions[0].visible, "a gap ends the visible run")
}

func TestInlinePlainFrameHasNoRegions(t *testing.T) {
	assert.Empty(t, inlineRegions([]string{"just", "text", "│ box │"}))
}

// inlineModel is a main-screen model in sixel mode with image id cached.
func inlineModel(t *testing.T) RootModel {
	t.Helper()
	m := newSizedModel(t)
	m.screen = ScreenMain
	m.imageMode = media.ModeSixel
	img := image.NewRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 6), B: 90, A: 255})
		}
	}
	m.imageCache.Add(11, img)
	return m
}

func inlineFrame() []string {
	return []string{
		"header",
		markedRow(11, 8, 4, 8, 4, 0, "        ▄▄▄▄▄▄▄▄"),
		markedRow(11, 8, 4, 8, 4, 1, "        ▄▄▄▄▄▄▄▄"),
		markedRow(11, 8, 4, 8, 4, 2, "        ▄▄▄▄▄▄▄▄"),
		markedRow(11, 8, 4, 8, 4, 3, "        ▄▄▄▄▄▄▄▄"),
		"footer",
	}
}

func finishInlinePaint(t *testing.T, m RootModel, cmd tea.Cmd) (RootModel, tea.RawMsg) {
	t.Helper()
	encoded, ok := cmd().(inlineSixelEncodedMsg)
	require.True(t, ok)
	next, write := m.handleInlineSixelEncoded(encoded)
	require.NotNil(t, write)
	raw, ok := write().(tea.RawMsg)
	require.True(t, ok)
	require.NotEmpty(t, fmt.Sprint(raw.Msg)) // Bubble Tea executes the raw write.
	return next, raw
}

func TestInlineTickPaintsThenSkipsUnchangedFrame(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())

	next, cmd := m.handleInlineSixelTick()
	require.NotNil(t, cmd, "first tick paints")
	next, raw := finishInlinePaint(t, next, cmd)
	payload := fmt.Sprint(raw.Msg)
	assert.Contains(t, payload, "\x1bP", "the payload is a sixel")
	assert.Contains(t, payload, "\x1b[2;1H", "positioned at the first art row")

	_, again := next.handleInlineSixelTick()
	assert.Nil(t, again, "an unchanged frame is not repainted")
}

func TestInlineTickRepaintsChangedFrame(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	next, cmd := m.handleInlineSixelTick()
	require.NotNil(t, cmd)
	next, _ = finishInlinePaint(t, next, cmd)

	// A selection change rewrites the bubble border on the image rows.
	changed := inlineFrame()
	changed[1] = "\x1b[1m" + changed[1]
	next.sixelInline.record(changed)
	_, cmd = next.handleInlineSixelTick()
	assert.NotNil(t, cmd, "a rewritten frame repaints")
}

func TestInlineTickSkipsWithoutCachedImage(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record([]string{
		markedRow(999, 8, 4, 8, 4, 0, "▄▄▄"),
	})
	_, cmd := m.handleInlineSixelTick()
	assert.Nil(t, cmd, "an uncached image is not painted")
}

func TestInlineTickBlocksUnderOverlay(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m.photoViewer = &photoViewer{cols: 10, rows: 10}

	_, cmd := m.handleInlineSixelTick()
	assert.Nil(t, cmd, "no inline paint under the photo modal")

	m2 := m
	m2.photoViewer = nil
	m2.sixelInline.record(inlineFrame()) // View restores the chat frame.
	next, cmd := m2.handleInlineSixelTick()
	assert.NotNil(t, cmd, "the chat repaints once the modal closes")
	_ = next
}

func TestInlineSeqCacheReusesEncoding(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	next, cmd := m.handleInlineSixelTick()
	require.NotNil(t, cmd)
	next, _ = finishInlinePaint(t, next, cmd)

	region := inlineRegions(inlineFrame())[0]
	cached, ok := next.sixelInline.seq(region.seqKey)
	require.True(t, ok, "the encoding is cached")
	assert.Contains(t, cached, "\x1bP")
}

func TestInlineCellSizeChangeDropsPixelEncodingCache(t *testing.T) {
	t.Cleanup(func() { media.SetCellPx(0, 0) })
	media.SetCellPx(8, 16)
	m := inlineModel(t)
	m.sixelInline.putSeq("crop", "old pixels")
	next, _ := m.updateUIMsg(uv.CellSizeEvent{Width: 10, Height: 20})
	_, cached := next.sixelInline.seq("crop")
	assert.False(t, cached)
}

func TestInlineScheduleOnlyInSixelMode(t *testing.T) {
	m := inlineModel(t)
	assert.NotNil(t, m.scheduleInlineSixelPaint())

	m.imageMode = media.ModeBlocks
	assert.Nil(t, m.scheduleInlineSixelPaint())
}

func TestInlineDropsEncodeAfterScroll(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, cmd := m.handleInlineSixelTick()
	require.NotNil(t, cmd)
	encoded := cmd().(inlineSixelEncodedMsg)
	m.sixelInline.record([]string{"a different viewport"})
	_, write := m.handleInlineSixelEncoded(encoded)
	assert.Nil(t, write, "old coordinates must never reach the terminal")
	assert.Empty(t, m.sixelInline.painted)
}

func TestInlineDropsEncodeAfterOverlayOrFontChange(t *testing.T) {
	for _, changed := range []string{"overlay", "font"} {
		t.Run(changed, func(t *testing.T) {
			m := inlineModel(t)
			m.sixelInline.record(inlineFrame())
			m, cmd := m.handleInlineSixelTick()
			encoded := cmd().(inlineSixelEncodedMsg)
			if changed == "overlay" {
				m.photoViewer = &photoViewer{}
			} else {
				m.sixelInline = newInlineSixelState()
			}
			_, write := m.handleInlineSixelEncoded(encoded)
			assert.Nil(t, write)
		})
	}
}

func TestInlineMovedAndRemovedImagesClearOnce(t *testing.T) {
	for _, moved := range []bool{false, true} {
		m := inlineModel(t)
		m.sixelInline.record(inlineFrame())
		m, encode := m.handleInlineSixelTick()
		m, _ = finishInlinePaint(t, m, encode)
		frame := []string{"no image"}
		if moved {
			frame = append([]string{"new row"}, inlineFrame()...)
		}
		m.sixelInline.record(frame)
		require.True(t, m.sixelInline.clearPending)
		m, clear := m.handleInlineSixelTick()
		require.NotNil(t, clear, "invalidate the terminal's stale graphics")
		assert.True(t, m.sixelInline.clearing)
		m, _ = m.handleInlineSixelCleared(inlineSixelClearedMsg{state: m.sixelInline})
		assert.False(t, m.sixelInline.clearPending)
		assert.Empty(t, m.sixelInline.painted)
		m.sixelInline.record(frame)
		assert.False(t, m.sixelInline.clearPending, "idle frames must not clear repeatedly")
	}
}

func TestInlineOnlyOneEncodeInFlight(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, first := m.handleInlineSixelTick()
	require.NotNil(t, first)
	_, duplicate := m.handleInlineSixelTick()
	assert.Nil(t, duplicate)
	_, _ = finishInlinePaint(t, m, first)
}

func TestInlineQueuedWriteDropsAfterClearOrScroll(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, encode := m.handleInlineSixelTick()
	m, raw := finishInlinePaint(t, m, encode)
	require.NotEmpty(t, fmt.Sprint(raw.Msg))
	m.sixelInline.record([]string{"closed chat"})
	_, clear := m.handleInlineSixelTick()
	require.NotNil(t, clear)
	assert.Empty(t, fmt.Sprint(raw.Msg), "an old Raw must not repaint after cleanup")
}

func TestInlineAlbumBadgesKeepPaintInsideEachTile(t *testing.T) {
	m := inlineModel(t).WithImageMode(media.ModeSixel)
	img, ok := m.imageCache.Peek(11)
	require.True(t, ok)
	m.imageCache.Add(22, img)
	m.chat.SetHeader(screens.ChatHeader{ChatID: 1, Title: "Album"})
	m.chat.SetKnownImages(m.imageCache)
	var messages []domain.Message
	for i, id := range []int64{11, 22} {
		messages = append(messages, domain.Message{ID: i + 1, ChatID: 1, SenderID: 7, GroupedID: 9,
			Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: id}})
	}
	m.chat.SetMessages(messages)
	regions := inlineRegions(strings.Split(m.View().Content, "\n"))
	require.Len(t, regions, 4, "each tile has a badge-cropped first row and the remaining rows")
	for _, id := range []int64{11, 22} {
		var rightEdges []int
		for _, r := range regions {
			if r.ref.ID == id {
				rightEdges = append(rightEdges, r.col+r.ref.WinCols)
			}
		}
		require.Len(t, rightEdges, 2)
		assert.Equal(t, rightEdges[0], rightEdges[1], "the badge must not shift a full-width image beyond its tile")
	}
}

func TestInlineClearInvalidatesRendererWithUnchangedCells(t *testing.T) {
	m := inlineModel(t)
	before := m.View().Content
	m, _ = m.handleInlineSixelCleared(inlineSixelClearedMsg{state: m.sixelInline})
	after := m.View().Content
	assert.NotEqual(t, before, after, "Bubble Tea must not skip its pending screen erase")
	assert.Equal(t, xansi.Strip(before), xansi.Strip(after))
	assert.Equal(t, after, m.View().Content, "idle views remain identical")
}

// The inline painter finds coordinates in the assembled ANSI frame. They must
// agree with the cell grid Bubble Tea actually draws, including on a row with
// an ASCII-based keycap in the chat list to the left of the image.
func TestInlineImageCoordinatesMatchUnicodeFrameCells(t *testing.T) {
	m := inlineModel(t).WithImageMode(media.ModeSixel)
	m.folderBar.SetFolders([]domain.FolderFilter{{ID: 1, Title: "Folder"}})
	rows := make([]project.ChatRow, 50)
	for i := range rows {
		rows[i] = project.ChatRow{ID: int64(i + 1), Title: "2\ufe0f\u20e3 Channel", Unread: 99}
	}
	m.chatList.SetWindow(0, len(rows), rows)
	m.chat.SetHeader(screens.ChatHeader{ChatID: 1, Title: "Synthetic photo"})
	m.chat.SetKnownImages(m.imageCache)
	m.chat.SetMessages([]domain.Message{{ID: 1, ChatID: 1,
		Media: &domain.MediaRef{Kind: domain.MediaPhoto}, Photo: &domain.PhotoRef{ID: 11}}})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(RootModel)
	frame := m.View().Content
	regions := inlineRegions(strings.Split(frame, "\n"))
	require.NotEmpty(t, regions)
	buf := uv.NewScreenBuffer(120, 40)
	buf.Method = xansi.GraphemeWidth
	uv.NewStyledString(frame).Draw(buf, uv.Rect(0, 0, 120, 40))
	for _, r := range regions {
		for y := r.row - 1; y < r.row-1+r.visible; y++ {
			cell := buf.CellAt(r.col+r.ref.WinCols-2, y)
			require.NotNil(t, cell)
			require.NotEmpty(t, cell.Content)
			require.Contains(t, "\u2580\u2584", cell.Content, "Sixel right edge differs from text image cells on row %d", y)
		}
	}
}

func TestWidthModeResetRejectsQueuedSixelWritesAndRepaints(t *testing.T) {
	m := inlineModel(t)
	m.sixelInline.record(inlineFrame())
	m, encode := m.handleInlineSixelTick()
	m, raw := finishInlinePaint(t, m, encode)
	require.NotEmpty(t, fmt.Sprint(raw.Msg))
	require.NotEmpty(t, m.sixelInline.painted)
	// The frame's cells are unchanged; only width negotiation cleared it.
	next, _ := m.updateInner(widthRepaintMsg{})
	m = next.(RootModel)
	require.Empty(t, m.sixelInline.painted)
	require.Empty(t, fmt.Sprint(raw.Msg), "a queued write must not use the previous width grid")
	_, repaint := m.handleInlineSixelTick()
	require.NotNil(t, repaint, "graphics erased by the reset need a fresh paint")
}
